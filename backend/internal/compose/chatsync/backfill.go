// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// Backfill: a link to an existing channel may ask for the talk so far. Only
// the latest closed episodes from before the link are judged; the older talk
// is skipped, and the cursor ends where the link began. A channel Margince
// created never asks.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
)

const (
	// backfillEpisodes is how many summaries the talk so far may file.
	backfillEpisodes = 5
	// backfillPages bounds one tick's read of a long history.
	backfillPages = 50
	// backfillWindow is how many messages of the talk so far stay in memory;
	// the latest episodes sit in them.
	backfillWindow = 500
)

func (p *Puller) backfill(ctx context.Context, session chatlinks.Session, link chatlinks.Link) error {
	read, err := readHistory(ctx, session.Chat, link, backfillPages, judged)
	if err != nil {
		return err
	}
	older, boundary, finished := beforeLink(read, link)
	cursor := link.HistoryCursor
	if !finished {
		// A long history: skip ahead, keeping the last window of talk for the
		// next tick to read again. A read that kept no more than the window
		// skips it too, or the next tick would read the same pages.
		to := read.highest
		if n := len(older); n > backfillWindow {
			to = older[n-backfillWindow-1].Seq
		}
		_, err := p.Store.AdvanceHistory(ctx, link.ID, cursor, to, false)
		return err
	}
	episodes := groupEpisodes(older, p.Now(), p.Zone)
	if n := len(episodes); n > 0 && !episodes[n-1].closed {
		return nil
	}
	if n := len(episodes); n > backfillEpisodes {
		skipped := episodes[n-backfillEpisodes-1].lastSeq()
		if moved, err := p.Store.AdvanceHistory(ctx, link.ID, cursor, skipped, false); err != nil || !moved {
			return err
		}
		cursor = &skipped
		episodes = episodes[n-backfillEpisodes:]
	}
	for _, ep := range episodes {
		if err := p.judge(ctx, session, link, ep); err != nil {
			return err
		}
		next := ep.lastSeq()
		if moved, err := p.Store.AdvanceHistory(ctx, link.ID, cursor, next, false); err != nil || !moved {
			return err
		}
		cursor = &next
	}
	_, err = p.Store.AdvanceHistory(ctx, link.ID, cursor, max(boundary, seqOf(cursor)), true)
	return err
}

// beforeLink splits what was read at the link's creation. It answers the
// messages from before it and the last seq the backfill may move the cursor
// to. It also says whether the talk before the link was read to its end.
func beforeLink(read historyRead, link chatlinks.Link) ([]dispact.Message, int64, bool) {
	for i, m := range read.kept {
		if !m.PostedAt.Before(link.CreatedAt) {
			return read.kept[:i], m.Seq - 1, true
		}
	}
	return read.kept, read.highest, read.ended
}
