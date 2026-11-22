// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Which tags a Live List's filter names that are no longer in use. An archived
// tag, whether merged away or retired, matches no record, so a clause on one
// selects nothing while the list still evaluates: only the list can say so.

import (
	"slices"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tagIDsArg names the bind parameter holding the tag ids a list names.
const tagIDsArg = "tag_ids"

// tagFieldsOf is the filter fields of one record type that hold tag ids,
// read off the vocabulary itself so a tag field added to it is judged too.
func tagFieldsOf(entityType string) map[string]bool {
	fields := map[string]bool{}
	for name, field := range segmentEngines[entityType].Fields {
		if field.References == storekit.RefTag {
			fields[name] = true
		}
	}
	return fields
}

// tagIDsNamed lists the tag ids a filter tree's leaves compare a tag field to,
// once each. An operand that is not an id is the compiler's to refuse, so it
// is skipped here.
func tagIDsNamed(p storekit.Predicate, tagFields map[string]bool) []ids.UUID {
	var out []ids.UUID
	add := func(raw any) {
		text, ok := raw.(string)
		if !ok {
			return
		}
		id, err := ids.Parse(text)
		if err != nil {
			return
		}
		if slices.Contains(out, id) {
			return
		}
		out = append(out, id)
	}
	for _, leaf := range storekit.Leaves(p) {
		if !tagFields[leaf.Field] {
			continue
		}
		if many, ok := leaf.Value.([]any); ok {
			for _, v := range many {
				add(v)
			}
			continue
		}
		add(leaf.Value)
	}
	return out
}
