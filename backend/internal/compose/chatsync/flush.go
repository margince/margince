// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// The chat_link_flush job: one link's pending items become one post. A post
// that lost an item to a narrowed audience is rewritten, and the card is kept
// current. Every activity line is read through sharedActivity again right
// before it is sent.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Window is how long events gather before they post together.
const Window = 2 * time.Minute

// chatRateLimitWait is how long a flush waits when the chat names no time.
const chatRateLimitWait = 30 * time.Second

// NewFlusher builds the flush of a worker role.
func NewFlusher(pool *pgxpool.Pool, chats chatlinks.Chats, store *chatlinks.Store, origin string, log *slog.Logger) *Flusher {
	return &Flusher{Pool: pool, Store: store, Chats: chats, Origin: origin, Log: log}
}

// Flusher sends one link's flush.
type Flusher struct {
	Pool   *pgxpool.Pool
	Store  *chatlinks.Store
	Chats  chatlinks.Chats
	Origin string
	Log    *slog.Logger
}

// Flush sends what the link has pending. ctx carries the job's workspace; the
// flush acts as the chat engine's system actor. A rate limit or more pending
// work answers a River snooze.
func (w *Flusher) Flush(ctx context.Context, linkID ids.UUID) error {
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	pending, err := w.Store.PendingFor(ctx, linkID)
	if errors.Is(err, chatlinks.ErrLinkNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if pending.Link.Status != chatlinks.StatusActive {
		return nil
	}
	session, err := w.Chats.Open(ctx)
	if errors.Is(err, chatlinks.ErrBotNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}
	if session.ConnectionID != pending.Link.ConnectionID {
		// The bot was disconnected and another connected: its team is not
		// the one this channel lives in, so the link pauses.
		return w.Store.MarkChannelGone(ctx, pending.Link.ID)
	}
	if err := w.send(ctx, session, pending); err != nil {
		return w.answerFailure(ctx, pending, err)
	}
	more, err := w.Store.HasPending(ctx, pending.Link.ID)
	if err != nil {
		return err
	}
	if more {
		return river.JobSnooze(Window)
	}
	return nil
}

// send keeps the card current, rewrites narrowed posts, and posts what is
// pending. The order puts the card before the first update.
func (w *Flusher) send(ctx context.Context, session chatlinks.Session, pending chatlinks.Pending) error {
	record, err := readRecord(ctx, w.Pool, pending.Link)
	if err != nil {
		return err
	}
	if err := w.refreshCard(ctx, session.Chat, pending.Link, record); err != nil {
		return err
	}
	for _, rewrite := range pending.Rewrites {
		lines, err := w.lines(ctx, rewrite.Items)
		if err != nil {
			return err
		}
		if err := session.Chat.Update(ctx, rewrite.MessageID, chatPostText(record, lines, w.Origin)); err != nil && !errors.Is(err, dispact.ErrChannelGone) {
			return err
		}
		if err := w.Store.MarkRewritten(ctx, pending.Link.ID, rewrite.MessageID); err != nil {
			return err
		}
	}
	return w.post(ctx, session.Chat, pending, record)
}

func (w *Flusher) post(ctx context.Context, chat chatlinks.Chat, pending chatlinks.Pending, record chatRecord) error {
	lines, err := w.lines(ctx, pending.Items)
	if err != nil || len(pending.Items) == 0 {
		return err
	}
	carried := make([]ids.UUID, 0, len(pending.Items))
	for _, item := range pending.Items {
		carried = append(carried, item.ID)
	}
	// The first item names the post, so a retry after a lost answer reuses the
	// same client_msg_id and the chat posts it once.
	postID := pending.Items[0].ID
	if len(lines) == 0 {
		return w.Store.MarkPosted(ctx, pending.Link.ID, postID, "", carried)
	}
	posted, err := chat.Post(ctx, pending.Link.ChannelSlug, chatPostText(record, lines, w.Origin), postID.String(), "")
	if err != nil {
		return err
	}
	return w.Store.MarkPosted(ctx, pending.Link.ID, postID, posted.ID, carried)
}

// lines renders each item, reading an activity through sharedActivity. An
// activity the workspace may no longer read is withdrawn instead of shown.
func (w *Flusher) lines(ctx context.Context, items []chatlinks.Item) ([]string, error) {
	out := make([]string, 0, len(items))
	for _, item := range items {
		var shared sharedText
		if item.ActivityID != nil {
			err := database.WithWorkspaceTx(ctx, w.Pool, func(tx pgx.Tx) error {
				var err error
				shared, err = sharedActivity(ctx, tx, *item.ActivityID)
				return err
			})
			if err != nil {
				return nil, err
			}
			if !shared.ok {
				if err := w.Store.Withdraw(ctx, *item.ActivityID); err != nil {
					return nil, err
				}
				continue
			}
		}
		if line := chatItemLine(item, shared); line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// refreshCard posts the card once and edits it only when a fact it shows
// changed. A card deleted in the chat is posted again.
func (w *Flusher) refreshCard(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, record chatRecord) error {
	text := chatCardText(record, w.Origin)
	fingerprint := chatFingerprint(text)
	if link.CardMessageID != "" && fingerprint == link.CardFingerprint {
		return nil
	}
	if link.CardMessageID != "" {
		err := chat.Update(ctx, link.CardMessageID, text)
		if err == nil {
			return w.Store.SetCard(ctx, link.ID, link.CardMessageID, fingerprint)
		}
		if !errors.Is(err, dispact.ErrChannelGone) {
			return err
		}
	}
	posted, err := chat.Post(ctx, link.ChannelSlug, text, "card:"+link.ID.String()+":"+fingerprint, "")
	if err != nil {
		return err
	}
	return w.Store.SetCard(ctx, link.ID, posted.ID, fingerprint)
}

// answerFailure decides what a chat failure does. A refused token parks the
// bot until an admin replaces it. A gone channel pauses the link. A rate limit
// waits, and anything else retries on River's ladder.
func (w *Flusher) answerFailure(ctx context.Context, pending chatlinks.Pending, err error) error {
	var limited *connector.RateLimitedError
	switch {
	case errors.Is(err, dispact.ErrTokenRejected):
		return capture.StopPollingChannelConnection(ctx, w.Pool, pending.Link.ConnectionID,
			capture.ChannelPollStoppedByBadToken, "the chat refused the bot token")
	case errors.Is(err, dispact.ErrChannelGone), errors.Is(err, dispact.ErrNotMember):
		return w.Store.MarkChannelGone(ctx, pending.Link.ID)
	case errors.As(err, &limited):
		wait := limited.RetryAfter
		if wait <= 0 {
			wait = chatRateLimitWait
		}
		return river.JobSnooze(wait)
	}
	carried := make([]ids.UUID, 0, len(pending.Items))
	for _, item := range pending.Items {
		carried = append(carried, item.ID)
	}
	if recordErr := w.Store.RecordFailure(ctx, pending.Link.ID, carried, chatFailureClass(err)); recordErr != nil {
		w.Log.WarnContext(ctx, "chat flush could not record its failure", "link", pending.Link.ID.String(), "err", recordErr)
	}
	return err
}

// chatFailureClass names a failure without the chat's own words.
func chatFailureClass(err error) string {
	if errors.Is(err, dispact.ErrUnreachable) {
		return "chat unreachable"
	}
	if errors.Is(err, dispact.ErrRequestRejected) {
		return "chat refused the post"
	}
	return "post failed"
}

// FlushJob is the args of the flush job, which compose declares.
type FlushJob interface {
	jobs.WorkspaceScoped
	Link() ids.UUID
}

// Worker runs the flush job. Bind is compose's one workspace binder, so the
// args' own WorkspaceID is the binding.
type Worker[T FlushJob] struct {
	Bind    func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Flusher *Flusher
}

// Work implements River's worker for the flush.
func (w *Worker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	ctx, err := w.Bind(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, w.Flusher.Flush(ctx, job.Args.Link()))
}
