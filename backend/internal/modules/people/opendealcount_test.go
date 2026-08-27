// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// "Open deal" is a definition, not a column: a status, an archive check, and a
// currency conversion, spelled once in the 0065 rollup view. The organization
// list's count and the company page's open-pipeline tile both read it, which is
// the only reason a company page cannot show a number the list disagrees with.
//
// The drift is not a second read of the view — it is a FIRST read of `deal`.
// A count assembled here would be a second definition of open, and the two
// would disagree the first time the view's own definition moved: the fx date it
// converts at, the archive predicate, or what statuses count. Both numbers are
// on screen at once, so a reader sees the disagreement before anyone else does.
//
// Two directions, because a one-directional census misses the shape that does
// not name the column: a query that never says `open_deal_count` and counts
// open deals off the table anyway is exactly the copy this is about.
//
// And the second direction cannot key on the word `open`. "The copy that does
// not say the word" is what the header claims to be about, and a census
// matching `'open'` sees none of the ways of writing the same constraint:
//
//	status = ANY($1)              the statuses bound as a parameter
//	status NOT IN ('won','lost')  the complement, which is the same set
//	status <> 'won' AND …         the complement again, one arm at a time
//
// So it matches a CONSTRAINT ON THE STATUS COLUMN, whatever it compares
// against. A query that constrains deal status here is assembling the
// definition here, and which values it names is not the question.

const openPipelineRollupView = "organization_open_pipeline_rollup"

// dealStatusConstraint matches a restriction on a status column: an equality,
// an inequality, a set membership or its negation. `= ANY(…)` is an equality
// and needs no arm of its own; `deal_status` does not match, because an
// underscore is a word character and the boundary holds.
var dealStatusConstraint = regexp.MustCompile(`(?i)\bstatus\b\s*(=|<>|!=|\bIN\b|\bNOT\s+IN\b)`)

func TestEveryOpenDealCountComesFromTheRollup(t *testing.T) {
	counts, dealReads := 0, 0
	for _, sql := range moduleSQLLiterals(t) {
		if strings.Contains(sql.text, "open_deal_count") {
			counts++
			if !gatekit.TableReadPattern(openPipelineRollupView).MatchString(sql.text) {
				t.Errorf("%s reads open_deal_count without reading %s:\n\n\t%s\n\n"+
					"The rollup is where open is defined. A count read from anywhere else is a "+
					"second definition, and the list and the company page show both at once.",
					sql.where, openPipelineRollupView, gatekit.FirstLineOf(sql.text))
			}
		}
		if !gatekit.TableReadPattern("deal").MatchString(sql.text) {
			continue
		}
		dealReads++
		match := dealStatusConstraint.FindString(sql.text)
		if match == "" || sql.reasoned {
			continue
		}
		t.Errorf("%s reads the deal table and constrains its status (`%s`):\n\n\t%s\n\n"+
			"That assembles the definition of open here rather than taking it from %s, "+
			"whichever statuses it names — the complement of won and lost is the same set "+
			"written backwards. Read the rollup, or say beside the query why this "+
			"population is not the one the rollup counts.",
			sql.where, strings.TrimSpace(match), gatekit.FirstLineOf(sql.text),
			openPipelineRollupView)
	}
	// Both arms need a subject. The first has one only while the module still
	// serves the count; the second only while the module still reaches the deal
	// table at all, and if it stops, the arm guarding that reach is dead weight
	// rather than a silent pass — so it says so instead of reading green.
	if counts == 0 {
		t.Errorf("no query in this module reads open_deal_count, so the arm holding it against "+
			"%s judged nothing", openPipelineRollupView)
	}
	if dealReads == 0 {
		t.Error("no query in this module reads the deal table, so the arm refusing a second " +
			"definition of open judged nothing — delete it, or find where the reads went")
	}
}

// moduleSQL is one string in the module's own sources, with where it is and
// whether a comment sits beside it.
//
// The reason matters because this gate offers one. Its failure says "or say
// beside the query why this population is not the one the rollup counts", and
// an instruction a gate does not read is an instruction that cannot be
// followed — the author writes the sentence, the gate fails anyway, and the
// only remaining move is to delete the gate. A legitimate join that constrains
// a LEAD's status alongside a deal read needs that door to exist.
type moduleSQL struct {
	where    string
	text     string
	reasoned bool
}

// moduleSQLLiterals returns every string in the module's non-test sources, with
// a concatenation folded into the one string it builds.
//
// Every string, not the ones that look like SQL: deciding what looks like SQL is
// where a census goes blind, and a literal that is not a query matches none of
// the patterns above anyway.
//
// Folding matters as much as the sweep. A query assembled as `"… FROM deal " +
// where + " AND status = 'won'"` puts the table in one literal and the
// constraint in another, and a census reading literals one at a time sees a
// deal read with no status and a status with no table — two halves, each
// innocent, of exactly the copy this holds against.
func moduleSQLLiterals(t *testing.T) []moduleSQL {
	t.Helper()
	var found []moduleSQL
	for _, parsed := range moduleFiles(t) {
		// Both the literals a chain consumed AND the chain's own inner nodes.
		// `a + b + c` parses as `(a + b) + c`, so walking every BinaryExpr
		// reports the outer chain and then the inner one again — the same query
		// found twice, at two positions, which reads as two defects.
		folded := map[ast.Node]bool{}
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			binary, isBinary := node.(*ast.BinaryExpr)
			if !isBinary || binary.Op != token.ADD || folded[ast.Node(binary)] {
				return true
			}
			text, parts := concatenatedText(binary, packageStrings(t))
			if len(parts) == 0 {
				return true
			}
			for _, part := range parts {
				folded[part] = true
			}
			ast.Inspect(binary, func(inner ast.Node) bool {
				if nested, isNested := inner.(*ast.BinaryExpr); isNested {
					folded[ast.Node(nested)] = true
				}
				return true
			})
			found = append(found, moduleSQL{
				where:    parsed.fset.Position(binary.Pos()).String(),
				text:     text,
				reasoned: hasReasonNear(parsed, binary.Pos()),
			})
			return true
		})
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			lit, isLit := node.(*ast.BasicLit)
			if !isLit || folded[ast.Node(lit)] {
				return true
			}
			text, isText := gatekit.LiteralText(lit)
			if !isText {
				return true
			}
			found = append(found, moduleSQL{
				where:    parsed.fset.Position(lit.Pos()).String(),
				text:     text,
				reasoned: hasReasonNear(parsed, lit.Pos()),
			})
			return true
		})
	}
	if len(found) == 0 {
		t.Fatal("no string literal found in the module's sources, so this census read nothing")
	}
	return found
}
