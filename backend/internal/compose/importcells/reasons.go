// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importcells

import (
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Phone is a phone cell in the spelling every store holds, read by the parser
// the stores write with.
func Phone(text string) (string, migration.Reason) {
	phone, err := values.ParsePhone(text)
	if err != nil {
		return "", parseReason(text, err, true)
	}
	return phone.String(), migration.Reason{}
}

// LinkedIn is a LinkedIn cell in the spelling the object's store holds. A
// company takes a company page; a contact and a lead take a profile.
func LinkedIn(object, text string) (string, migration.Reason) {
	normalize := contacts.NormalizeContactLinkedInURL
	if object == migration.ObjectCompany {
		normalize = contacts.NormalizeCompanyLinkedInURL
	}
	stored, err := normalize(text)
	if err != nil {
		return "", parseReason(text, err, false)
	}
	return stored, migration.Reason{}
}

func parseReason(text string, err error, phone bool) migration.Reason {
	parseErr, ok := errors.AsType[*values.ParseError](err)
	if !ok {
		return migration.Why(migration.ReasonValueUnreadable, "value", text)
	}
	if phone {
		return phoneReason(text, parseErr)
	}
	return migration.Why(migration.ReasonValueRefused, "value", text, "detail", parseErr.Message)
}

// phoneReason asks for the country code only of a cell that holds a number.
// Text with more in it than digits, a plus and separators is no phone number.
func phoneReason(text string, parseErr *values.ParseError) migration.Reason {
	const digits = "0123456789"
	numeric := strings.ContainsAny(text, digits) &&
		strings.Trim(text, digits+"+"+strings.Join(values.PhoneSeparators(), "")) == ""
	switch {
	case !numeric:
		return migration.Why(migration.ReasonNotAPhone, "value", text)
	case parseErr.Code == "phone_needs_country_code":
		return migration.Why(migration.ReasonPhoneNeedsCountry, "value", text)
	}
	return migration.Why(migration.ReasonValueRefused, "value", text, "detail", parseErr.Message)
}

// AmountReason says what is wrong with text as an amount in currency: its
// sign, its decimal places, or its form. The example is one the currency takes.
func AmountReason(text, currency string) migration.Reason {
	digits := values.MinorUnitDigits(currency)
	unsigned, negative := strings.CutPrefix(text, "-")
	scale := values.MinorUnitScale(currency)
	example := values.MajorUnits(12000*scale+scale/2, currency)
	switch {
	case negative && values.PlainDecimal(unsigned, len(unsigned), len(unsigned)):
		return migration.Why(migration.ReasonAmountNegative, "value", text)
	case values.PlainDecimal(text, len(text), len(text)) && !values.PlainDecimal(text, len(text), digits):
		return migration.Why(migration.ReasonAmountTooManyDecimals, "value", text, "currency", currency, "example", example)
	}
	return migration.Why(migration.ReasonNotAnAmount, "value", text, "currency", currency, "example", example)
}
