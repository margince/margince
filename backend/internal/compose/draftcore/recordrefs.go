// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import "github.com/margince/margince/backend/internal/shared/kernel/promptref"

// refPrefix is the ref prefix of each record kind a draft payload shows. A
// kind not listed is spelled out, so two kinds never share a prefix; a record
// of no stated kind reads r1.
var refPrefix = map[string]string{"deal": "d", "activity": "a", "contact": "c", "lead": "l", "": "r"}

// RecordRefs names each record one draft payload shows by a short ref, one
// table per record kind, and reads a cited ref back to its record id. A
// citation is a pair, so a ref only reads back under the kind it was shown as.
type RecordRefs struct {
	tables map[string]*promptref.Refs
}

// NewRecordRefs starts an empty table set.
func NewRecordRefs() *RecordRefs {
	return &RecordRefs{tables: map[string]*promptref.Refs{}}
}

// For returns the ref the payload shows for id, a record of kind. An empty id
// stays empty: the field it fills names no record.
func (r *RecordRefs) For(kind, id string) string {
	if id == "" {
		return ""
	}
	table, ok := r.tables[kind]
	if !ok {
		prefix, listed := refPrefix[kind]
		if !listed {
			prefix = kind
		}
		table = promptref.New(prefix)
		r.tables[kind] = table
	}
	return table.For(id)
}

// ID reads ref back to the id of the record of kind it names, and false for a
// ref the payload never showed as that kind.
func (r *RecordRefs) ID(kind, ref string) (string, bool) {
	if r == nil {
		return "", false
	}
	table, ok := r.tables[kind]
	if !ok {
		return "", false
	}
	return table.ID(ref)
}
