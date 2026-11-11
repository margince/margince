// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The proof an Outlook message was deleted by its owner. A folder tombstone or
// a 404 cannot be that proof: a move to the Online Archive changes even the
// immutable id. Arrival in Recoverable Items Deletions can, and the purge
// waits deletionHold so a restore in that window keeps the message.

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	// deletionHold matches Exchange's default deleted-item retention, the
	// owner's own window to restore a hard-deleted message.
	deletionHold = 14 * 24 * time.Hour
	// pendingPurgeCap bounds the cursor. An arrival past it is not recorded,
	// so its captured copy is kept.
	pendingPurgeCap = 5000
	// purgeChecksPerSync bounds the lookups one sync makes for due purges.
	purgeChecksPerSync = 100
)

// deletionsListing is the fallback's memory: the digests of the ids the last
// listing returned, so the next one can tell an arrival from old content.
type deletionsListing struct {
	Seen []string `json:"seen"`
}

// cancelPurges drops the pending purge of every message a synced folder
// holds again: the owner restored it.
func (s *cursorState) cancelPurges(messages []MessageRef) {
	for _, m := range messages {
		delete(s.PendingPurges, m.ID)
	}
}

func (s *cursorState) holdPurge(id string, seen time.Time) {
	if _, held := s.PendingPurges[id]; held || len(s.PendingPurges) >= pendingPurgeCap {
		return
	}
	if s.PendingPurges == nil {
		s.PendingPurges = map[string]time.Time{}
	}
	s.PendingPurges[id] = seen
}

// trackDeletions records what arrived in Deletions since the last sync. The
// first round, and a round re-opened after a 410, only set the baseline:
// what the folder already held is not news. A failure here never stops mail
// capture; the folder is read again next cycle.
func (c *Connector) trackDeletions(ctx context.Context, access string, state *cursorState) error {
	baseline := state.DeletionsDeltaLink == ""
	var arrivals []string
	visit := func(_ context.Context, page DeltaPage) error {
		for _, m := range page.Messages {
			arrivals = append(arrivals, m.ID)
		}
		return nil
	}
	next, err := c.api.DeletionsDelta(ctx, access, state.DeletionsDeltaLink, visit)
	if errors.Is(err, ErrDeltaGone) {
		baseline, arrivals = true, nil
		next, err = c.api.DeletionsDelta(ctx, access, "", visit)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || next == "" {
		c.listDeletions(ctx, access, state)
		return ctx.Err()
	}
	state.DeletionsDeltaLink, state.DeletionsListing = next, nil
	if !baseline {
		now := c.now().UTC()
		for _, id := range arrivals {
			state.holdPurge(id, now)
		}
	}
	return nil
}

// listDeletions is the fallback when the folder's delta fails: a bounded
// listing compared against the previous one. The first listing is a baseline.
func (c *Connector) listDeletions(ctx context.Context, access string, state *cursorState) {
	ids, err := c.api.ListDeletions(ctx, access)
	if err != nil {
		return
	}
	seen := make([]string, 0, len(ids))
	for _, id := range ids {
		seen = append(seen, idDigest(id))
	}
	slices.Sort(seen)
	seen = slices.Compact(seen)
	if state.DeletionsListing != nil {
		now := c.now().UTC()
		for _, id := range ids {
			if _, known := slices.BinarySearch(state.DeletionsListing.Seen, idDigest(id)); !known {
				state.holdPurge(id, now)
			}
		}
	}
	state.DeletionsListing = &deletionsListing{Seen: seen}
}

// purgeDeleted reports each purge whose hold has run out as a mailbox
// deletion. A message found again outside Deletions was restored and is kept.
// A lookup that fails keeps the purge pending for the next cycle.
func (c *Connector) purgeDeleted(ctx context.Context, access string, state *cursorState, sink connector.Sink) error {
	due := duePurges(state.PendingPurges, c.now())
	if len(due) == 0 {
		return nil
	}
	deletionsID, err := c.api.FolderID(ctx, access, folderDeletions)
	if err != nil {
		return ctx.Err()
	}
	for _, id := range due {
		folder, found, err := c.api.MessageFolder(ctx, access, id)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if found && folder != deletionsID {
			delete(state.PendingPurges, id)
			continue
		}
		if err := reportRemovals(ctx, sink, []string{id}); err != nil {
			return err
		}
		delete(state.PendingPurges, id)
	}
	return nil
}

// duePurges lists the ids held at least deletionHold, oldest first, at most
// purgeChecksPerSync of them.
func duePurges(pending map[string]time.Time, now time.Time) []string {
	var due []string
	for id, seen := range pending {
		if now.Sub(seen) >= deletionHold {
			due = append(due, id)
		}
	}
	slices.SortFunc(due, func(a, b string) int {
		if c := pending[a].Compare(pending[b]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	if len(due) > purgeChecksPerSync {
		due = due[:purgeChecksPerSync]
	}
	return due
}
