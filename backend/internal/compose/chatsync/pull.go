// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// The chat_pull job: each active link of one bot connection reads its
// channel's history after its cursor. It cuts the talk into episodes and
// judges each closed one in order. A relevant episode is filed as a note; the cursor then
// moves to the episode's last message, so nothing is judged twice. The first
// open episode stops the pull, and the next tick reads it again.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// pullPages caps the history pages one link reads per tick.
const pullPages = 5

// Puller judges the talk of one bot connection's linked channels.
type Puller struct {
	Pool       *pgxpool.Pool
	Store      *chatlinks.Store
	Chats      chatlinks.Chats
	Activities *activities.Store
	// Lane is the chat_summary lane; nil files nothing.
	Lane ai.Completer
	Now  func() time.Time
	// Zone is where a day ends for the end-of-day rule.
	Zone *time.Location
	Log  *slog.Logger
}

// Pull reads every active link of the connection. ctx carries the job's
// workspace; the pull acts as the chat engine's system actor. One link's gone
// channel pauses that link and the others go on.
func (p *Puller) Pull(ctx context.Context, connectionID ids.UUID) error {
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	session, err := p.Chats.Open(ctx)
	if errors.Is(err, chatlinks.ErrBotNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}
	if session.ConnectionID != connectionID {
		return nil
	}
	links, err := p.Store.PullLinks(ctx, connectionID)
	if err != nil {
		return err
	}
	for _, link := range links {
		var err error
		if link.BackfillRequested {
			err = p.backfill(ctx, session, link)
		} else {
			err = p.pullLink(ctx, session, link)
		}
		if err == nil {
			continue
		}
		if stop := p.answerFailure(ctx, link, err); errors.Is(stop, errModelDeferred) {
			return nil
		} else if stop != nil {
			return stop
		}
	}
	return nil
}

// errModelDeferred stops a pull whose model asked it to wait; the next tick
// tries again from the same cursor.
var errModelDeferred = errors.New("chat pull: the model deferred the call")

// pullLink judges the closed episodes after the link's cursor. Talk from
// before the link was made is never judged here: only a backfill asked for
// at link time reads it.
func (p *Puller) pullLink(ctx context.Context, session chatlinks.Session, link chatlinks.Link) error {
	read, err := readHistory(ctx, session.Chat, link, pullPages, func(m dispact.Message) bool {
		return judged(m) && !m.PostedAt.Before(link.CreatedAt)
	})
	if err != nil {
		return err
	}
	episodes := groupEpisodes(read.kept, p.Now(), p.Zone)
	// The episode the last page cut may go on in the next one. It is judged
	// only when it fills the whole read, so one long day cannot stall a link.
	if n := len(episodes); !read.ended && n > 1 {
		episodes[n-1].closed = false
	}
	cursor := link.HistoryCursor
	for _, ep := range episodes {
		if !ep.closed {
			return nil
		}
		if err := p.judge(ctx, session, link, ep); err != nil {
			return err
		}
		next := ep.lastSeq()
		if moved, err := p.Store.AdvanceHistory(ctx, link.ID, cursor, next, false); err != nil || !moved {
			return err
		}
		cursor = &next
	}
	if read.highest > seqOf(cursor) {
		_, err = p.Store.AdvanceHistory(ctx, link.ID, cursor, read.highest, false)
	}
	return err
}

// judge summarises one closed episode and files it when it matters. An
// episode no model can judge, or whose answer was refused, is passed over.
// The cursor moves on, so a channel never waits for a model.
func (p *Puller) judge(ctx context.Context, session chatlinks.Session, link chatlinks.Link, ep episode) error {
	record, err := readRecord(ctx, p.Pool, link)
	if err != nil {
		return err
	}
	reply, err := summarise(ctx, p.Lane, link, record, ep.messages, false)
	switch {
	case errors.Is(err, errNoModel):
		p.Log.InfoContext(ctx, "chat episode passed over", "link", link.ID.String(), "first_seq", ep.firstSeq(), "reason", "no_model")
		return nil
	case errors.Is(err, ai.ErrOutputRejected):
		p.Log.WarnContext(ctx, "chat episode passed over", "link", link.ID.String(), "first_seq", ep.firstSeq(), "reason", "answer_rejected")
		return nil
	case err != nil:
		return err
	case !reply.Relevant:
		return nil
	}
	said := talkOf(ep.key(link.ChannelSlug), ep.url(session.Address, link.ChannelSlug), ep.messages)
	_, err = fileNote(principal.WithCorrelationID(ctx, ids.NewV7()), p.Activities, link, said, reply)
	return err
}

// answerFailure decides what one link's failure does to the pull. It answers
// nil to go on with the next link. A refused token parks the bot and a rate
// limit or a model deferral stops the pull until the next tick.
func (p *Puller) answerFailure(ctx context.Context, link chatlinks.Link, err error) error {
	var limited *connector.RateLimitedError
	switch {
	case errors.Is(err, dispact.ErrTokenRejected):
		return capture.StopPollingChannelConnection(ctx, p.Pool, link.ConnectionID,
			capture.ChannelPollStoppedByBadToken, "the chat refused the bot token")
	case errors.Is(err, dispact.ErrChannelGone), errors.Is(err, dispact.ErrNotMember):
		return p.Store.MarkChannelGone(ctx, link.ID)
	case errors.As(err, &limited):
		wait := limited.RetryAfter
		if wait <= 0 {
			wait = chatRateLimitWait
		}
		return river.JobSnooze(wait)
	case ai.IsDeferral(err):
		p.Log.InfoContext(ctx, "chat pull waits for the model", "link", link.ID.String())
		return errModelDeferred
	}
	p.Log.WarnContext(ctx, "chat pull failed for a link", "link", link.ID.String(), "err", err)
	return nil
}

// historyRead is what one pull read of a channel: the messages kept, oldest
// first, and the highest seq seen, kept or not.
type historyRead struct {
	kept    []dispact.Message
	highest int64
	ended   bool
}

// readHistory reads up to pages pages after the link's cursor.
func readHistory(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, pages int, keep func(dispact.Message) bool) (historyRead, error) {
	out := historyRead{highest: seqOf(link.HistoryCursor)}
	after := out.highest
	for range pages {
		page, err := chat.History(ctx, link.ChannelSlug, after, dispact.HistoryPageLimit)
		if err != nil {
			return historyRead{}, err
		}
		for _, m := range page.Messages {
			out.highest = max(out.highest, m.Seq)
			if keep(m) {
				out.kept = append(out.kept, m)
			}
		}
		if page.NextCursor == nil || len(page.Messages) == 0 {
			out.ended = true
			return out, nil
		}
		after = *page.NextCursor
	}
	return out, nil
}

func seqOf(cursor *int64) int64 {
	if cursor == nil {
		return 0
	}
	return *cursor
}
