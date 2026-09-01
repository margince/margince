// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The messages delta: how a mailbox is read forward. An anchor opens a filtered
// round over one well-known folder and walks it to a deltaLink; every later
// cycle resumes from the link the last one stored.
//
// The watermark only ever moves forward, so a round that fell short of its own
// window would drop what it missed for good. That is why an anchor reconciles
// its walk against the folder's own tally before the link it ends on is allowed
// to stand as a watermark.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// receivedAfterFilter renders Graph's only supported delta/list filter:
// receivedDateTime ge <instant>. The window boundary is a product parameter
// measured in months, so second-grain UTC rendering loses nothing.
func receivedAfterFilter(after time.Time) string {
	return "receivedDateTime ge " + filterInstant(after).Format(time.RFC3339)
}

// filterInstant is the lower bound AS MICROSOFT WILL READ IT: second-grain,
// because that is what RFC3339 rendering leaves of it.
//
// Spelled once and shared with the reconciliation, so the two cannot drift. They
// did: the filter asked Graph for everything from the top of the second while
// the tally compared against the exact instant, so a message arriving in the
// fractional remainder counted on Microsoft's side and not on ours — a complete
// anchor refused as short, every time a mailbox happened to receive one there.
//
// Held by: TestASubsecondLowerBoundDoesNotRefuseACompleteAnchor
// (backend/internal/modules/capture/graph/client_test.go)
func filterInstant(t time.Time) time.Time { return t.UTC().Truncate(time.Second) }

// receivedBetweenFilter closes that window at the top: ge <after> and lt
// <before>.
//
// The upper bound keeps its subsecond digits (RFC3339Nano) where the lower one
// does not need them. The lower bound is a product parameter measured in
// months; this one is the instant a walk opened, and Format TRUNCATES, so
// second-grain rendering would move it back by up to a second and drop whatever
// arrived in that second out of the tally the walk is measured against. Losing
// it in that direction is losing it quietly, which is the whole failure this
// window exists to catch.
func receivedBetweenFilter(after, before time.Time) string {
	return receivedAfterFilter(after) + " and receivedDateTime lt " + before.UTC().Format(time.RFC3339Nano)
}

// The two well-known folders a standing sync follows. Named constants because
// they are Microsoft's own identifiers rather than folder names a person chose,
// and because which folder a message came from is what attests whether the
// owner sent it.
const (
	folderInbox = "inbox"
	folderSent  = "sentitems"
)

// deltaPage is one page of a messages delta. A tombstoned entry
// carries @removed instead of message fields — nothing to fetch for it.
type deltaPage struct {
	Value []struct {
		ID      string           `json:"id"`
		Removed *json.RawMessage `json:"@removed"`
		// ReceivedAt is what decides whether an entry belongs to the window a
		// filtered round asked for. A delta tracks a FOLDER, not a query, so
		// Microsoft may emit an entry for a message outside the filter when
		// something about it changed — a read/unread flip on an old message.
		// Zero where the entry does not say.
		ReceivedAt time.Time `json:"receivedDateTime"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	} `json:"value"`
	NextLink  string `json:"@odata.nextLink"`  //nolint:tagliatelle // Microsoft's wire format; must match to decode
	DeltaLink string `json:"@odata.deltaLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
}

// deltaEntry is one message a walk named, with the instant that decides which
// window it belongs to.
type deltaEntry struct {
	id         string
	receivedAt time.Time
}

// ids drops the timestamps: every entry a walk named is a message to fetch,
// whatever window it belongs to.
func entryIDs(entries []deltaEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.id)
	}
	return out
}

// countWithin counts the DISTINCT entries a walk named that belong to
// [after, before) — the window a tally covers, with the lower bound read the way
// Microsoft read it.
//
// An entry that carries no instant does not count. The round ASKS for the field
// ($select on the opening request), so its absence is not the ordinary silence
// of a response that was never asked — it is an entry that cannot be shown to
// belong to the window being reconciled, and one that cannot be shown to belong
// is exactly what must not stand in for a message the round never reached. The
// cost of the strict reading is a loud refusal that the next cycle retries; the
// cost of the lenient one is a watermark stored over a gap, and the watermark
// only moves forward.
func countWithin(entries []deltaEntry, after, before time.Time) int {
	from := filterInstant(after)
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		// One comparison covers the undated entry too: an absent instant decodes
		// to the zero time, which precedes every window an anchor can ask for.
		if e.receivedAt.Before(from) || !e.receivedAt.Before(before) {
			continue
		}
		seen[e.id] = true
	}
	return len(seen)
}

