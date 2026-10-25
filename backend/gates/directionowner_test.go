// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every reader of exchanges that asks "did they write" or "did we write" asks
// relstrength.TheyWroteSQL and WeWroteSQL. A reader that filters on
// activity.direction by itself counts a colleague's reply as the contact's, and
// the contact page and the company page then disagree about one relationship.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// exchangeReaderRenderers mark a file as a reader of exchanges.
var exchangeReaderRenderers = []string{
	"InteractionCountsSQL", "InteractionCountsAtSQL", "DirectionClockSQL", "WrittenByHumanSQL", "ContactExchangeSQL",
}

// directionRuleRenderers are relstrength's direction rule.
var directionRuleRenderers = []string{"TheyWroteSQL", "WeWroteSQL", "DirectionOf"}

// literalDirection matches SQL that spells a message direction by hand.
var literalDirection = regexp.MustCompile(`direction\s*(=|<>|IS DISTINCT FROM|IS NOT DISTINCT FROM)\s*'(inbound|outbound)'|direction\s+IN\s*\(`)

// messageStateReaders read a direction to answer a question about one message
// or thread, not which sides of a relationship have written.
var messageStateReaders = gatekit.Waive(map[string]string{
	"internal/compose/signalscan.go": "asks whether the account's newest message is ours; a colleague's reply " +
		"at the account answers us there by design",
	"internal/modules/activities/awaitingreply.go": "asks whether our message on a thread is still waiting for any answer",
	"internal/modules/activities/meetingfollowup.go": "asks whether anything of ours followed a meeting, " +
		"not who on their side wrote",
})

func TestEveryExchangeReaderAsksTheDirectionRule(t *testing.T) {
	t.Parallel()
	defer messageStateReaders.AssertAllMatched(t)
	var readers, withRule int
	for _, pkg := range goPackagesUnder(t, ".") {
		for _, file := range pkg.files {
			if strings.Contains(file.relPath, relstrengthDir) || !readsRelstrength(file, exchangeReaderRenderers) {
				continue
			}
			readers++
			if readsRelstrength(file, directionRuleRenderers) {
				withRule++
			}
			if !spellsDirection(file) || messageStateReaders.Waived(t, file.relPath) {
				continue
			}
			t.Errorf("%s reads exchanges and spells activity.direction in its own SQL. Ask relstrength.TheyWroteSQL "+
				"(the contact sent it) or WeWroteSQL. If this file asks about one message, not the relationship, "+
				"say which in messageStateReaders under %q", file.relPath, file.relPath)
		}
	}
	// The strength fold, both 360 pages and the colleague graph read the rule;
	// a walk that found fewer read a smaller tree.
	if readers < 10 || withRule < 4 {
		t.Fatalf("found %d exchange readers and %d asking the direction rule; the census has stopped seeing them", readers, withRule)
	}
}

func readsRelstrength(file goFile, names []string) bool {
	for _, name := range names {
		if gatekit.References(file.ast, relstrengthImport, name) {
			return true
		}
	}
	return false
}

func spellsDirection(file goFile) bool {
	found := false
	ast.Inspect(file.ast, func(node ast.Node) bool {
		if expr, isExpr := node.(ast.Expr); isExpr {
			if text, isText := gatekit.LiteralText(expr); isText && literalDirection.MatchString(text) {
				found = true
			}
		}
		return !found
	})
	return found
}

func TestTheDirectionCensusSeesAPlantedReader(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"equality": "package planted\nvar q = `SELECT max(a.occurred_at) FILTER (WHERE a.direction = 'inbound')`",
		"in list":  "package planted\nvar q = `SELECT 1 FROM activity a WHERE a.direction IN ('inbound', 'outbound')`",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "planted.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !spellsDirection(goFile{relPath: "planted.go", ast: parsed}) {
			t.Errorf("the census misses a planted %s", name)
		}
	}
}
