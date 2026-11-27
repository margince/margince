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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Filer files a chosen channel message on its record.
type Filer struct {
	Reconfirm  ReconfirmationWriter
	Pool       *pgxpool.Pool
	Store      *chatlinks.Store
	Chats      chatlinks.Chats
	Activities *activities.Store
	// Contacts resolves a chat author's address to a contact, so a restricted or
	// erased subject's message is not filed.
	Contacts      *contacts.Store
	Lane          ai.Completer
	Reauthorize   func(context.Context) (context.Context, error)
	PreviewOrigin func() string
}

// restricted answers which of the author addresses the summary must withhold.
func (f *Filer) restricted(ctx context.Context, emails []string) (map[string]bool, error) {
	return lookupRestricted(ctx, f.Pool, f.Contacts, emails)
}

// FileMessage reads the message ref names in the record's channel, or its
// whole thread, and files one summary note. A ref is a seq, a #slug:seq, the
// chat's <m:#slug:seq|…> mention, or a message link.
func (f *Filer) FileMessage(ctx context.Context, kind string, id ids.UUID, ref string, thread bool) (crmcontracts.Activity, error) {
	var err error
	if f.Reauthorize != nil {
		ctx, err = f.Reauthorize(ctx)
		if err != nil {
			return crmcontracts.Activity{}, err
		}
	}
	link, err := f.Store.FilingLink(ctx, kind, id)
	if err != nil {
		return crmcontracts.Activity{}, err
	}

	session, err := chatlinks.OpenProvider(ctx, f.Chats, link.Provider)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if session.ConnectionID != link.ConnectionID {
		return crmcontracts.Activity{}, chatlinks.ErrBotNotConnected
	}
	if _, err := chatlinks.LinkedChannel(ctx, session.Chat, link); err != nil {
		return crmcontracts.Activity{}, err
	}
	if reader, ok := session.Chat.(chatwire.RootReader); ok {
		return f.fileNativeMessage(ctx, session, link, reader, ref, thread)
	}
	seq, ok := dispact.MessageSequence(ref, link.ChannelSlug, thread)
	if !ok {
		return crmcontracts.Activity{}, chatlinks.ErrMessageNotFound
	}
	messages, err := chosenMessages(ctx, session.Chat, link, seq, thread)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	reply, err := f.summariseChosen(ctx, link, messages)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if f.Reauthorize != nil {
		ctx, err = f.Reauthorize(ctx)
		if err != nil {
			return crmcontracts.Activity{}, err
		}
	}
	key := link.ChannelSlug + ":" + strconv.FormatInt(seq, 10) + ":message"
	if thread {
		key = link.ChannelSlug + ":" + strconv.FormatInt(seq, 10) + ":thread"
	}
	url := chatlinks.ChannelURL(session.Address, link.ChannelSlug)
	return fileNote(ctx, filing{reconfirm: f.Reconfirm, pool: f.Pool, store: f.Store, acts: f.Activities, link: link}, talkOf(key, url, messages), reply)
}

// chosenMessages reads the message at seq and, for a thread, the replies in
// its thread that follow it, all posted since the link was made.
func chosenMessages(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, seq int64, thread bool) ([]chatwire.Message, error) {
	from := seq - 1
	link.HistoryCursor = &from
	pages := 1
	if thread {
		pages = pullPages
	}
	var anchor *chatwire.Message
	read, err := readHistory(ctx, chat, link, pages, func(m chatwire.Message) bool {
		if m.Seq == seq {
			anchor = &m
			return true
		}
		return thread && anchor != nil && judged(m) && postedSinceLink(m, link) && m.ThreadTS != "" &&
			(m.ThreadTS == anchor.ThreadTS || m.ThreadTS == anchor.TS)
	})
	if err != nil {
		return nil, chatlinks.ChatFailure(err)
	}
	// Talk from before the link answers as missing, so no one learns it is there.
	if anchor == nil || !judged(*anchor) || !postedSinceLink(*anchor, link) {
		return nil, chatlinks.ErrMessageNotFound
	}
	// A thread read only in part would be filed under its permanent key.
	if thread && !read.ended {
		return nil, chatlinks.ErrThreadTooLong
	}
	return read.kept, nil
}

func (f *Filer) summariseChosen(ctx context.Context, link chatlinks.Link, messages []chatwire.Message) (chatsummary.Reply, error) {
	record, err := readRecord(ctx, f.Pool, link)
	if err != nil {
		return chatsummary.Reply{}, err
	}
	reply, err := summariser{lane: f.Lane, pool: f.Pool, restricted: f.restricted}.summarise(ctx, link, record, messages, true)
	// errUnmatchableAuthor is a restricted or unidentifiable author: like a
	// missing model, it files nothing and answers that no summary is available.
	if errors.Is(err, errNoModel) || errors.Is(err, errUnmatchableAuthor) {
		return chatsummary.Reply{}, chatlinks.ErrNoSummaryModel
	}
	if err != nil {
		return chatsummary.Reply{}, fmt.Errorf("%w: %w", chatlinks.ErrSummaryFailed, err)
	}
	return reply, nil
}
