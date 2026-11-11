// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The proof an Outlook message was deleted by its owner. A folder tombstone or
// a 404 cannot be that proof: a move to the Online Archive changes even the
// immutable id. Arrival in Recoverable Items Deletions can, and the purge
// waits deletionHold so a restore in that window keeps the message. The
// cursor holding the arrivals is written only by the sync; the API serves it
// read-only.

import (
	"cmp"
	"context"
	"errors"
	"maps"
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

// pendingPurge is one message seen arriving in Deletions. InternetMessageID
// finds it again if a restore changed its id.
type pendingPurge struct {
	Seen              time.Time `json:"seen"`
	InternetMessageID string    `json:"internet_message_id,omitempty"`
}

// cancelPurges drops the pending purge of every message a synced folder
// holds again: the owner restored it.
func (s *cursorState) cancelPurges(messages []MessageRef) {
	for _, m := range messages {
		delete(s.PendingPurges, m.ID)
	}
}

func (s *cursorState) holdPurge(m MessageRef, seen time.Time) {
	if _, held := s.PendingPurges[m.ID]; held || len(s.PendingPurges) >= pendingPurgeCap {
		return
	}
	if s.PendingPurges == nil {
		s.PendingPurges = map[string]pendingPurge{}
	}
	s.PendingPurges[m.ID] = pendingPurge{Seen: seen, InternetMessageID: m.InternetMessageID}
}

// boundPendingPurges drops an entry this code cannot have written: one with no
// time or a time after now. Past the cap it keeps the oldest, as holdPurge does.
func (s *cursorState) boundPendingPurges(now time.Time) {
	for id, entry := range s.PendingPurges {
		if entry.Seen.IsZero() || entry.Seen.After(now) {
			delete(s.PendingPurges, id)
		}
	}
	if len(s.PendingPurges) <= pendingPurgeCap {
		return
	}
	for _, id := range oldestPurges(s.PendingPurges)[pendingPurgeCap:] {
		delete(s.PendingPurges, id)
	}
}

// trackDeletions records what arrived in Deletions since the last sync. The
// first round, and a round re-opened after a 410, only set the baseline:
// what the folder already held is not news. A failure here never stops mail
// capture; the folder is read again next cycle.
func (c *Connector) trackDeletions(ctx context.Context, access string, state *cursorState) error {
	baseline := state.DeletionsDeltaLink == ""
	var arrivals []MessageRef
	visit := func(_ context.Context, page DeltaPage) error {
		arrivals = append(arrivals, page.Messages...)
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
		for _, m := range arrivals {
			state.holdPurge(m, now)
		}
	}
	return nil
}

// listDeletions is the fallback when the folder's delta fails: a bounded
// listing compared against the previous one. The first listing is a baseline.
func (c *Connector) listDeletions(ctx context.Context, access string, state *cursorState) {
	listed, err := c.api.ListDeletions(ctx, access)
	if err != nil {
		return
	}
	seen := make([]string, 0, len(listed))
	for _, m := range listed {
		seen = append(seen, idDigest(m.ID))
	}
	slices.Sort(seen)
	seen = slices.Compact(seen)
	if state.DeletionsListing != nil {
		now := c.now().UTC()
		for _, m := range listed {
			if _, known := slices.BinarySearch(state.DeletionsListing.Seen, idDigest(m.ID)); !known {
				state.holdPurge(m, now)
			}
		}
	}
	state.DeletionsListing = &deletionsListing{Seen: seen}
}

// purgeVerdict is what one due purge's lookups prove.
type purgeVerdict int

const (
	// purgeRetry: a lookup failed, so the entry waits for the next sync.
	purgeRetry purgeVerdict = iota
	// purgeCancel: the message is filed outside Deletions, so it is kept.
	purgeCancel
	// purgeReport: the message is in Deletions or gone from the mailbox.
	purgeReport
)

// purgeDeleted reports each purge whose hold has run out as a mailbox
// deletion, but only on proof. A message found outside Deletions is kept.
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
		switch c.purgeProof(ctx, access, deletionsID, id, state.PendingPurges[id]) {
		case purgeRetry:
			if ctx.Err() != nil {
				return ctx.Err()
			}
		case purgeCancel:
			delete(state.PendingPurges, id)
		case purgeReport:
			if err := reportRemovals(ctx, sink, []string{id}); err != nil {
				return err
			}
			delete(state.PendingPurges, id)
		}
	}
	return nil
}

// purgeProof looks the message up by its id, then by its internetMessageId:
// a restore, such as one to the Online Archive, can change the id. Only a
// find in Deletions, or no copy outside it, proves the deletion. With no
// internetMessageId a 404 proves nothing, so the message is kept.
func (c *Connector) purgeProof(ctx context.Context, access, deletionsID, id string, entry pendingPurge) purgeVerdict {
	folder, found, err := c.api.MessageFolder(ctx, access, id)
	switch {
	case err != nil:
		return purgeRetry
	case found && folder == deletionsID:
		return purgeReport
	case found, entry.InternetMessageID == "":
		return purgeCancel
	}
	folders, err := c.api.FindMessage(ctx, access, entry.InternetMessageID)
	if err != nil {
		return purgeRetry
	}
	if slices.ContainsFunc(folders, func(f string) bool { return f != deletionsID }) {
		return purgeCancel
	}
	return purgeReport
}

// duePurges lists the ids held at least deletionHold, oldest first, at most
// purgeChecksPerSync of them.
func duePurges(pending map[string]pendingPurge, now time.Time) []string {
	var due []string
	for _, id := range oldestPurges(pending) {
		if now.Sub(pending[id].Seen) < deletionHold || len(due) == purgeChecksPerSync {
			break
		}
		due = append(due, id)
	}
	return due
}

// oldestPurges lists every pending id, oldest arrival first.
func oldestPurges(pending map[string]pendingPurge) []string {
	ids := slices.Collect(maps.Keys(pending))
	slices.SortFunc(ids, func(a, b string) int {
		if c := pending[a].Seen.Compare(pending[b].Seen); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	return ids
}
