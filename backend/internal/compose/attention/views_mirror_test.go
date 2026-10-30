// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"os"
	"regexp"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The browser draws views and category buttons from its own copy of the view
// split. These tests hold that copy to views.go in both directions: a change on
// either side alone fails here, and tsc refuses a contract value the copy lacks.

const frontendViews = "../../../../frontend/src/lib/worklistviews.ts"

var (
	tsViewEntry = regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*(null|"([a-z_]+)"),?\s*$`)
	tsComments  = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)
)

// mirrorTable reads one `NAME ... = { key: "view", ... };` literal.
func mirrorTable(t *testing.T, name string) map[string]string {
	t.Helper()
	source, err := os.ReadFile(frontendViews)
	if err != nil {
		t.Fatalf("reading the frontend view table: %v", err)
	}
	text := tsComments.ReplaceAllString(string(source), "")
	start := strings.Index(text, "const "+name+":")
	if start < 0 {
		t.Fatalf("%s is gone from %s; this test cannot check the mirror", name, frontendViews)
	}
	end := strings.Index(text[start:], "};")
	entries := map[string]string{}
	for _, m := range tsViewEntry.FindAllStringSubmatch(text[start:start+end], -1) {
		entries[m[1]] = m[3]
	}
	if len(entries) == 0 {
		t.Fatalf("%s holds no entry this test can read", name)
	}
	return entries
}

func TestTheFrontendFilterViewsMatchTheServer(t *testing.T) {
	for filter, view := range mirrorTable(t, "FILTER_VIEW") {
		if got := viewOfFilter(filter); got != view {
			t.Errorf("filter %q: the server puts it in view %q, the frontend in %q", filter, got, view)
		}
	}
}

func TestTheFrontendCategoryViewsMatchTheServer(t *testing.T) {
	views := []string{viewToday, viewDecisions, viewSystem}
	for category, view := range mirrorTable(t, "CATEGORY_VIEW") {
		row := ranked{item: crmcontracts.WorklistItem{Category: crmcontracts.WorklistItemCategory(category)}}
		for _, candidate := range views {
			if inView(row, candidate) != (candidate == view) {
				t.Errorf("category %q: the frontend puts it in view %q, the server disagrees about %q", category, view, candidate)
			}
		}
	}
}
