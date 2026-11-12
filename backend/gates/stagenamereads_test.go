// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// A stage's name reaches a reader in their UI language. Server SQL reads it
// through labeltranslation.SQL, which takes the column names as literals of
// their own. A statement that still selects a stage alias's `name` reads the
// stored name. Each such read is declared here with why no reader sees it.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// storedStageNameReads are the files that read the stored name, with why.
var storedStageNameReads = gatekit.Waive(map[string]string{
	"internal/compose/captureofflinedemo.go":      "writes demo mail that every reader of the demo shares, so it keeps the stored names",
	"internal/compose/dealverdictinput.go":        "model input; the verdict is stored once for every reader, so it keeps the stored names",
	"internal/compose/extsystemread.go":           "a system read for extensions, which carry no UI language",
	"internal/modules/deals/deal_advance.go":      "the deal.stage_changed event keeps the stored name for subscribers",
	"internal/modules/deals/keynamedrepair.go":    "writes the stored stage into a deal name that every reader shares",
	"internal/modules/deals/legalreviewpacket.go": "a frozen packet several reviewers read, each in their own language",
})

var (
	stageAlias       = regexp.MustCompile(`(?i)\b(?:join|from)\s+stage\s+(?:as\s+)?(\w+)`)
	stageNameSubread = regexp.MustCompile(`(?i)\bselect\s+(?:\w+\.)?name\s+from\s+stage\b`)
)

func TestEveryStageNameAReaderSeesIsTranslated(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	files := 0
	err := gatekit.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files++
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err == nil && readsStoredStageName(text) {
				found[filepath.ToSlash(path)] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal/: %v", err)
	}
	if files < 2000 {
		t.Fatalf("read %d Go files; the census went short", files)
	}
	for path := range found {
		if !storedStageNameReads.Waived(t, path) {
			t.Errorf("%s selects a stage's stored name; read it through labeltranslation.SQL, or declare why no reader sees it", path)
		}
	}
	storedStageNameReads.AssertAllMatched(t)
}

func readsStoredStageName(statement string) bool {
	if stageNameSubread.MatchString(statement) {
		return true
	}
	for _, match := range stageAlias.FindAllStringSubmatch(statement, -1) {
		alias := match[1]
		if strings.EqualFold(alias, "on") || strings.EqualFold(alias, "where") {
			continue
		}
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.name\b`).MatchString(statement) {
			return true
		}
	}
	return false
}
