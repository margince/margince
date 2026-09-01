// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package graphcal is a read-only Microsoft 365 (Outlook) calendar capture
// connector: it authorizes a user's calendar over the Microsoft identity
// platform, pulls events incrementally through the Graph calendarView delta
// query, and normalizes each external meeting into a meeting activity. It
// implements connector.Connector, so every captured row lands through the ONE
// capture Sink (audit + outbox in one transaction) — this package owns the
// provider I/O (client.go) and the Graph event decode (event.go), and composes
// the shared meeting rules (capture/meetingmap); nothing about the write.
//
// It is the Microsoft twin of the gcal connector, and a SEPARATE connection
// from the Outlook mailbox for the same reason gcal is separate from Gmail:
// one consent, one scope, one thing a person can disconnect without losing the
// other.
//
// Like the mail connectors a calendar connection is standing: the refresh token
// is persisted (via the registry → keyvault) and the sync cursor is Graph's
// deltaLink, so each Sync resumes where the last left off and is idempotent on
// (graphcal, event id). Polling-first by design: Graph change-notification
// subscriptions are a later follow-up, as they are for the Outlook mailbox —
// this connector is driven only by the background DueConnections → SyncOnce
// poll.
package graphcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const connectorName = "graphcal"

// reanchorAfter is how long a calendarView window is tracked before it is
// reopened around the current moment.
//
// Monthly, because the cost and the benefit are both small and neither is
// urgent: a re-anchor is one bounded walk of the window, and what it buys is
// that a meeting booked past the forward edge enters the window within a month
// of being bookable at all. The forward edge is a year out, so nothing anybody
// schedules for this quarter is ever waiting on it.
const reanchorAfter = 30 * 24 * time.Hour

// Connector authorizes and syncs Microsoft 365 calendars. It holds NO
// per-connection or per-run state: one instance is registered and shared, and
// every Sync derives the owner as a local so concurrent syncs of different
// connections never race. (owner is a field only for the pure Normalize
// surface, which is test-only — Sync never touches it.)
type Connector struct {
	oauth Authorizer
	api   API
	owner string // used ONLY by Normalize (the test-guarded pure mapping); never set by Sync
	// now dates the calendar window; a field so tests pin the clock.
	now func() time.Time
	// rotations receives the replacement credential Microsoft issues on every
	// token redemption — the same seam the Outlook mailbox uses, because the
	// calendar holds its own refresh token and ages out on the same clock.
	rotations connector.CredentialSink
}

// WithCredentialSink returns a COPY that reports the refresh token Microsoft
// replaces on every redemption (connector.CredentialRotator).
//
// A copy, not a field on the shared instance: one registry entry serves every
// connection the fleet syncs at once.
//
//nolint:ireturn // implements connector.CredentialRotator, whose contract returns the Connector seam
func (c *Connector) WithCredentialSink(sink connector.CredentialSink) connector.Connector {
	copied := *c
	copied.rotations = sink
	return &copied
}

// New returns a calendar connector over the given Authorizer + API surfaces.
//
// The Authorizer is the TOKEN half of the handshake, not the whole of it: a
// connector never sends anybody to a consent screen, and the installation's app
// may be resolved per call, which building a consent URL cannot report a failure
// from. The transport holds the wider OAuth surface.
func New(oauth Authorizer, api API) *Connector {
	return &Connector{oauth: oauth, api: api, now: time.Now}
}

var (
	_ connector.Connector         = (*Connector)(nil)
	_ connector.GrantedScoper     = (*Connector)(nil)
	_ connector.AccountLabeler    = (*Connector)(nil)
	_ connector.CredentialRotator = (*Connector)(nil)
)

// cursorState is the persisted incremental watermark: Graph's deltaLink, plus
// the account the watermark belongs to — mirroring the mail cursor shape so
// anything that routes on sync_cursor->>'email' works unchanged.
type cursorState struct {
	DeltaLink string `json:"delta_link"`
	Email     string `json:"email"`
	// AnchoredAt is when the calendarView window this delta tracks was opened.
	//
	// It exists because the window does NOT move on its own. Graph's
	// calendarView delta reports only events starting inside the range it was
	// opened against, and a valid deltaLink is resumed forever — so a meeting
	// booked past the forward edge (a room held for next year's offsite, an
	// annual recurrence) would never be reported and nothing would say so.
	// Re-anchoring on a schedule is what walks the edge forward. Zero means a
	// cursor written before this was recorded, which re-anchors on the next
	// sync and starts keeping it.
	AnchoredAt time.Time `json:"anchored_at,omitzero"`
}

