// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnlyOneReaderAnswersTheAutomaticLookupPosture holds settings.go's claim
// about how the automatic path reaches the posture: through
// automaticLookupEnabled, never through the entry directly.
//
// The failure it exists to catch is two readers drifting: admission refusing a
// run the sweep would have queued, or the reverse, because one of them read the
// entry directly and grew its own default or its own error handling. That
// disagreement is invisible in a diff — both call sites look correct alone.
//
// It reads THIS package's sources rather than a list, so a new file that
// reaches for the entry is caught the day it is written.
func TestOnlyOneReaderAnswersTheAutomaticLookupPosture(t *testing.T) {
	t.Parallel()

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package's sources: %v", err)
	}

	// settingsentry.go declares the entry and settings.go wraps it; every other
	// file must go through the wrapper.
	const declarer, wrapper = "settingsentry.go", "settings.go"

	var offenders []string
	for _, src := range sources {
		if src == declarer || src == wrapper || strings.HasSuffix(src, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", src, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if ok && ident.Name == "AutomaticLookup" {
				offenders = append(offenders, src)
			}
			return true
		})
	}

	if len(offenders) > 0 {
		t.Errorf("AutomaticLookup is read outside %s in: %s\n\n"+
			"The automatic path reads the posture through automaticLookupEnabled so admission "+
			"and the catch-up sweep cannot answer differently. Call that instead, or move the "+
			"claim in %s if a second reader is genuinely right.",
			wrapper, strings.Join(offenders, ", "), wrapper)
	}
}
