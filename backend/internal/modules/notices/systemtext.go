// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func noticeSystemDisplay(ctx context.Context, message *systemtext.Message, subject, body string) *crmcontracts.SystemDisplay {
	if message == nil {
		return nil
	}
	locale := systemtext.Locale(ctx)
	title, text := message.Displays(subject, body, locale)
	if title == nil && text == nil {
		return nil
	}
	return &crmcontracts.SystemDisplay{Locale: crmcontracts.SystemDisplayLocale(locale), Subject: title, Body: text}
}

// InLanguage renders an authorized recipient’s copy without changing stored text.
func (n Notice) InLanguage(locale textlang.Lang) Notice {
	subject, body := n.SystemText.Displays(n.Subject, n.Body, locale)
	if subject != nil {
		n.Subject = *subject
	}
	if body != nil {
		n.Body = *body
	}
	return n
}
