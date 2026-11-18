// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// cardRefusal is the reason a card cannot be imported because of what it
// states, or "" when nothing in it is refused. A refused card is reported by
// name and the rest of the file still imports.
func cardRefusal(entry VCardEntry) string {
	if len(entry.Unreadable) > 0 {
		return fmt.Sprintf("the %s on this card is not readable text; save the file as UTF-8 and import it again",
			vcardPropertyLabel(entry.Unreadable[0]))
	}
	for _, email := range entry.Emails {
		if _, err := values.ParseEmail(email.Value); err != nil {
			return valueRefusal("email address", email.Value, err)
		}
	}
	for _, phone := range entry.Phones {
		if _, err := values.ParsePhone(phone.Value); err != nil {
			return valueRefusal("phone number", phone.Value, err)
		}
	}
	return ""
}

// valueRefusal states which value of the card was refused and why. Any other
// error is not about the card's content and is returned as "" by the caller's
// own check.
func valueRefusal(what, value string, err error) string {
	if parse, ok := errors.AsType[*values.ParseError](err); ok {
		return fmt.Sprintf("the %s %q is not valid: %s", what, value, parse.Message)
	}
	return fmt.Sprintf("the %s %q is not valid", what, value)
}

// cardParseRefusal is the reason for a refusal that only the write could see,
// such as a name or title the record cannot hold.
func cardParseRefusal(err error) (string, bool) {
	parse, ok := errors.AsType[*values.ParseError](err)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("the %s is not valid: %s", parse.Field, parse.Message), true
}

func vcardPropertyLabel(property string) string {
	switch strings.ToUpper(property) {
	case "FN", "N":
		return "name"
	case "ORG":
		return "company"
	case "TITLE":
		return "job title"
	case "EMAIL":
		return "email address"
	case "TEL":
		return "phone number"
	case "ADR":
		return "address"
	}
	return "URL"
}
