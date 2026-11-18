// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// cardRefusal is the reason a card cannot be imported because of what it
// states, or "" when nothing in it is refused. A refused card is reported by
// name and the rest of the file still imports.
func cardRefusal(entry VCardEntry) string {
	if len(entry.Unreadable) > 0 {
		return fmt.Sprintf("the %s on this card is not readable text; save the file as UTF-8 and import it again",
			vcardStoredProperties[entry.Unreadable[0]])
	}
	for _, email := range entry.Emails {
		if _, err := values.ParseEmail(email.Value); err != nil {
			return valueRefusal("the email address", err)
		}
	}
	for _, phone := range entry.Phones {
		if _, err := values.ParsePhone(phone.Value); err != nil {
			return valueRefusal("the phone number", err)
		}
	}
	return ""
}

// valueRefusal states which kind of value on the card was refused and why. It
// never repeats the value: the reason reaches logs, and an address or number is
// personal data there.
func valueRefusal(what string, err error) string {
	if parse, ok := errors.AsType[*values.ParseError](err); ok {
		return fmt.Sprintf("%s on this card is not valid: %s", what, parse.Message)
	}
	return fmt.Sprintf("%s on this card is not valid", what)
}

// cardParseRefusal is the reason for a refusal that only the write could see,
// such as a country code or a name the record cannot hold. It covers the
// errors a module declares as bad input; any other error is an infrastructure
// failure and still aborts the file.
func cardParseRefusal(err error) (string, bool) {
	if parse, ok := errors.AsType[*values.ParseError](err); ok {
		return fmt.Sprintf("the %s is not valid: %s", parse.Field, parse.Message), true
	}
	if fault, ok := errors.AsType[apperrors.FieldFault](err); ok {
		field, _, message := fault.FieldFault()
		return fmt.Sprintf("the %s is not valid: %s", field, message), true
	}
	if faults, ok := errors.AsType[apperrors.FieldFaults](err); ok {
		if first := faults.FieldFaults(); len(first) > 0 {
			return fmt.Sprintf("the %s is not valid: %s", first[0].Field, first[0].Message), true
		}
	}
	if errors.Is(err, apperrors.ErrInvalidArgument) {
		return "a value on this card is not valid", true
	}
	return "", false
}
