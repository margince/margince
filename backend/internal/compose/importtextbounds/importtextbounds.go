// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importtextbounds finds the column each import target writes in the
// text registry. The import's test run then refuses a cell the column's guard
// would refuse at commit.
package importtextbounds

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// reasons words an object's target refusal its own way; any other says
// ReasonValueTooLong.
var reasons = map[string]migration.ReasonCode{migration.ObjectCompany + ".description": migration.ReasonDescriptionTooLong}

// Refusal answers the first target, in name order, whose trimmed cell is past
// the limit of the column it writes.
func Refusal(object string, fields map[string]string) (target string, why migration.Reason, refused bool) {
	for _, target := range slices.Sorted(maps.Keys(fields)) {
		limit := limitOf(object, target)
		length := utf8.RuneCountInString(strings.TrimSpace(fields[target]))
		if limit > 0 && length > limit {
			reason, worded := reasons[object+"."+target]
			if !worded {
				reason = migration.ReasonValueTooLong
			}
			return target, migration.Why(reason, "length", strconv.Itoa(length), "max", strconv.Itoa(limit)), true
		}
	}
	return "", migration.Reason{}, false
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
