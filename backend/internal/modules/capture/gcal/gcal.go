// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package gcal is a read-only Google Calendar capture connector: it authorizes
// a user's calendar over OAuth2, pulls events incrementally through the
// Calendar v3 sync-token API, and normalizes each external meeting into a
// meeting activity. It implements connector.Connector, so every captured row
// lands through the ONE capture Sink (audit + outbox in one transaction) — this
// package owns the provider I/O (client.go) and the pure event mapping
// (event.go), nothing about the write.
//
// Like the Gmail connector (and unlike the one-shot IMAP puller) a calendar
// connection is standing: the refresh token is persisted (via the registry →
// keyvault) and the sync cursor is the Calendar syncToken, so each Sync resumes
// where the last left off and is idempotent on (gcal, event id). Polling-first
// by design: real-time push (Calendar watch channels) is a later follow-up —
// this connector is driven only by the background DueConnections → SyncOnce
// poll.
package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const connectorName = "gcal"

// Connector authorizes and syncs Google Calendars. It holds NO per-connection
// or per-run state: one instance is registered and shared, and every Sync
// derives the owner as a local so concurrent syncs of different connections
// never race. (owner is a field only for the pure Normalize surface, which is
// test-only — Sync never touches it.)
type Connector struct {
	oauth googleconn.Authorizer
	api   API
	owner string // used ONLY by Normalize (the test-guarded pure mapping); never set by Sync
	// now dates the capture horizon and the list's age; a field so tests pin
	// the clock.
	now func() time.Time
}

// New returns a Calendar connector over the given OAuth + API surfaces.
func New(oauth googleconn.Authorizer, api API) *Connector {
	return &Connector{oauth: oauth, api: api, now: time.Now}
}

var (
	_ connector.Connector      = (*Connector)(nil)
	_ connector.GrantedScoper  = (*Connector)(nil)
	_ connector.AccountLabeler = (*Connector)(nil)
)

// GrantedScopes reports the Google scopes this calendar connection actually
// holds — the shared Google bundle reader; Google's vocabulary, never this
// system's.
func (c *Connector) GrantedScopes(auth connector.Auth) ([]string, error) {
	return googleconn.GrantedScopes(auth)
}

// AccountLabel reports the calendar account this connection authorized — the
// shared Google bundle reader. It is what lets a human holding two Google
// connections tell them apart, and what binds a connection's cursor to an
// account rather than a row.
func (c *Connector) AccountLabel(auth connector.Auth) (string, error) {
	return googleconn.AccountLabel(auth)
}

// cursorState is the persisted incremental watermark: Calendar's syncToken.
type cursorState struct {
	SyncToken string `json:"sync_token"`
	// ListedAt is when the full list this token descends from was taken. Zero
	// is a cursor written before it was recorded, which re-lists on the next
	// sync and starts keeping it (listIsStale).
	ListedAt time.Time `json:"listed_at,omitzero"`
}

// AuthRequestFrom packages an OAuth callback's code into the opaque connector
// AuthRequest the callback handler passes to Authenticate. It is the shared
// Google handshake — the calendar owner is resolved from the primary calendar
// during Authenticate (googleconn.OwnerResolver).
func AuthRequestFrom(code, redirectURI string) (connector.AuthRequest, error) {
	return googleconn.AuthRequestFrom(code, redirectURI)
}

// Descriptor is the connector's static metadata: name "gcal", read-only
// (TierAutoExecute), producing activities — the shared Google connector shape.
func (c *Connector) Descriptor() connector.Descriptor {
	return googleconn.Descriptor(connectorName)
}

// Authenticate exchanges the authorization code for a refresh token, resolves
// the calendar owner (the primary calendar's address), and returns the opaque
// Auth the registry seals into the vault. The shared Google handshake does the
// work; the only calendar-specific step is resolving the owner.
func (c *Connector) Authenticate(ctx context.Context, req connector.AuthRequest) (connector.Auth, error) {
	return googleconn.Authenticate(ctx, c.oauth, req, c.Descriptor().Scopes, c.api.PrimaryOwner)
}

