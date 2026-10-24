// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every statement that inserts a lead says where the lead came from.
//
// A lead's origin is the one fact its score, the origin filter and the report
// read. A writer whose INSERT leaves the column out files the lead as
// unclassified without anyone deciding that, and the statement still runs. The
// two writers today are the contacts store (every typed, imported and tool
// create) and the capture sink (every connector). The corpus is every
// production string literal in backend/internal that opens `INSERT INTO lead (`,
// so a third writer is judged the day it appears. A census that finds fewer
// than two statements has read a smaller tree than the one that exists and
// fails instead of passing.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var insertsLead = regexp.MustCompile(`(?is)\bINSERT\s+INTO\s+lead\s*\(`)

var namesOrigin = regexp.MustCompile(`(?is)\borigin\b`)

func TestEveryLeadInsertNamesTheOrigin(t *testing.T) {
	t.Parallel()
	fset := gatekit.SourceFileSet()
	statements := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, "internal/contracts/") ||
			isIntegrationTagged(path) || !strings.HasPrefix(where, "internal/") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			at := insertsLead.FindStringIndex(text)
			if at == nil {
				return true
			}
			statements++
			columns := text[at[1]:]
			if end := strings.Index(columns, ")"); end >= 0 {
				columns = columns[:end]
			}
			if !namesOrigin.MatchString(columns) {
				t.Errorf("%s inserts a lead without naming `origin`. Give the column a value, even NULL, so the "+
					"lead's classification is a decision of the writer and not an omission", fset.Position(lit.Pos()))
			}
			return true
		})
	}
	if statements < 2 {
		t.Fatalf("found %d statements that insert a lead, want at least two (the contacts store and the capture sink): "+
			"the walk read a smaller tree than the one that exists", statements)
	}
}
