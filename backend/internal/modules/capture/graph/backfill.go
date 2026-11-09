// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The bounded backfill (ADR-0063): Graph enumerates a mailbox backward from a
// date boundary via /me/messages $filter=receivedDateTime ge. The walk keeps to
// the folders the standing sync reads, and the estimate is the provider's
// @odata.count over those folders. The page walk uses the same GetMIME and
// capture path as incremental sync, so a message both paths see lands once.

package graph

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/modules/capture/orderedfetch"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// backfillPageSize bounds one BackfillPage call ($top); the engine commits
// cursor and counters between pages, so this is also the resume granularity.
const backfillPageSize = 100

// EstimateBackfill asks the provider how many messages the window holds.
//
// Never a floor: Graph answers an exact `$count` for the filter, so there is
// no cap to bind and nothing to qualify.
func (c *Connector) EstimateBackfill(ctx context.Context, auth connector.Auth, after time.Time) (connector.BackfillEstimate, error) {
	st, err := graphconn.Read(connectorName, auth)
	if err != nil {
		return connector.BackfillEstimate{}, err
	}
	refreshed, err := c.oauth.Refresh(ctx, st.RefreshToken, st.Granted)
	if err != nil {
		return connector.BackfillEstimate{}, err
	}
	count, err := c.api.EstimateAfter(ctx, refreshed.AccessToken, after)
	if err != nil {
		return connector.BackfillEstimate{}, err
	}
	return connector.BackfillEstimate{Messages: count}, nil
}

// BackfillPage pulls one page of the window, oldest-boundary inclusive,
// through the same capture path as incremental sync. The page token is the
// provider's own @odata.nextLink (the client refuses one that points off the
// Graph API).
func (c *Connector) BackfillPage(ctx context.Context, auth connector.Auth, after time.Time, pageToken string, sink connector.Sink) (connector.BackfillPageResult, error) {
	st, err := graphconn.Read(connectorName, auth)
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	refreshed, err := c.oauth.Refresh(ctx, st.RefreshToken, st.Granted)
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	access := refreshed.AccessToken
	// The listing spans the whole mailbox, so the page resolves the synced
	// folders' opaque ids first and steps over mail filed anywhere else. Sent
	// Items is also the proof the owner sent a message. Without it the page
	// would store its mail as unattested for good, so a failure stops the page.
	folders, err := c.syncedFolderIDs(ctx, access)
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	msgs, next, err := c.api.ListAfter(ctx, access, after, pageToken, backfillPageSize)
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	res := connector.BackfillPageResult{NextToken: next}
	// The engine hears the tally per message. Scanned counts messages walked so
	// far, because a page that dies mid-walk is retried from its committed
	// token. Mail outside the synced folders is not counted, as in the estimate.
	progress := connector.BackfillProgressFrom(ctx)
	var walked []MessageRef
	for _, msg := range msgs {
		if _, synced := folders[msg.ParentFolderID]; synced {
			walked = append(walked, msg)
		}
	}
	err = backfillWalk.InOrder(ctx, len(walked),
		func(ctx context.Context, i int) ([]byte, error) {
			// Through the folder the listing found it in, so a message moved to
			// Junk Email since is refused by Graph instead of read.
			return c.api.GetMIME(ctx, access, walked[i].ParentFolderID, walked[i].ID)
		},
		func(i int, raw []byte, err error) error {
			msg := walked[i]
			captured, err := c.backfillOne(ctx, raw, err, msg, sink, st.Owner, folders[msg.ParentFolderID] == folderSent)
			if err != nil {
				return err
			}
			res.Scanned++
			if captured {
				res.Captured++
			} else {
				res.Skipped++
			}
			progress.Observed(ctx, res.Scanned, res.Captured, res.Skipped)
			return nil
		})
	if err != nil {
		return connector.BackfillPageResult{}, err
	}
	return res, nil
}

// syncedFolderIDs maps each synced folder's opaque id to its well-known name.
func (c *Connector) syncedFolderIDs(ctx context.Context, access string) (map[string]string, error) {
	out := make(map[string]string, len(syncedFolders))
	for _, folder := range syncedFolders {
		id, err := c.api.FolderID(ctx, access, folder)
		if err != nil {
			return nil, err
		}
		out[id] = folder
	}
	return out, nil
}

// backfillOne settles one listed message from its fetched MIME. Its bool is the
// page's tally: captured, or skipped for an ordinary reason a message yields no
// row.
func (c *Connector) backfillOne(ctx context.Context, raw []byte, fetchErr error, msg MessageRef, sink connector.Sink, owner string, sentByOwner bool) (bool, error) {
	if errors.Is(fetchErr, connector.ErrSkip) {
		// A message the provider refuses to hand over: deleted between the
		// listing and the fetch, or an oversized MIME blob that truncated
		// would not be honest evidence. Both are per-message drops, counted
		// and stepped over the same way the incremental pull does —
		// returning either here would fail the page, and the engine would
		// retry from its committed token straight back onto the same
		// message, forever.
		return false, nil
	}
	if fetchErr != nil {
		// Stop the page without advancing, so a retry resumes from the
		// committed token. Whether there IS a retry is the engine's call on
		// the class this error carries: a rate limit or an unreachable
		// provider is waited out, anything else ends the run.
		return false, fetchErr
	}
	return captureOne(ctx, raw, sink, c.bounces, owner, msg.ParentFolderID, msg.ID, sentByOwner, msg.ReceivedAt)
}

// backfillWalk fetches a few messages of a page at once and captures them in
// the page's order. Microsoft allows four concurrent requests per mailbox. Two
// leave one for live sync and one for a send. A fetch reserves twice the MIME cap, since
// reading grows its buffer past what it keeps, from a budget every walk in the
// process shares.
var backfillWalk = orderedfetch.Walk[[]byte]{
	Workers: 2, Lookahead: 4,
	Budget: semaphore.NewWeighted(8 * 2 * maxMIMELen), Reserve: 2 * maxMIMELen,
	Retained: func(raw []byte) int64 { return int64(cap(raw)) },
	// Every fetch error but a refused message ends the page (backfillOne).
	// So none is worth the rest of the look-ahead, a rate limit least of all.
	Halts: func(err error) bool { return !errors.Is(err, connector.ErrSkip) },
}
