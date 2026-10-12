// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// What the by-id guard census counts as a compare-and-set, and the cases that
// only look like one.

import (
	"regexp"
	"testing"
)

// returningClause ends the WHERE: the tail whereClause captures runs to the end
// of the statement, and RETURNING is not a predicate.
var returningClause = regexp.MustCompile(`(?i)\bRETURNING\b`)

// statePredicate is a conjunct constraining a column: `status = $3`,
// `status IN ('queued','running')`, `attempt_count < $4`. Either side may be a
// parameter or a literal. Either can fail because of an interleaved write,
// which is what makes the statement match nothing where a blind one overwrites.
var statePredicate = regexp.MustCompile(`(?i)\b([a-z_]+)\s*(?:=|<>|!=|>=|<=|>|<|\bIN\b|\bIS\b)`)

// scopingColumn names what a predicate can constrain without constraining
// STATE: which row to write, not what state it is in. A scope is fixed for the
// row's life, so pinning it cannot fail on a concurrent edit. Tenant scoping
// beside the id reads as a second key rather than a guard.
var scopingColumn = regexp.MustCompile(`(?i)^(id|[a-z_]*_id|installation_id|workspace_id|tenant_id|key|kind|type|slug)$`)

// livenessOnly is `archived_at IS NULL` and its deleted_at twin: a filter two
// concurrent editors of one LIVE row both pass, so it refuses a write to a
// retired row and says nothing about a lost update. Credited only beside a
// predicate that does constrain state.
// The qualifier may be any table or alias name and the conjunct may arrive
// parenthesised, so neither narrows what this recognises: a liveness filter
// spelled `ws.archived_at IS NULL` or `(deleted_at IS NULL)` would otherwise
// read as a predicate on state and be credited.
var livenessOnly = regexp.MustCompile(
	`(?i)^[\s(]*(?:[a-z_]+\.)?(?:archived_at|deleted_at)\s+IS\s+(?:NOT\s+)?NULL[\s)]*$`,
)

// topLevelOr finds a disjunction outside any parentheses, which widens the
// predicate instead of narrowing it: `id = $1 AND status = $2 OR TRUE` writes
// every row.
//
// Parenthesised groups are removed first and the rest read with a word
// boundary, because SQL in a raw string breaks lines and indents with tabs: a
// scan for a literal space on each side misses the spelling a reader is most
// likely to write.
func topLevelOr(where string) bool {
	flat := where
	for {
		reduced := innerParens.ReplaceAllString(flat, " ")
		if reduced == flat {
			break
		}
		flat = reduced
	}
	return bareOr.MatchString(flat)
}

// innerParens matches one parenthesised group with no nested group inside it,
// applied repeatedly so a nested predicate collapses from the inside out.
var innerParens = regexp.MustCompile(`\([^()]*\)`)

var bareOr = regexp.MustCompile(`(?i)\bOR\b`)

// conditionalWrite reports whether a by-id update constrains mutable state
// beside the primary key: the compare-and-set that makes an interleaved write
// lose, rather than a second key or a widened predicate.
func conditionalWrite(lit string) bool {
	w := whereClause.FindStringSubmatch(lit)
	if w == nil {
		return false
	}
	where := returningClause.Split(w[1], 2)[0]
	if topLevelOr(where) {
		return false
	}
	for _, conj := range regexp.MustCompile(`(?i)\bAND\b`).Split(where, -1) {
		if livenessOnly.MatchString(conj) {
			continue
		}
		m := statePredicate.FindStringSubmatch(conj)
		if m != nil && !scopingColumn.MatchString(m[1]) {
			return true
		}
	}
	return false
}

// What counts as constraining state, and what only looks like it.
//
// The three cases this rejects were each found in review of the widened census:
// a scope beside the id is a second key, a disjunction outside parentheses
// widens the predicate instead of narrowing it, and a liveness filter is one
// two concurrent editors of a live row both pass.
func TestOnlyAPredicateOnMutableStateCountsAsACompareAndSet(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		stmt string
		cas  bool
	}{
		{"a parameter on state", "UPDATE t SET a = $2 WHERE id = $1 AND version = $3", true},
		{"a literal on state", "UPDATE t SET a = $2 WHERE id = $1 AND status IN ('queued','running')", true},
		{"the archive's own stamp", "UPDATE t SET a = $2 WHERE id = $1 AND archived_at = $3", true},
		{"state beside a scope", "UPDATE t SET a = $2 WHERE id = $1 AND workspace_id = $3 AND status = $4", true},
		{"a scope alone", "UPDATE t SET a = $2 WHERE id = $1 AND workspace_id = $3", false},
		{"liveness alone", "UPDATE t SET a = $2 WHERE id = $1 AND archived_at IS NULL", false},
		{"a widened WHERE", "UPDATE t SET a = $2 WHERE id = $1 AND status = $3 OR TRUE", false},
		{"the id alone", "UPDATE t SET a = $2 WHERE id = $1", false},
		{"a disjunction inside the predicate", "UPDATE t SET a = $2 WHERE id = $1 AND (status = 'queued' OR status = 'running')", true},
		// The spellings a raw string actually carries: a qualified liveness
		// column, a parenthesised one, and a disjunction broken over lines.
		{"liveness under an alias", "UPDATE t SET a = $2 WHERE id = $1 AND ws.archived_at IS NULL", false},
		{"liveness in parentheses", "UPDATE t SET a = $2 WHERE id = $1 AND (deleted_at IS NULL)", false},
		{"a disjunction across lines", "UPDATE t SET a = $2 WHERE id = $1 AND status = $3\n\t\tOR TRUE", false},
		{"a nested group beside a disjunction", "UPDATE t SET a = $2 WHERE id = $1 AND (status = $3 AND (b = 1))\n OR TRUE", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := conditionalWrite(tc.stmt); got != tc.cas {
				t.Errorf("conditionalWrite(%q) = %v, want %v", tc.stmt, got, tc.cas)
			}
		})
	}
}
