// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The brief cache's upsert is spelled in two places and they must stay one
// statement.
//
// contactbrief.Service.save writes the cache; an integration test asserts that
// an erasure in flight stops that write, and it cannot reach save() without an
// assembled page and a model lane, so it runs the statement itself. A mirror
// that drifts is worse than no test: the guard could be dropped from the writer
// and the test would go on proving it about a string only it uses.
//
// Fails in both directions — a change to either side that the other does not
// carry.
func TestTheBriefCacheTestsMirrorTheWriterTheyStandIn(t *testing.T) {
	t.Parallel()
	writer := statementAfter(t, "internal/compose/contactbrief/service.go", "INSERT INTO contact_brief")
	mirror := statementAfter(t, "internal/compose/erasurebriefcache_integration_test.go", "INSERT INTO contact_brief")
	if writer != mirror {
		t.Errorf("the brief cache upsert differs between the writer and the test standing in for it.\n"+
			"writer: %s\nmirror: %s\n\n"+
			"Carry the change to both: contactbrief.Service.save and briefUpsertSQL in the "+
			"integration test. The test asserts an in-flight erasure stops this write, which it "+
			"can only do about the statement the writer actually runs.", writer, mirror)
	}
	if !strings.Contains(writer, "FOR SHARE") || !strings.Contains(writer, "archived_at IS NULL") {
		t.Errorf("the brief cache upsert no longer reads the contact row for liveness under a "+
			"lock, so a brief composed before an erasure can land after it: %s", writer)
	}
}

var sqlWhitespace = regexp.MustCompile(`\s+`)

// statementAfter reads the backquoted SQL literal containing the needle, with
// its whitespace flattened so indentation is not a difference.
func statementAfter(t *testing.T, path, needle string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	for _, chunk := range strings.Split(string(raw), "`")[1:] {
		if strings.Contains(chunk, needle) {
			return strings.TrimSpace(sqlWhitespace.ReplaceAllString(chunk, " "))
		}
	}
	t.Fatalf("%s holds no backquoted statement containing %q", path, needle)
	return ""
}
