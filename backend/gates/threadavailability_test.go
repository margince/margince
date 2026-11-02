// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestEveryActivityReaderKeepsAThreadRepairUnavailable(t *testing.T) {
	t.Parallel()
	defer threadRepairReadersAdmitted.AssertAllMatched(t)
	dimension := threadAvailabilityDimension()
	graphs := map[string]map[string]*graphFunc{}
	bindings := map[string]map[string]map[string]bool{}
	for _, src := range activityReaderScope.Files(t) {
		dir := path.Dir(src.Path)
		if _, done := graphs[dir]; !done {
			graphs[dir] = packageCallGraph(t, dir)
			bindings[dir] = packageBindingNames(parsePackageFiles(t, dir))
		}
		for _, offender := range unguardedActivityReaders(graphs[dir], bindings[dir], src.File, dimension) {
			if threadRepairReadersAdmitted.Waived(t, src.Path+":"+strings.SplitN(offender, ":", 2)[0]) {
				continue
			}
			t.Errorf("%s: %s reads activity while a thread repair can be incomplete", src.Path, strings.SplitN(offender, ":", 2)[0])
		}
	}
}

func TestEveryActivityWriterKeepsAThreadRepairUnavailable(t *testing.T) {
	t.Parallel()
	defer threadRepairWritersAdmitted.AssertAllMatched(t)
	graphs := map[string]map[string]*graphFunc{}
	seen := map[string]bool{}
	count := 0
	targets := threadKeyedTables(t)
	for owner, writes := range collectTableWrites(t) {
		for _, write := range writes {
			site := path.Join(owner, write.site)
			if !targets[write.table] || seen[site] {
				continue
			}
			seen[site] = true
			count++
			file, fn, found := strings.Cut(site, ":")
			if !found {
				t.Fatalf("activity writer has no function: %s", site)
			}
			dir := path.Dir(file)
			if _, done := graphs[dir]; !done {
				graphs[dir] = packageCallGraph(t, dir)
			}
			if guardedInItsPackage(graphs[dir], fn, threadWriteAvailabilityDimension()) {
				continue
			}
			if threadRepairWritersAdmitted.Waived(t, site) {
				continue
			}
			t.Errorf("%s writes %s while a thread repair can be incomplete", site, write.table)
		}
	}
	if count == 0 {
		t.Fatal("activity writer census found no subjects")
	}
}

func threadAvailabilityDimension() activityDimension {
	return activityDimension{
		scopeMarkers:   []string{"ActivityDiscoverClause", "ActivityContentClause", "ActivityAvailableClause", "EnsureActivityVisible", "EnsureActivityVisibleLive", "EnsureActivityContentVisible", "EnsureActivityContentVisibleLive", "EnsureActivityWritable", "EnsureActivityThreadAvailable", "EnsureThreadAvailable", "lockActivityForWrite", "ThreadSQL", "ConversationSQL", "SubjectMailSQL", "AudienceWorkspaceOnly", "CurrentCommitmentClause", "CurrentTaskClause", "CurrentSourceMailClause", "ClassifyBacklogPredicate", "SignatureSourceEligible", "CitedActivityTime", "CitedActivityStamp", "CitedActivityPrecision", "signatureSourceSQL", "signatureMailSQL", "hideUnreadableLinks"},
		literalMarkers: []*regexp.Regexp{regexp.MustCompile(`(?i)\b(?:AND|WHERE|ON)\s+activity_thread_available\s*\(`)},
	}
}

func threadWriteAvailabilityDimension() activityDimension {
	return activityDimension{
		scopeMarkers:   []string{"EnsureActivityWritable", "EnsureActivityThreadAvailable", "EnsureThreadAvailable", "lockActivityForWrite"},
		literalMarkers: threadAvailabilityDimension().literalMarkers,
	}
}

func threadKeyedTables(t *testing.T) map[string]bool {
	t.Helper()
	tables := map[string]bool{}
	for name, table := range parseHeadCatalog(t).tables {
		for _, column := range table.columns {
			if column.name == "thread_key" {
				tables[name] = true
			}
		}
	}
	if !tables["activity"] {
		t.Fatal("committed schema lost activity.thread_key")
	}
	return tables
}

func TestAThreadRepairNeedsAPositiveAvailabilityExclusion(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		sql     string
		guarded bool
	}{
		{`SELECT id FROM activity a WHERE activity_thread_available(a.thread_key)`, true},
		{`SELECT id FROM activity a WHERE a.id = $1 AND activity_thread_available(a.thread_key)`, true},
		{`SELECT id FROM activity a WHERE NOT activity_thread_available(a.thread_key)`, false},
		{`SELECT id FROM activity a WHERE a.id = $1 OR activity_thread_available(a.thread_key)`, false},
		{`SELECT id FROM activity a -- WHERE activity_thread_available(a.thread_key)`, false},
	} {
		graph := map[string]*graphFunc{"reader": {statements: []string{check.sql}}}
		if got := reachesAnExclusion(graph, "reader", false, threadAvailabilityDimension()); got != check.guarded {
			t.Errorf("guarded = %t, want %t for %s", got, check.guarded, check.sql)
		}
	}
	graph := map[string]*graphFunc{"reader": {calls: map[string]bool{"EnsureVisible": true, "ScopeClauseFor": true}}}
	if reachesAnExclusion(graph, "reader", false, threadAvailabilityDimension()) {
		t.Fatal("generic row visibility proves no activity thread availability")
	}
}

func TestAThreadWriterCannotBorrowAGenericReaderGuard(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ActivityContentClause", "ScopeClauseFor", "CurrentTaskClause", "EnsureActivityVisible", "AudienceWorkspaceOnly"} {
		graph := map[string]*graphFunc{"writer": {calls: map[string]bool{name: true}}}
		if guardedInItsPackage(graph, "writer", threadWriteAvailabilityDimension()) {
			t.Errorf("writer admitted generic read helper %s", name)
		}
	}
	for _, name := range []string{"EnsureActivityWritable", "EnsureActivityThreadAvailable", "EnsureThreadAvailable", "lockActivityForWrite"} {
		graph := map[string]*graphFunc{"writer": {calls: map[string]bool{name: true}}}
		if !guardedInItsPackage(graph, "writer", threadWriteAvailabilityDimension()) {
			t.Errorf("writer lost explicit boundary %s", name)
		}
	}
}
