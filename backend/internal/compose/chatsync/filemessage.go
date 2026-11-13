// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// file_channel_message: a member picks one message or its thread, and files a
// summary of it on the record. The agent in the chat may do it for them. The
// message is read from the chat and summarised like an episode, and the note
// is filed as the member.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Filer files a chosen channel message on its record.
type Filer struct {
	Pool       *pgxpool.Pool
	Store      *chatlinks.Store
	Chats      chatlinks.Chats
	Activities *activities.Store
	Lane       ai.Completer
}

// FileMessage reads the message ref names in the record's channel, or its
// whole thread, and files one summary note. A ref is a seq, a #slug:seq, the
// chat's <m:#slug:seq|…> mention, or a message link.
func (f *Filer) FileMessage(ctx context.Context, kind string, id ids.UUID, ref string, thread bool) (crmcontracts.Activity, error) {
	link, err := f.Store.FilingLink(ctx, kind, id)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	seq, ok := messageSeq(ref, link.ChannelSlug)
	if !ok {
		return crmcontracts.Activity{}, chatlinks.ErrMessageNotFound
	}
	session, err := f.Chats.Open(ctx)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if session.ConnectionID != link.ConnectionID {
		return crmcontracts.Activity{}, chatlinks.ErrBotNotConnected
	}
	messages, err := chosenMessages(ctx, session.Chat, link, seq, thread)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	record, err := readRecord(ctx, f.Pool, link)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	reply, err := summarise(ctx, f.Lane, link, record, messages, true)
	if errors.Is(err, errNoModel) {
		return crmcontracts.Activity{}, chatlinks.ErrNoSummaryModel
	}
	if err != nil {
		return crmcontracts.Activity{}, fmt.Errorf("%w: %w", chatlinks.ErrSummaryFailed, err)
	}
	key := link.ChannelSlug + ":" + strconv.FormatInt(seq, 10) + ":message"
	if thread {
		key = link.ChannelSlug + ":" + strconv.FormatInt(seq, 10) + ":thread"
	}
	url := chatlinks.ChannelURL(session.Address, link.ChannelSlug) + "?seq=" + strconv.FormatInt(seq, 10)
	return fileNote(ctx, f.Activities, link, talkOf(key, url, messages), reply)
}

// chosenMessages reads the message at seq and, for a thread, the replies in
// its thread that follow it.
func chosenMessages(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, seq int64, thread bool) ([]dispact.Message, error) {
	from := seq - 1
	link.HistoryCursor = &from
	pages := 1
	if thread {
		pages = pullPages
	}
	var anchor *dispact.Message
	read, err := readHistory(ctx, chat, link, pages, func(m dispact.Message) bool {
		if m.Seq == seq {
			anchor = &m
			return true
		}
		return thread && anchor != nil && judged(m) && m.ThreadTS != "" &&
			(m.ThreadTS == anchor.ThreadTS || m.ThreadTS == anchor.ID)
	})
	if err != nil {
		return nil, chatlinks.ChatFailure(err)
	}
	if anchor == nil || !judged(*anchor) {
		return nil, chatlinks.ErrMessageNotFound
	}
	return read.kept, nil
}

// messageSeq reads the seq out of a message reference, refusing one that
// names another channel.
func messageSeq(ref, slug string) (int64, bool) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "<m:") {
		ref, _, _ = strings.Cut(strings.TrimSuffix(strings.TrimPrefix(ref, "<m:"), ">"), "|")
	}
	if before, after, found := strings.Cut(ref, "?seq="); found {
		if channelSlugOfLink(before) != slug {
			return 0, false
		}
		ref = after
	}
	if channel, number, found := strings.Cut(ref, ":"); found {
		if strings.ToLower(strings.TrimPrefix(channel, "#")) != slug {
			return 0, false
		}
		// The thread-reply form names the thread's first message, then the reply.
		ref, _, _ = strings.Cut(number, ":")
	}
	seq, err := strconv.ParseInt(ref, 10, 64)
	return seq, err == nil && seq > 0
}

// channelSlugOfLink is the last path segment of a channel link.
func channelSlugOfLink(link string) string {
	link = strings.TrimRight(link, "/")
	return strings.ToLower(link[strings.LastIndex(link, "/")+1:])
}