// AuthRequestFrom packages an OAuth callback's code into the opaque connector
// AuthRequest the callback handler passes to Authenticate — the shared
// Microsoft payload.
func AuthRequestFrom(code, redirectURI string) (connector.AuthRequest, error) {
	return graphconn.AuthRequestFrom(connectorName, code, redirectURI)
}

// Descriptor is the connector's static metadata — the shared Microsoft shape,
// named "graphcal".
func (c *Connector) Descriptor() connector.Descriptor {
	return graphconn.Descriptor(connectorName)
}

// Authenticate exchanges the authorization code for a refresh token, resolves
// the calendar owner, and returns the opaque Auth the registry seals into the
// vault. The access token is discarded — only the refresh token persists.
func (c *Connector) Authenticate(ctx context.Context, req connector.AuthRequest) (connector.Auth, error) {
	code, redirectURI, err := graphconn.ReadAuthRequest(connectorName, req)
	if err != nil {
		return nil, err
	}
	if code == "" {
		return nil, fmt.Errorf("graphcal: authorization code required: %w", ErrAuthRejected)
	}
	grant, err := c.oauth.Exchange(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}
	access, err := c.oauth.AccessToken(ctx, grant.RefreshToken)
	if err != nil {
		return nil, err
	}
	owner, err := c.api.Owner(ctx, access)
	if err != nil {
		return nil, err
	}
	state := graphconn.AuthState{
		RefreshToken: grant.RefreshToken, Owner: owner,
		Scopes: graphconn.ScopeStrings(c.Descriptor().Scopes), Granted: grant.Scopes,
	}
	return graphconn.Seal(connectorName, state)
}

// AccountLabel reports the authorizing account, read from the sealed bundle the
// caller already holds — no vault round-trip, no network. It is what lets a
// human holding a Microsoft mailbox AND a Microsoft calendar tell the two rows
// apart, and what binds a connection's cursor to an account rather than a row.
// A bundle that names no account reports none: absence is not a failure.
func (c *Connector) AccountLabel(auth connector.Auth) (string, error) {
	st, err := readAuth(auth)
	if err != nil {
		return "", err
	}
	return st.Owner, nil
}

// GrantedScopes reports the Microsoft permissions this connection actually
// holds, read from the sealed bundle the consent produced — Microsoft's
// vocabulary, never this system's. A bundle sealed before the grant was
// recorded reports none.
func (c *Connector) GrantedScopes(auth connector.Auth) ([]string, error) {
	st, err := readAuth(auth)
	if err != nil {
		return nil, err
	}
	return st.Granted, nil
}

// Sync mints a fresh access token, then pulls incrementally: with no cursor it
// anchors a fresh calendarView delta over the connector's window; with a cursor
// it resumes the stored deltaLink. A link Graph no longer honors (ErrDeltaGone,
// HTTP 410) degrades to the same bounded re-anchor rather than a failure. The
// advanced deltaLink is returned as the new cursor; the registry persists it
// only on a fully-successful Sync.
func (c *Connector) Sync(ctx context.Context, auth connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	st, err := readAuth(auth)
	if err != nil {
		return nil, err
	}
	owner := st.Owner // local — never stored on the shared instance

	// The grant's own scopes, never the deployment's configured list: asking an
	// older calendar grant for a permission it never consented to is refused by
	// Microsoft at the REFRESH, which stops it syncing rather than narrowing it.
	refreshed, err := c.oauth.Refresh(ctx, st.RefreshToken, st.Granted)
	if err != nil {
		return nil, err
	}
	access := refreshed.AccessToken
	// Reported before the pull: the replacement is already issued and the old
	// token already spent by the time Graph answers.
	graphconn.ReportRotation(ctx, connectorName, c.rotations, st, refreshed.Rotated)

	prior, err := parseCursor(cursor)
	if err != nil {
		// A stored cursor we can't read is a bug/corruption, NOT a fresh
		// calendar: stop and let the next cycle retry rather than silently
		// re-anchoring and overwriting the watermark.
		return nil, err
	}
	events, nextDelta, err := c.selectEvents(ctx, access, prior)
	if err != nil {
		return nil, err
	}

	for _, raw := range events {
		if err := meetingmap.CaptureOne(ctx, raw, sink, owner, connectorName, decodeEvent); err != nil {
			return nil, err
		}
	}

	// selectEvents (through walk) guarantees a non-empty deltaLink on a
	// successful pull, so the advanced watermark is always real here.
	//
	// The anchor carries forward across an incremental round and is reset by a
	// re-anchor, because it dates the WINDOW rather than the pull: refreshing
	// it every sync would mean the window never reads as stale and never moves.
	anchoredAt := prior.AnchoredAt
	if anchoredAt.IsZero() || c.windowIsStale(anchoredAt) {
		anchoredAt = c.now()
	}
	return marshalCursor(nextDelta, owner, anchoredAt), nil
}