// Sync mints a fresh access token, then pulls incrementally: with no cursor it
// backfills a bounded window and anchors the returned syncToken; with a cursor
// it lists the events changed since. A cursor Google no longer honors
// (ErrSyncTokenGone) degrades to a bounded re-list rather than a full re-scan.
// The advanced syncToken is returned as the new cursor; the registry persists
// it only on a fully-successful Sync.
func (c *Connector) Sync(ctx context.Context, auth connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	owner, access, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return nil, err
	}

	prior, err := parseCursor(cursor)
	if err != nil {
		// A stored cursor we can't read is a bug/corruption, NOT a fresh
		// calendar: stop and let the next cycle retry rather than silently
		// backfilling and overwriting the watermark.
		return nil, err
	}
	events, nextToken, relisted, err := c.selectEvents(ctx, access, prior)
	if err != nil {
		return nil, err
	}

	edge := c.now().Add(captureForwards)
	for _, raw := range events {
		// Past the horizon the event is not captured, but a meeting captured
		// while it was nearer and moved out since still moves with it.
		if beyondHorizon(raw, owner, edge) {
			if err := meetingmap.MoveOne(ctx, raw, sink, owner, connectorName, decodeEvent); err != nil {
				return nil, err
			}
			continue
		}
		if err := meetingmap.CaptureOne(ctx, raw, sink, owner, connectorName, decodeEvent); err != nil {
			return nil, err
		}
	}

	// selectEvents (through listPages) guarantees a non-empty syncToken on a
	// successful pull, so the advanced watermark is always real here.
	//
	// The listing date carries forward across an incremental round and is reset
	// only by a re-list, because it dates the WINDOW rather than the pull:
	// refreshing it every sync would mean the list never reads as stale.
	listedAt := prior.ListedAt
	if relisted {
		listedAt = c.now()
	}
	return marshalCursor(nextToken, listedAt), nil
}

// selectEvents resolves which events to pull and the syncToken to advance to,
// choosing the initial-backfill or the incremental path, and reports whether it
// re-listed.
//
// It re-lists on THREE conditions: no cursor (a fresh calendar), a token Google
// no longer honors, and a list gone stale. The third is not an error path — the
// token works, but occurrences that were past the capture horizon when the list
// was taken never change, so no incremental pull will ever mention them again.
//
// A stale list with a working token drains the token FIRST. The full list only
// reaches back 90 days, so a change still waiting in the old token — a decline
// on a meeting that ended four months ago — would otherwise be dropped with the
// token it was waiting in.
func (c *Connector) selectEvents(ctx context.Context, access string, cur cursorState) ([][]byte, string, bool, error) {
	if cur.SyncToken == "" {
		events, next, err := c.api.ListInitial(ctx, access)
		return events, next, true, err
	}
	events, next, err := c.api.ListIncremental(ctx, access, cur.SyncToken)
	if errors.Is(err, ErrSyncTokenGone) {
		events, next, err = c.api.ListInitial(ctx, access)
		return events, next, true, err
	}
	if err != nil {
		return nil, "", false, err
	}
	if c.listIsStale(cur.ListedAt) {
		listed, fresh, err := c.api.ListInitial(ctx, access)
		if err != nil {
			return nil, "", false, err
		}
		return append(events, listed...), fresh, true, nil
	}
	return events, next, false, nil
}

// Normalize maps ONE raw Calendar event resource to its meeting activity — the
// shared calendar mapping over this connector's own decode.
func (c *Connector) Normalize(_ context.Context, raw connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return meetingmap.NormalizeOne(raw, c.owner, connectorName, decodeEvent)
}

// HealthCheck confirms the stored credential still mints a token and the
// calendar answers. An outage degrades capture but never blocks core CRM.
func (c *Connector) HealthCheck(ctx context.Context, auth connector.Auth) error {
	_, access, err := googleconn.Session(ctx, c.oauth, auth)
	if err != nil {
		return err
	}
	if _, err := c.api.PrimaryOwner(ctx, access); err != nil {
		return err
	}
	return nil
}

// parseCursor reads the stored watermark. An empty cursor means a genuinely
// fresh calendar (→ initial backfill); a NON-empty but unreadable cursor is an
// error, not a silent re-anchor — the caller stops rather than backfill and
// overwrite the watermark.
func parseCursor(cur connector.Cursor) (cursorState, error) {
	if len(cur) == 0 {
		return cursorState{}, nil
	}
	var cs cursorState
	if err := json.Unmarshal(cur, &cs); err != nil {
		return cursorState{}, fmt.Errorf("gcal: unreadable sync cursor: %w", err)
	}
	if cs.SyncToken == "" {
		// A stored-but-empty token is corruption, NOT a fresh calendar: stop
		// rather than silently re-backfill and overwrite the watermark.
		return cursorState{}, fmt.Errorf("gcal: sync cursor carries no token")
	}
	return cs, nil
}

func marshalCursor(syncToken string, listedAt time.Time) connector.Cursor {
	// cursorState holds a string and a time, so Marshal cannot fail here.
	b, _ := json.Marshal(cursorState{SyncToken: syncToken, ListedAt: listedAt}) //nolint:errchkjson // string+time struct never errors
	return b
}
