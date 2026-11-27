// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

var errIncompleteActivityBody = errors.New("chat: a complete source body is unavailable")

func (w *Flusher) outboundSummary(ctx context.Context, link chatlinks.Link, record chatRecord, item chatlinks.Item, source sharedText) (string, error) {
	if source.kind == "email" && !source.complete {
		return "", errIncompleteActivityBody
	}
	body := strings.TrimSpace(source.body)
	limit := chatsummary.BriefSummaryRunes
	if link.MessageDetail == chatlinks.MessageDetailFull {
		limit = chatsummary.MaxSummaryRunes
	}
	if utf8.RuneCountInString(body) <= limit {
		return body, nil
	}
	if w.Lane == nil {
		return "", errNoModel
	}
	in := chatsummary.Input{Record: record.kind + " " + record.name, Channel: link.ChannelName, FromCRM: true, Brief: link.MessageDetail == chatlinks.MessageDetailBrief, Messages: []chatsummary.Message{{ID: item.ActivityID.String(), Author: "CRM " + source.kind + " record", At: item.CreatedAt, Text: source.subject + "\n\n" + body}}}
	request, err := chatsummary.Request(in, model.AboutActivities(*item.ActivityID))
	if err != nil {
		return "", chatlinks.ErrThreadTooLong
	}
	check := func(raw string) error { return checkSummaryReply(raw, in) }
	ctx = ai.WithSubject(ctx, ids.Ref{Type: activityKind, ID: *item.ActivityID}, source.subject)
	response, err := ai.Ask(ctx, w.Lane, request, check)
	if errors.Is(err, ai.ErrUnconfiguredModel) {
		return "", errNoModel
	}
	if err != nil {
		return "", err
	}
	if err := check(response.Text); err != nil {
		return "", err
	}
	reply, err := chatsummary.Parse(response.Text)
	if err != nil {
		return "", err
	}
	parts := []string{reply.Summary}
	if len(reply.Decisions) > 0 {
		parts = append(parts, "Decisions: "+strings.Join(reply.Decisions, "; "))
	}
	if len(reply.OpenQuestions) > 0 {
		parts = append(parts, "Open questions: "+strings.Join(reply.OpenQuestions, "; "))
	}
	return strings.Join(parts, "\n"), nil
}

type outboundLines struct {
	text     []string
	carried  []ids.UUID
	sources  map[ids.UUID]sharedText
	audience outboundAudience
}

type outboundLineState uint8

const (
	lineDeferred outboundLineState = iota
	lineReady
	lineWithdrawn
)

type outboundLine struct {
	text   string
	source sharedText
	state  outboundLineState
}

func (w *Flusher) nativeLines(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, record chatRecord, items []chatlinks.Item, budget int) (outboundLines, error) {
	audience, err := w.outboundAudience(ctx, chat, link)
	if err != nil {
		return outboundLines{}, err
	}
	out := outboundLines{audience: audience, sources: map[ids.UUID]sharedText{}}
	length := 0
	for _, item := range items {
		line, err := w.nativeLine(ctx, link, record, item, audience, budget)
		if err != nil {
			return outboundLines{}, err
		}
		switch line.state {
		case lineWithdrawn:
			out.carried = append(out.carried, item.ID)
			continue
		case lineDeferred:
			continue
		}
		if budget > 0 && length+len(line.text) > budget && len(out.text) > 0 {
			break
		}
		length += len(line.text)
		out.text = append(out.text, line.text)
		out.carried = append(out.carried, item.ID)
		if item.ActivityID != nil {
			out.sources[*item.ActivityID] = line.source
		}
	}
	if err := w.recheckNativeSources(ctx, chat, link, out); err != nil {
		return outboundLines{}, err
	}
	return out, nil
}

func (w *Flusher) nativeLine(ctx context.Context, link chatlinks.Link, record chatRecord, item chatlinks.Item, audience outboundAudience, budget int) (outboundLine, error) {
	var source sharedText
	if item.ActivityID != nil {
		var err error
		source, err = w.outboundSource(ctx, *item.ActivityID, audience)
		if err != nil {
			return outboundLine{}, err
		}
		if !source.ok {
			if err := w.withdrawUnreadableSource(ctx, *item.ActivityID); err != nil {
				return outboundLine{}, err
			}
			return outboundLine{state: lineWithdrawn}, nil
		}
	}
	line := chatItemLine(item, source)
	if item.ActivityID == nil {
		return outboundLine{text: line, state: lineReady}, nil
	}
	summary, err := w.outboundSummary(ctx, link, record, item, source)
	if err != nil {
		return outboundLine{state: lineDeferred}, w.deferSummaryFailure(ctx, link, item.ID, err, budget)
	}
	if summary != "" {
		if link.Provider != chatlinks.ProviderTeams {
			summary = chatSafe(summary)
		}
		line += "\n" + summary
	}
	return outboundLine{text: line, source: source, state: lineReady}, nil
}

func (w *Flusher) deferSummaryFailure(ctx context.Context, link chatlinks.Link, item ids.UUID, failure error, budget int) error {
	reason, global := nativeRootFailure(failure)
	if errors.Is(failure, errIncompleteActivityBody) {
		reason, global = "source_incomplete", false
	}
	if budget <= 0 || global || w.previewOnly {
		return failure
	}
	return w.Store.DeferPostItem(ctx, link, item, reason, 5*time.Minute)
}

func (w *Flusher) withdrawUnreadableSource(ctx context.Context, activity ids.UUID) error {
	if w.previewOnly {
		return nil
	}
	return w.Store.Withdraw(ctx, activity)
}

func (w *Flusher) recheckNativeSources(ctx context.Context, chat chatlinks.Chat, link chatlinks.Link, prepared outboundLines) error {
	current, err := w.outboundAudience(ctx, chat, link)
	if err != nil {
		return err
	}
	if current.key != prepared.audience.key {
		return apperrors.ErrConflict
	}
	for id, prior := range prepared.sources {
		live, err := w.outboundSource(ctx, id, current)
		if err != nil {
			return err
		}
		if live != prior {
			return apperrors.ErrConflict
		}
	}
	return nil
}