// selectEvents resolves which events to pull and the deltaLink to advance to,
// choosing the anchor or the incremental path and folding both fallbacks into
// one place.
//
// It re-anchors on THREE conditions, and the third is the one that is easy to
// miss: no cursor at all (a fresh calendar), a link Graph no longer honors, and
// a window that has gone stale. That last one is not an error path — the delta
// is working perfectly, and reporting only events inside a range fixed months
// ago is exactly what it promises to do.
func (c *Connector) selectEvents(ctx context.Context, access string, cur cursorState) ([][]byte, string, error) {
	if cur.DeltaLink == "" || c.windowIsStale(cur.AnchoredAt) {
		return c.api.ViewInitial(ctx, access)
	}
	events, next, err := c.api.ViewDelta(ctx, access, cur.DeltaLink)
	if errors.Is(err, connector.ErrCursorGone) {
		return c.api.ViewInitial(ctx, access)
	}
	if err != nil {
		return nil, "", err
	}
	return events, next, nil
}

// windowIsStale reports whether the tracked range has fallen far enough behind
// the calendar to be worth reopening. A zero anchor is a cursor written before
// this was recorded: re-anchor once, and it starts carrying one.
func (c *Connector) windowIsStale(anchoredAt time.Time) bool {
	return anchoredAt.IsZero() || c.now().Sub(anchoredAt) >= reanchorAfter
}

// Normalize maps ONE raw Graph event resource to its meeting activity — the
// shared calendar mapping over this connector's own decode.
func (c *Connector) Normalize(_ context.Context, raw connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return meetingmap.NormalizeOne(raw, c.owner, connectorName, decodeEvent)
}

// HealthCheck confirms the stored credential still mints a token and the
// account answers. An outage degrades capture but never blocks core CRM.
func (c *Connector) HealthCheck(ctx context.Context, auth connector.Auth) error {
	st, err := readAuth(auth)
	if err != nil {
		return err
	}
	access, err := c.oauth.AccessToken(ctx, st.RefreshToken)
	if err != nil {
		return err
	}
	if _, err := c.api.Owner(ctx, access); err != nil {
		return err
	}
	return nil
}

// readAuth opens the sealed bundle. One reader so every entry point reports a
// malformed bundle the same way rather than four spellings of one failure.
func readAuth(auth connector.Auth) (graphconn.AuthState, error) {
	var st graphconn.AuthState
	if err := json.Unmarshal(auth, &st); err != nil {
		return graphconn.AuthState{}, fmt.Errorf("graphcal: malformed auth bundle: %w", err)
	}
	return st, nil
}

// parseCursor reads the stored watermark. An empty cursor means a genuinely
// fresh calendar (→ initial anchor); a NON-empty but unreadable cursor is an
// error, not a silent re-anchor — the caller stops rather than re-anchor and
// overwrite the watermark.
func parseCursor(cur connector.Cursor) (cursorState, error) {
	if len(cur) == 0 {
		return cursorState{}, nil
	}
	var cs cursorState
	if err := json.Unmarshal(cur, &cs); err != nil {
		return cursorState{}, fmt.Errorf("graphcal: unreadable sync cursor: %w", err)
	}
	if cs.DeltaLink == "" {
		// A stored-but-empty link is corruption, NOT a fresh calendar: stop
		// rather than silently re-anchor and overwrite the watermark.
		return cursorState{}, fmt.Errorf("graphcal: sync cursor carries no delta link")
	}
	return cs, nil
}

func marshalCursor(deltaLink, email string, anchoredAt time.Time) connector.Cursor {
	// cursorState carries only strings and a time, so Marshal cannot fail here.
	b, _ := json.Marshal(cursorState{DeltaLink: deltaLink, Email: email, AnchoredAt: anchoredAt}) //nolint:errchkjson // no unsupported types
	return b
}
