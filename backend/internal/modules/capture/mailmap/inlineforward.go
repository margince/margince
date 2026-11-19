// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	netmail "net/mail"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// inlineForward reads the original an inline forward quotes, by the header
// block a mail program wrote above it (mailquote.FirstQuotedHeader). nil when
// there is none, or its From names no address.
func inlineForward(text string) *connector.ForwardedOriginal {
	header, ok := mailquote.FirstQuotedHeader(strings.Split(text, "\n"))
	if !ok {
		return nil
	}
	address, err := netmail.ParseAddress(mailtoAsAngle(header.From))
	if err != nil {
		return nil
	}
	return &connector.ForwardedOriginal{
		From:        strings.ToLower(strings.TrimSpace(address.Address)),
		DisplayName: address.Name,
		Subject:     header.Subject,
		Date:        header.Date,
		Inline:      true,
	}
}

// mailtoAsAngle turns Outlook's `Name [mailto:a@b.example]` into the
// `Name <a@b.example>` an address parser reads.
func mailtoAsAngle(value string) string {
	start := strings.Index(strings.ToLower(value), "[mailto:")
	if start < 0 || !strings.HasSuffix(value, "]") {
		return value
	}
	return value[:start] + "<" + value[start+len("[mailto:"):len(value)-1] + ">"
}
