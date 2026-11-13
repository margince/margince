// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// Summaries: a stretch of channel talk is read again from the chat. The
// chat_summary task's episode site judges it, and it is filed as one note.
// The messages live only in memory for the call; the note keeps the summary.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// errNoModel is an installation with no chat_summary lane: nothing is filed.
var errNoModel = errors.New("chat summary: no model is bound to the chat_summary task")

// summarise asks the episode site about messages of the link's channel. A
// lane that cannot answer at all reads as errNoModel.
func summarise(ctx context.Context, lane ai.Completer, link chatlinks.Link, record chatRecord, messages []dispact.Message, chosen bool) (chatsummary.Reply, error) {
	if lane == nil {
		return chatsummary.Reply{}, errNoModel
	}
	in := chatsummary.Input{Record: record.kind + " " + record.name, Channel: link.ChannelName, Chosen: chosen}
	if record.stage != "" {
		in.Facts = append(in.Facts, "Stage: "+record.stage)
	}
	for _, m := range messages {
		in.Messages = append(in.Messages, chatsummary.Message{Author: authorOf(m), At: m.PostedAt, Text: m.Text})
	}
	about := &model.Subjects{}
	if link.DealID != nil {
		about.Deals = append(about.Deals, *link.DealID)
	}
	if link.ProjectID != nil {
		about.Projects = append(about.Projects, *link.ProjectID)
	}
	check := func(raw string) error {
		reply, err := chatsummary.Parse(raw)
		if err == nil {
			err = chatsummary.Check(reply)
		}
		if err == nil && chosen && !reply.Relevant {
			err = errors.New("a colleague chose this talk, so summarise it")
		}
		return err
	}
	ctx = ai.WithSubject(ctx, recordRef(record), record.name)
	res, err := ai.Ask(ctx, lane, chatsummary.Request(in, about), check)
	if errors.Is(err, ai.ErrUnconfiguredModel) {
		return chatsummary.Reply{}, errNoModel
	}
	if err != nil {
		return chatsummary.Reply{}, err
	}
	// Judged again: a lane that cannot re-ask hands back the reply unchecked.
	if err := check(res.Text); err != nil {
		return chatsummary.Reply{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	return chatsummary.Parse(res.Text)
}

func authorOf(m dispact.Message) string {
	if m.UserName != "" {
		return m.UserName
	}
	if m.UserEmail != "" {
		return m.UserEmail
	}
	return "someone"
}

// talk is what a note says about the messages it summarises. It holds no
// message text.
type talk struct {
	key     string
	count   int
	started time.Time
	ended   time.Time
	url     string
}

// talkOf is what a note says about messages, oldest first.
func talkOf(key, url string, messages []dispact.Message) talk {
	return talk{key: key, url: url, count: len(messages),
		started: messages[0].PostedAt, ended: messages[len(messages)-1].PostedAt}
}

func recordRef(record chatRecord) ids.Ref { return ids.Ref{Type: record.kind, ID: record.id} }

// fileNote files the summary as one note on the link's records, keyed on the
// talk so a replay returns the note already there. ctx carries the actor: the
// chat engine for a pull, the member for a message they chose.
func fileNote(ctx context.Context, acts *activities.Store, link chatlinks.Link, said talk, reply chatsummary.Reply) (crmcontracts.Activity, error) {
	system := activities.ChatSummarySource
	raw := map[string]any{
		"message_count": said.count,
		"ended_at":      said.ended.UTC().Format(time.RFC3339),
		"url":           said.url,
	}
	var links []activities.ActivityLinkInput
	if link.DealID != nil {
		links = append(links, activities.ActivityLinkInput{EntityType: chatlinks.RecordDeal, EntityID: *link.DealID})
	}
	if link.ProjectID != nil {
		links = append(links, activities.ActivityLinkInput{EntityType: chatlinks.RecordProject, EntityID: *link.ProjectID})
	}
	body := noteBody(reply)
	// Both the title and the summary are the model's words, in the talk's language.
	written := textlang.Detect(reply.Summary)
	note, _, err := acts.LogActivity(ctx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote),
		SystemText: &systemtext.Message{Version: 1,
			Subject: systemtext.Prose(systemtext.Generated, written), Body: systemtext.Prose(systemtext.Generated, written)},
		Subject:      &reply.Title,
		Body:         &body,
		OccurredAt:   &said.started,
		SourceSystem: &system,
		SourceID:     &said.key,
		Raw:          &raw,
		Links:        links,
		Source:       "chat_links",
		Origin:       activities.OriginAgent,
	})
	if err != nil {
		return crmcontracts.Activity{}, fmt.Errorf("chat summary: filing the note for %s: %w", said.key, err)
	}
	return note, nil
}

// noteHeadings name the lists under a summary, in the language it is written in.
var noteHeadings = map[textlang.Lang][2]string{
	textlang.English:    {"Decisions", "Open questions"},
	textlang.German:     {"Entscheidungen", "Offene Fragen"},
	textlang.Vietnamese: {"Quyết định", "Câu hỏi còn mở"},
}

// noteBody is the summary, then the decisions and the open questions.
func noteBody(reply chatsummary.Reply) string {
	headings, ok := noteHeadings[textlang.Detect(reply.Summary)]
	if !ok {
		headings = noteHeadings[textlang.English]
	}
	var body strings.Builder
	body.WriteString(reply.Summary)
	for i, items := range [][]string{reply.Decisions, reply.OpenQuestions} {
		if len(items) == 0 {
			continue
		}
		body.WriteString("\n\n" + headings[i] + ":")
		for _, item := range items {
			body.WriteString("\n- " + item)
		}
	}
	return body.String()
}
