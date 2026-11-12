// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// checkpointEveryPages is how many pages pass between two checkpoints. It
// trades what an interrupted round reads again against cursor writes.
const checkpointEveryPages = 10

// pullFolder walks one folder's delta page by page, captures each page, and
// returns the folder's watermark. It checkpoints every checkpointEveryPages
// pages, after those pages are captured.
//
// Sent Items is attested by its folder: Microsoft files a copy there because
// the signed-in account sent it, which a forged header cannot imitate.
func (c *Connector) pullFolder(
	ctx context.Context, access, folder string, state *cursorState, anchorFrom time.Time,
	sink connector.Sink, owner string,
) (string, error) {
	sentByOwner := folder == folderSent
	pages, sinceCheckpoint := 0, 0
	visit := func(ctx context.Context, page DeltaPage) error {
		// The owner's own deletions, reported before the page's new mail is
		// captured. Reporting first is the safe half when a message is deleted
		// and re-delivered in one page: the re-delivery re-captures it.
		if err := reportRemovals(ctx, sink, page.Removed); err != nil {
			return err
		}
		state.cancelPurges(page.Messages)
		if err := c.captureMessages(ctx, access, folder, page.Messages, sink, owner, sentByOwner); err != nil {
			return err
		}
		pages++
		sinceCheckpoint += len(page.Messages)
		if page.Resume.NextLink == "" || !c.checkpointDue(pages, sinceCheckpoint) {
			return nil
		}
		sinceCheckpoint = 0
		return checkpointRound(ctx, state, folder, page.Resume)
	}
	var resume *DeltaResume
	if state.Rounds != nil {
		resume = state.Rounds[folder]
	}
	next, err := c.walkFolder(ctx, access, folder, state.link(folder), resume, anchorFrom, visit)
	if errors.Is(err, ErrAnchorShort) {
		// Resuming the same census cannot find what it missed, so the next
		// cycle opens a fresh anchor instead.
		return "", errors.Join(err, dropRound(ctx, state, folder))
	}
	if err != nil {
		return "", err
	}
	if next == "" {
		next = state.link(folder) // provider closed the round without a new link; keep the prior watermark
	}
	return next, nil
}

// checkpointDue reports whether the page just captured closes a checkpoint
// span, by pages or, where set, by messages.
func (c *Connector) checkpointDue(pages, messages int) bool {
	return pages%checkpointEveryPages == 0 || (c.checkpointMessages > 0 && messages >= c.checkpointMessages)
}

// captureMessages fetches and captures one page's messages.
func (c *Connector) captureMessages(
	ctx context.Context, access, folder string, messages []MessageRef,
	sink connector.Sink, owner string, sentByOwner bool,
) error {
	for _, message := range messages {
		raw, err := c.api.GetMIME(ctx, access, "", message.ID)
		if errors.Is(err, connector.ErrSkip) {
			// The provider refused this message: deleted since the delta named
			// it, or too large to fetch whole. The message is dropped and the
			// cursor still advances past it.
			continue
		}
		if err != nil {
			// A fetch fault is transient: stop without advancing the cursor so
			// the next cycle retries from the last checkpoint.
			return err
		}
		if _, err := captureOne(ctx, raw, sink, c.bounces, owner, folder, message.ID, sentByOwner, message.ReceivedAt); err != nil {
			return err
		}
	}
	return nil
}

// checkpointRound saves the cursor with folder's round standing at resume.
func checkpointRound(ctx context.Context, state *cursorState, folder string, resume DeltaResume) error {
	saved := *state
	saved.Rounds = map[string]*DeltaResume{folder: &resume}
	cursor, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("graph: encoding the %s checkpoint: %w", folder, err)
	}
	return connector.CheckpointSync(ctx, cursor)
}

// dropRound saves the cursor without folder's round, keeping its watermarks.
func dropRound(ctx context.Context, state *cursorState, folder string) error {
	saved := *state
	saved.Rounds = nil
	for other, round := range state.Rounds {
		if other != folder {
			if saved.Rounds == nil {
				saved.Rounds = map[string]*DeltaResume{}
			}
			saved.Rounds[other] = round
		}
	}
	cursor, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("graph: encoding the cursor without the %s round: %w", folder, err)
	}
	return connector.CheckpointSync(ctx, cursor)
}

// walkFolder picks the round to walk: one a checkpoint left, a fresh anchor
// for a folder with no watermark, or the stored watermark's round. A 410
// falls back to a fresh anchor. An anchor round reports no removals, because
// it has no prior state to delete from.
func (c *Connector) walkFolder(
	ctx context.Context, access, folder, link string, resume *DeltaResume, anchorFrom time.Time, visit PageVisitor,
) (string, error) {
	var next string
	var err error
	switch {
	case resume != nil && resume.Anchor != nil:
		next, err = c.api.DeltaInit(ctx, access, folder, anchorFrom, resume, visit)
	case link == "":
		return c.api.DeltaInit(ctx, access, folder, anchorFrom, nil, visit)
	default:
		next, err = c.api.Delta(ctx, access, link, resume, visit)
	}
	if errors.Is(err, ErrDeltaGone) {
		return c.api.DeltaInit(ctx, access, folder, anchorFrom, nil, visit)
	}
	return next, err
}
