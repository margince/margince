// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every reader of "which activity is an exchange" asks
// relstrength.InteractionCountsSQL, which carries who wrote the row. A reader
// that filters on the scoring kinds by itself counts a calendar invitation and
// the product's own mail as exchanges. Nothing on screen contradicts it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// bareKindRenderers are relstrength's renderers of the scoring kinds without
// the written-by-human rule.
var bareKindRenderers = []string{"InteractionKindSQLGroup", "InteractionKindSQLList", "IsInteractionKind"}

// humanRuleRenderers carry the written-by-human rule.
var humanRuleRenderers = []string{"InteractionCountsSQL", "InteractionCounts", "DirectionClockSQL", "WrittenByHumanSQL"}

// literalKindList matches a SQL list naming the scoring kinds by hand.
var literalKindList = regexp.MustCompile(`'email'\s*,\s*'call'\s*,\s*'meeting'`)

const relstrengthDir = "internal/shared/kernel/relstrength/"

// bareKindReaders are the files allowed to read the kinds without the rule,
// each with the reason its question is not "is this an exchange".
var bareKindReaders = gatekit.Waive(map[string]string{})

func TestEveryExchangeReaderAsksWhoWroteTheRow(t *testing.T) {
	t.Parallel()
	defer bareKindReaders.AssertAllMatched(t)
	var withRule int
	ruleDirs := map[string]bool{}
	for _, pkg := range goPackagesUnder(t, ".") {
		for _, file := range pkg.files {
			if strings.Contains(file.relPath, relstrengthDir) {
				continue
			}
			for _, name := range humanRuleRenderers {
				if gatekit.References(file.ast, relstrengthImport, name) {
					withRule++
					ruleDirs[file.relPath[:strings.LastIndex(file.relPath, "/")]] = true
					break
				}
			}
			for _, finding := range bareKindReadsIn(file) {
				if bareKindReaders.Waived(t, file.relPath) {
					continue
				}
				t.Errorf("%s %s. An exchange reader asks relstrength.InteractionCountsSQL (or DirectionClockSQL for a direction clock), "+
					"which also asks whether a human wrote the row. If this file asks another question, say which in bareKindReaders under %q",
					file.relPath, finding, file.relPath)
			}
		}
	}
	// The readers live in several modules; a walk that found few of them read
	// a smaller tree and would report a clean one.
	if withRule < 8 || len(ruleDirs) < 5 {
		t.Fatalf("only %d files in %d packages ask relstrength's exchange rule; the census has stopped seeing its readers", withRule, len(ruleDirs))
	}
}

// bareKindReadsIn names each way one file reads the scoring kinds without the
// written-by-human rule.
func bareKindReadsIn(file goFile) []string {
	var out []string
	for _, name := range bareKindRenderers {
		if gatekit.References(file.ast, relstrengthImport, name) {
			out = append(out, "renders the scoring kinds through relstrength."+name+" without the written-by-human rule")
		}
	}
	ast.Inspect(file.ast, func(node ast.Node) bool {
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return true
		}
		if text, isText := gatekit.LiteralText(expr); isText && literalKindList.MatchString(text) {
			out = append(out, "spells the scoring kinds as a literal SQL list")
		}
		return true
	})
	return out
}

func TestTheExchangeCensusSeesAPlantedReader(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"bare renderer": `package planted
import "github.com/margince/margince/backend/internal/shared/kernel/relstrength"
var q = "SELECT count(*) FROM activity a WHERE a.kind IN " + relstrength.InteractionKindSQLGroup()`,
		"literal list": "package planted\nvar q = `SELECT 1 FROM activity a WHERE a.kind IN ('email', 'call', 'meeting', 'message')`",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "planted.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := bareKindReadsIn(goFile{relPath: "planted.go", ast: parsed}); len(got) == 0 {
			t.Errorf("the census misses a planted %s", name)
		}
	}
}
