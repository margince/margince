// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"strings"
	"testing"
)

// The drift sweep's id scan and the backlog count read the same pending set
// (#10966). The sweep's old copy trimmed text before the anti-join and had no
// embeddable predicate, so it read every mail body on each pass. Both
// statements must narrow by the source's predicate and read text afterwards.
func TestThePendingStatementsNarrowFirstAndReadTextAfterTheAntiJoin(t *testing.T) {
	for entityType, src := range pendingSources {
		for name, sql := range map[string]string{
			"pendingIDsSQL":        src.pendingIDsSQL(entityType, DriftFull),
			"pendingIDsSQL recent": src.pendingIDsSQL(entityType, DriftRecent),
			"pendingStatsSQL":      src.pendingStatsSQL(entityType),
		} {
			if !strings.Contains(sql, src.embeddablePredicate()) {
				t.Errorf("%s(%s) does not narrow by the source's embeddable predicate %q", name, entityType, src.embeddablePredicate())
			}
			antiJoin := strings.Index(sql, "NOT EXISTS")
			firstTrim := strings.Index(sql, "btrim(")
			if antiJoin < 0 || firstTrim < 0 {
				t.Fatalf("%s(%s) lost its anti-join or its non-empty-text filter:\n%s", name, entityType, sql)
			}
			if firstTrim < antiJoin {
				t.Errorf("%s(%s) reads the text before the embedding anti-join, detoasting every row:\n%s", name, entityType, sql)
			}
		}
	}
}

// The activity source is the one whose predicate matters: without it the sweep
// enumerates participants/selected mail on every pass.
func TestTheSweepIDScanAdmitsOnlyWorkspaceActivity(t *testing.T) {
	sql := pendingSources[entityActivity].pendingIDsSQL(entityActivity, DriftFull)
	if !strings.Contains(sql, "t.audience = 'workspace'") {
		t.Fatalf("the sweep's activity id scan does not narrow to audience = 'workspace':\n%s", sql)
	}
}

// The recent pass bounds the outer table by its indexed column, inside the
// anti-join's CTE, so the bound narrows the rows the anti-join visits. The
// full pass and the backlog count carry no bound.
func TestTheRecentPassBoundsTheOuterTableAndTheFullPassDoesNot(t *testing.T) {
	for entityType, src := range pendingSources {
		recent := src.pendingIDsSQL(entityType, DriftRecent)
		bound := "t." + src.recentColumn + " >= now() - interval '" + recentDriftWindow + "'"
		if src.recentColumn != "" {
			before, _, ok := strings.Cut(recent, "pending_text AS")
			if !ok {
				t.Fatalf("the recent pass over %s has no pending_text CTE:\n%s", entityType, recent)
			}
			unembedded := before
			if !strings.Contains(unembedded, bound) {
				t.Errorf("the recent pass over %s does not bound the outer table by %s:\n%s", entityType, src.recentColumn, recent)
			}
		}
		for name, sql := range map[string]string{
			"full pass":     src.pendingIDsSQL(entityType, DriftFull),
			"backlog count": src.pendingStatsSQL(entityType),
		} {
			if strings.Contains(sql, "interval") {
				t.Errorf("the %s over %s carries a recency bound, so it misses older rows:\n%s", name, entityType, sql)
			}
		}
	}
	for _, entityType := range []string{entityActivity, entityContact, entityCompany} {
		if pendingSources[entityType].recentColumn == "" {
			t.Errorf("%s has no recent column, so every recent pass reads the whole table", entityType)
		}
	}
}
