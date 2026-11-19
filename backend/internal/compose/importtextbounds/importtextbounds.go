// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importtextbounds finds the column each import target writes in the
// text registry. The import's test run then refuses a cell the column's guard
// would refuse at commit.
package importtextbounds

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// reasons words an object's target refusal its own way; any other says
// ReasonValueTooLong.
var reasons = map[string]migration.ReasonCode{migration.ObjectCompany + ".description": migration.ReasonDescriptionTooLong}

// Refusal answers a contact name past the name limit first, with the text it
// measured. Otherwise it answers the first target, in name order, whose trimmed
// cell is past its column's limit.
func Refusal(object string, fields map[string]string) (target, value string, why migration.Reason, refused bool) {
	if object == migration.ObjectContact {
		// The registry exempts the name columns; the contacts module owns their limit.
		if column, text, length, over := contacts.OverlongContactName(fields); over {
			return column, text, tooLong(migration.ReasonValueTooLong, length, contacts.ContactNameLimit), true
		}
	}
	for _, target := range slices.Sorted(maps.Keys(fields)) {
		limit := limitOf(object, target)
		length := utf8.RuneCountInString(strings.TrimSpace(fields[target]))
		if limit > 0 && length > limit {
			reason, worded := reasons[object+"."+target]
			if !worded {
				reason = migration.ReasonValueTooLong
			}
			return target, fields[target], tooLong(reason, length, limit), true
		}
	}
	return "", "", migration.Reason{}, false
}

// NameTooLong reads a store's contact-name refusal as the reason an import
// skips its row, in the wording the row check gives.
func NameTooLong(err error) (migration.Reason, bool) {
	refusal, ok := errors.AsType[*contacts.ContactNameTooLongError](err)
	if !ok {
		return migration.Reason{}, false
	}
	return tooLong(migration.ReasonValueTooLong, refusal.Runes, contacts.ContactNameLimit), true
}

func tooLong(reason migration.ReasonCode, length, limit int) migration.Reason {
	return migration.Why(reason, "length", strconv.Itoa(length), "max", strconv.Itoa(limit))
}

// limitOf reads a target as the column of the object's table it names. A custom
// field holds the document limit, and an address part maps to its address_
// column. A target that names no bounded column answers 0.
func limitOf(object, target string) int {
	if datasource.IsCustomFieldKey(target) {
		return values.TextDocumentCharacters
	}
	column := strings.Replace(target, "address.", "address_", 1)
	return textbounds.LimitOf("public." + object + "." + column)
}
