// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildsRunIndex finds a statement that creates workflow_run_by_automation.
var buildsRunIndex = regexp.MustCompile(`(?s)CREATE INDEX (?:CONCURRENTLY )?workflow_run_by_automation ON [^;]*;`)

// The index serves the run readers only while it holds the expression they
// match on. The latest migration that builds it is read back and compared.
func TestTheRunIndexHoldsTheExpressionTheReadersMatch(t *testing.T) {
	ups, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatalf("listing core migrations: %v", err)
	}
	if len(ups) < 300 {
		t.Fatalf("found %d core migrations, expected at least 300: the path no longer reaches them", len(ups))
	}
	var build string
	for _, path := range ups {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		if found := buildsRunIndex.FindAllString(string(body), -1); len(found) > 0 {
			build = found[len(found)-1]
		}
	}
	if build == "" {
		t.Fatal("no core migration builds workflow_run_by_automation, so every run read scans")
	}
	want := "ON workflow_run (handler, " + runAutomationIDSQL + ", "
	if !strings.Contains(build, want) {
		t.Errorf("the run index is built as\n  %s\nwhich does not lead with %q, the expression the readers match", build, want)
	}
}