func (a *httpAPI) DeltaInit(ctx context.Context, accessToken, folder string, after time.Time) ([]string, string, error) {
	opened := time.Now().UTC()
	// $select, so the walk is answered in the two fields it reconciles on rather
	// than in whatever Graph would send by default: receivedDateTime is not
	// promised on every entry of a delta round, and this is the request that
	// asks for it. It rides the deltaLink into every later round of the same
	// stream, so a resume is answered the same way.
	q := url.Values{
		paramFilter: {receivedAfterFilter(after)},
		paramSelect: {"id,receivedDateTime"},
	}
	// receivedDateTime bounds BOTH folders: Exchange stamps it on a sent
	// message too, so one filter serves the pair rather than the sent side
	// needing sentDateTime and a second spelling of "recently".
	entries, deltaLink, err := a.deltaWalk(ctx, accessToken,
		a.base+"/me/mailFolders/"+url.PathEscape(folder)+"/messages/delta?"+q.Encode())
	if err != nil {
		return nil, "", err
	}
	// A round that ends with a deltaLink says "you are at the end" but a
	// filtered delta closing early would say exactly the same thing, and the
	// watermark that follows only ever moves forward — so anything the round
	// fell short of would never be offered again. Measure the walk against the
	// folder's own tally before letting the watermark stand.
	held, err := a.countInFolder(ctx, accessToken, folder, receivedBetweenFilter(after, opened))
	if err != nil {
		return nil, "", err
	}
	// The tally is taken AFTER the walk over a window closed at the instant the
	// walk opened, so both ways a mailbox can move meanwhile push the walk's
	// side up rather than the tally's: a message arriving mid-walk is outside
	// the counted window yet may be inside the walk, and one deleted mid-walk
	// leaves both. A walk short of the tally is therefore evidence the round
	// ended early, never an artifact of the gap between the two calls.
	// Measured on the entries that BELONG to the counted window, not on
	// everything the round named: a delta tracks a folder rather than a query,
	// so an entry for an older message that merely changed would otherwise pad
	// this side and let a short round pass. Every entry is still returned — a
	// message named by the round is a message to fetch, whatever window it
	// belongs to.
	if walked := countWithin(entries, after, opened); walked < held {
		return nil, "", fmt.Errorf(
			"graph: the %s anchor closed holding %d message(s) from the window but the folder holds %d since %s: %w",
			folder, walked, held, after.UTC().Format(time.RFC3339), ErrUnreachable)
	}
	return entryIDs(entries), deltaLink, nil
}

// countInFolder reports how many messages one folder holds for a filter, so a
// walk over that folder can be measured against the provider's own tally
// rather than trusted because its last round closed.
func (a *httpAPI) countInFolder(ctx context.Context, accessToken, folder, filter string) (int, error) {
	var out struct {
		// A POINTER, so "Microsoft said none" is distinguishable from "Microsoft
		// did not answer the question". Read as an int, a 2xx that omits
		// @odata.count — the shape a request missing ConsistencyLevel can take —
		// would arrive as a tally of zero, and a tally of zero is one no walk can
		// ever fall short of. The reconciliation would pass every round while
		// measuring nothing, which is the failure it exists to catch wearing the
		// costume of a green check.
		Count *int `json:"@odata.count"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	}
	q := url.Values{
		paramFilter: {filter},
		paramCount:  {"true"},
		paramSelect: {"id"},
		paramTop:    {"1"},
	}
	// $count=true needs the eventual-consistency header on Graph.
	hdr := http.Header{"ConsistencyLevel": {"eventual"}}
	u := a.base + "/me/mailFolders/" + url.PathEscape(folder) + "/messages?" + q.Encode()
	if _, err := a.get(ctx, accessToken, u, hdr, &out); err != nil {
		return 0, err
	}
	if out.Count == nil {
		return 0, fmt.Errorf(
			"graph: the %s tally came back without a count: %w", folder, ErrUnreachable)
	}
	return *out.Count, nil
}

func (a *httpAPI) Delta(ctx context.Context, accessToken, deltaLink string) ([]string, string, error) {
	if err := a.sameAPIOrigin(deltaLink); err != nil {
		return nil, "", err
	}
	entries, next, err := a.deltaWalk(ctx, accessToken, deltaLink)
	if err != nil {
		return nil, "", err
	}
	return entryIDs(entries), next, nil
}

// deltaWalk follows a delta round from startURL through every nextLink until
// Graph hands back the deltaLink that closes it, collecting the ids of the
// non-tombstoned messages along the way. A 410 anywhere in the walk means the
// server no longer honors this delta state (ErrDeltaGone).
func (a *httpAPI) deltaWalk(ctx context.Context, accessToken, startURL string) ([]deltaEntry, string, error) {
	var entries []deltaEntry
	next := startURL
	for {
		var page deltaPage
		status, err := a.get(ctx, accessToken, next, nil, &page)
		if err != nil {
			if status == http.StatusGone {
				return nil, "", ErrDeltaGone
			}
			return nil, "", err
		}
		for _, m := range page.Value {
			if m.ID != "" && m.Removed == nil {
				entries = append(entries, deltaEntry{id: m.ID, receivedAt: m.ReceivedAt.UTC()})
			}
		}
		if page.NextLink == "" {
			if page.DeltaLink == "" {
				// A closed round with neither a next nor a delta link has lost
				// the resumable cursor — treat the malformed response as
				// unreachable rather than reporting success with no watermark.
				return nil, "", ErrUnreachable
			}
			// The deltaLink gets the same check as a nextLink, and needs it
			// more: this one is STORED. A nextLink is followed once and the
			// round ends either way, while an off-origin deltaLink becomes the
			// cursor, and every later cycle then fails its own origin check
			// before fetching anything — a mailbox stuck with no way back,
			// since that refusal is not the 410 a re-anchor answers.
			if err := a.sameAPIOrigin(page.DeltaLink); err != nil {
				return nil, "", err
			}
			return entries, page.DeltaLink, nil
		}
		if err := a.sameAPIOrigin(page.NextLink); err != nil {
			return nil, "", err
		}
		next = page.NextLink
	}
}

// sameAPIOrigin refuses any continuation URL that does not live under the
// configured Graph base. nextLink/deltaLink are server-supplied URLs the
// client follows bearing the access token — and the deltaLink round-trips
// through the stored sync cursor — so an off-origin link (tampered cursor,
// broken provider) must never be fetched.
