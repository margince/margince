// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oauthflow

import (
	"errors"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ErrMailboxAddress marks a shared mailbox address that cannot name one
// mailbox. The address goes into a request path or a signed subject, so
// anything that could change either is refused.
var ErrMailboxAddress = errors.New("a shared mailbox needs one plain address")

// MailboxAddress returns the lowercased address of a shared mailbox, or
// ErrMailboxAddress. It accepts one bare address with no display name.
func MailboxAddress(raw string) (string, error) {
	if strings.ContainsAny(raw, `/\%?#`) || strings.ContainsFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return "", ErrMailboxAddress
	}
	addr, err := values.ParseEmail(raw)
	if err != nil {
		return "", ErrMailboxAddress
	}
	return addr.String(), nil
}
