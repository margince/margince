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
	return "receivedDateTime ge " + after.UTC().Format(time.RFC3339)
}

// receivedBetweenFilter closes that window at the top: ge <after> and lt
// <before>. Second-grain rendering floors the upper bound, which narrows the
// window rather than widening it — the safe direction for a tally a walk is
// measured against.
func receivedBetweenFilter(after, before time.Time) string {
	return receivedAfterFilter(after) + " and receivedDateTime lt " + before.UTC().Format(time.RFC3339)
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
	} `json:"value"`
	NextLink  string `json:"@odata.nextLink"`  //nolint:tagliatelle // Microsoft's wire format; must match to decode
	DeltaLink string `json:"@odata.deltaLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
}

func (a *httpAPI) DeltaInit(ctx context.Context, accessToken, folder string, after time.Time) ([]string, string, error) {
	opened := time.Now().UTC()
	q := url.Values{paramFilter: {receivedAfterFilter(after)}}
	// receivedDateTime bounds BOTH folders: Exchange stamps it on a sent
	// message too, so one filter serves the pair rather than the sent side
	// needing sentDateTime and a second spelling of "recently".
	ids, deltaLink, err := a.deltaWalk(ctx, accessToken,
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
	if len(ids) < held {
		return nil, "", fmt.Errorf(
			"graph: the %s anchor closed after %d message(s) but the folder holds %d since %s: %w",
			folder, len(ids), held, after.UTC().Format(time.RFC3339), ErrUnreachable)
	}
	return ids, deltaLink, nil
}

// countInFolder reports how many messages one folder holds for a filter, so a
// walk over that folder can be measured against the provider's own tally
// rather than trusted because its last round closed.
func (a *httpAPI) countInFolder(ctx context.Context, accessToken, folder, filter string) (int, error) {
	var out struct {
		Count int `json:"@odata.count"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
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
	return out.Count, nil
}

func (a *httpAPI) Delta(ctx context.Context, accessToken, deltaLink string) ([]string, string, error) {
	if err := a.sameAPIOrigin(deltaLink); err != nil {
		return nil, "", err
	}
	return a.deltaWalk(ctx, accessToken, deltaLink)
}

// deltaWalk follows a delta round from startURL through every nextLink until
// Graph hands back the deltaLink that closes it, collecting the ids of the
// non-tombstoned messages along the way. A 410 anywhere in the walk means the
// server no longer honors this delta state (ErrDeltaGone).
func (a *httpAPI) deltaWalk(ctx context.Context, accessToken, startURL string) ([]string, string, error) {
	var ids []string
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
				ids = append(ids, m.ID)
			}
		}
		if page.NextLink == "" {
			if page.DeltaLink == "" {
				// A closed round with neither a next nor a delta link has lost
				// the resumable cursor — treat the malformed response as
				// unreachable rather than reporting success with no watermark.
				return nil, "", ErrUnreachable
			}
			return ids, page.DeltaLink, nil
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
