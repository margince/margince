// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func systemTextInsert(ctx context.Context, in LogActivityInput, args []any) (string, string, []any) {
	message := systemtext.ForWrite(ctx, in.SystemText, deref(in.Subject), deref(in.Body))
	if message == nil {
		return "", "", args
	}
	args = append(args, message)
	return ", system_text", fmt.Sprintf(", $%d", len(args)), args
}

func activitySystemDisplay(ctx context.Context, message *systemtext.Message, subject, body *string, version int64) *crmcontracts.SystemDisplay {
	if message == nil {
		return nil
	}
	locale := systemtext.Locale(ctx)
	title, text := message.Displays(deref(subject), deref(body), locale)
	parts := message.GeneratedParts(deref(subject), deref(body))
	if subject == nil {
		title = nil
	}
	if body == nil {
		text = nil
	}
	if title == nil && text == nil && len(parts) == 0 {
		return nil
	}
	out := &crmcontracts.SystemDisplay{Locale: crmcontracts.SystemDisplayLocale(locale), Subject: title, Body: text, SourceVersion: &version}
	if len(parts) > 0 {
		selected := make([]crmcontracts.SystemDisplayGeneratedParts, len(parts))
		for i, part := range parts {
			selected[i] = crmcontracts.SystemDisplayGeneratedParts(part)
		}
		out.GeneratedParts = &selected
	}
	return out
}

func readSystemTextForMutation(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (*systemtext.Message, error) {
	var message *systemtext.Message
	args := []any{id}
	err := tx.QueryRow(ctx, fmt.Sprintf("SELECT system_text FROM activity WHERE id=$%d", len(args)), args...).Scan(&message)
	return message, err
}

func outboundMessageText(original string, html bool) *systemtext.Message {
	body := systemtext.Prose(systemtext.Quote, "")
	if html {
		body = systemtext.Sentence("activity.source_body", systemtext.Excerpt(0, utf8.RuneCountInString(original)))
		body.Layout = "paragraphs"
		body.Following = []systemtext.Component{*systemtext.Sentence("activity.html_alternative_marker", systemtext.MessageArgument("activity.html_alternative"))}
	}
	return &systemtext.Message{Subject: systemtext.Prose(systemtext.Quote, ""), Body: body}
}
