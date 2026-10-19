// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// SendLocalized uses markup when supported, preserving text-only relay compatibility.
func SendLocalized(ctx context.Context, relay Mailer, to, subject, text, language string) error {
	if alternative, ok := relay.(AlternativeSender); ok {
		markup := emailview.Render(LocalizedMessage(subject, text, language))
		return alternative.SendAlternative(ctx, to, subject, text, markup, "")
	}
	return relay.Send(ctx, to, subject, text)
}

// LocalizedMessage supplies the shared semantic headings for product messages.
func LocalizedMessage(subject, text, language string) emailview.Message {
	words := mailcopy.For(language)
	title := subject
	first, rest, found := strings.Cut(text, "\n\n")
	if found && (strings.HasPrefix(first, words.MorningHeading) || strings.HasPrefix(first, words.WeeklyHeading)) {
		title, text = first, rest
	}
	return emailview.Message{Title: title, Preheader: words.Preview(subject), Text: text, Language: string(words.Language), Sections: []string{words.MailPersonal, words.MailTeam, words.WeeklyResults}}
}
