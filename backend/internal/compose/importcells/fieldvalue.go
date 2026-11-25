// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importcells reads one import cell the way the store will hold it.
// Each check reads only the cell's text and the field it fills, and says in the
// import's reason codes why the store would refuse it.
package importcells

import (
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// FieldValue turns one custom-field cell into the value storekit.SQLValue takes.
// SQLValue decides whether it binds, so the dry run accepts the text the store
// does. A refusal lists a picklist's options only
// when the caller may read them (custom_field:read).
//
//craft:ignore naked-any the value is SQLValue's own input, whose shape depends on the column type
func FieldValue(col fieldcatalog.Column, raw string, optionsReadable bool) (any, migration.Reason) {
	text := strings.TrimSpace(raw)
	var wire any = text
	var message migration.Reason
	switch col.Type {
	case fieldcatalog.TypePicklist:
		option, refusal := picklistValue(col, text, optionsReadable)
		wire, message = option, refusal
	case fieldcatalog.TypeMultiselect:
		chosen, refusal := multiselectValue(col, text, optionsReadable)
		wire, message = chosen, refusal
	case fieldcatalog.TypeCurrency:
		minor, refusal := currencyValue(col, text)
		wire, message = minor, refusal
	case fieldcatalog.TypeBoolean:
		value, refusal := booleanValue(text)
		wire, message = value, refusal
	}
	if message.Code == "" {
		if _, binds := storekit.SQLValue(col, wire); !binds {
			message = unboundReason(col, text)
		}
	}
	if message.Code != "" {
		return nil, message
	}
	return wire, migration.Reason{}
}

// picklistValue is the catalog's spelling of the option the text names.
func picklistValue(col fieldcatalog.Column, text string, optionsReadable bool) (string, migration.Reason) {
	if option, found := optionNamed(col.Options, text); found {
		return option, migration.Reason{}
	}
	return "", notAllowedReason(col, text, optionsReadable)
}

// multiselectValue splits on semicolons and names each option in the catalog's
// spelling. A comma with no semicolon is the usual mistake, so it gets its own
// sentence once the whole text is not itself an option.
func multiselectValue(col fieldcatalog.Column, text string, optionsReadable bool) ([]string, migration.Reason) {
	var chosen []string
	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		option, found := optionNamed(col.Options, part)
		if !found {
			if strings.Contains(text, ",") && !strings.Contains(text, ";") {
				return nil, migration.Why(migration.ReasonOptionsNeedSemicolons, "value", text)
			}
			return nil, notAllowedReason(col, part, optionsReadable)
		}
		chosen = append(chosen, option)
	}
	return chosen, migration.Reason{}
}

// currencyValue is the amount in the column's minor units. A column with no
// currency has no unit to count, so its values cannot be read at all.
func currencyValue(col fieldcatalog.Column, text string) (float64, migration.Reason) {
	if col.Currency == "" {
		return 0, migration.Why(migration.ReasonCurrencyUnset, "value", text)
	}
	minor, ok := values.MinorUnits(text, col.Currency)
	if !ok {
		return 0, AmountReason(text, col.Currency)
	}
	return float64(minor), migration.Reason{}
}

// spreadsheetBooleans are the spellings a spreadsheet uses for yes and no.
//
//nolint:goconst // a cell's spelling; the constant goconst offers names a query argument's
var spreadsheetBooleans = map[string]bool{"true": true, "yes": true, "1": true, "false": false, "no": false, "0": false}

// booleanValue reads a cell spelled as one of spreadsheetBooleans.
func booleanValue(text string) (bool, migration.Reason) {
	if value, ok := spreadsheetBooleans[strings.ToLower(text)]; ok {
		return value, migration.Reason{}
	}
	return false, migration.Why(migration.ReasonNotYesNo, "value", text)
}

func optionNamed(options []string, text string) (string, bool) {
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option), text) {
			return option, true
		}
	}
	return "", false
}

func notAllowedReason(col fieldcatalog.Column, text string, optionsReadable bool) migration.Reason {
	if !optionsReadable {
		return migration.Why(migration.ReasonOptionNotAllowed, "value", text)
	}
	return migration.Why(migration.ReasonOptionNotInList, "value", text, "options", strings.Join(col.Options, ", "))
}

// unboundReason says why SQLValue refused a value of this type.
func unboundReason(col fieldcatalog.Column, text string) migration.Reason {
	switch col.Type {
	case fieldcatalog.TypeNumber:
		return migration.Why(migration.ReasonNotANumber, "value", text)
	case fieldcatalog.TypeDate:
		return migration.Why(migration.ReasonNotADate, "value", text)
	}
	return migration.Why(migration.ReasonCannotWriteType, "value", text, "type", string(col.Type))
}
