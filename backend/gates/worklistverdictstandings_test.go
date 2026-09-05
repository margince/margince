// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The queue's verdict standings ARE the deal card's, and this derives them from
// the card rather than keeping a second list of them.
//
// Two schemas in one contract name the same four words. `DealStatusCardVerdict`
// is where a deal's standing is DECIDED — compose/dealstatus writes it and the
// deal page prints it. `WorklistDealVerdict` is where the queue CARRIES that
// answer onto a row. A word on one side only is a real defect with no failing
// assertion of its own: a fifth standing added to the card reaches the queue's
// mapper, is refused as unrecognised, and every row about such a deal quietly
// loses its verdict while every test stays green.
//
// The card's side is deliberately not an enum — its own description says so, and
// names DealRoomParticipantCapability as the precedent — so the four words live
// in its PROSE, one per line, each written as a backticked word followed by an
// em dash. That is the shape this reads.
//
// THERE ARE THREE SPELLINGS, NOT TWO, and comparing only the two schemas would
// leave the one that actually drops a row uncovered. attention/dealstanding.go's
// knownStanding switch is the third: it decides at RUNTIME whether a word
// reaches the wire, and a word missing from it is dropped whatever the two
// schemas agree. So this reads all three — the card's prose, the queue's enum,
// and the switch's case arms out of the Go AST — and requires them equal.
//
// A GATE THAT HARD-CODED THE FOUR WOULD BE THE SECOND COPY it exists to prevent
// (AGENTS.md, "Reuse before you build" rule 5), so no list is written here at
// all. What IS written down is the SHAPE of the card's prose, which is the one
// thing a parser cannot derive. That is also the one way this can fail short —
// a reworded description parses to fewer words, all three sets shrink together,
// and the comparison passes over a smaller subject. The second test is the
// guard against exactly that, and it is not a magic number: it takes its floor
// from the switch, which is code and cannot be reworded by accident.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheQueueCarriesEveryStandingTheDealCardCanDecide(t *testing.T) {
	t.Parallel()
	decided := standingsTheCardDecides(t)
	carried := standingsTheQueueCarries(t)
	served := standingsTheMapperServes(t)

	if !slices.Equal(decided, carried) {
		t.Errorf("the deal card decides %v and the worklist row carries %v.\n"+
			"align: backend/api/crm.yaml — WorklistDealVerdict.standing must hold exactly the words "+
			"DealStatusCardVerdict.standing describes. A word the card decides and the row does not carry "+
			"is a standing no client can draw; a word the row carries and the card never decides is a "+
			"value nothing writes.",
			decided, carried)
	}
	if !slices.Equal(decided, served) {
		t.Errorf("the deal card decides %v and attention/dealstanding.go's knownStanding serves %v.\n"+
			"align: add the missing case arm. A word the card decides and the switch does not name falls "+
			"to its default and is DROPPED, so every row about such a deal loses its verdict while both "+
			"schemas agree and every other test stays green — which is the exact failure this gate exists "+
			"to make loud.",
			decided, served)
	}
}

// The census must not be able to pass by reading nothing.
//
// A description reworded so the parse finds fewer would shrink every set
// together and the comparison above would pass over a smaller subject — the
// under-recognition AGENTS.md rule 8 names: it reads less, reports PASS, and
// there is no failing assertion to notice.
//
// The floor is taken from the SWITCH rather than written here as a number. Prose
// can be reworded by an author who never thinks about this gate; a case arm
// cannot be lost without editing Go, and a genuine fifth standing raises the
// floor by itself. So the two tests fail in opposite directions and neither can
// be satisfied by reading nothing.
func TestTheStandingParseSeesEveryWordTheMapperNames(t *testing.T) {
	t.Parallel()
	decided := standingsTheCardDecides(t)
	served := standingsTheMapperServes(t)

	if len(served) == 0 {
		t.Fatal("parsed no case arms out of knownStanding: the switch's shape changed and this gate " +
			"is now reading nothing. Fix the parse before trusting any result above.")
	}
	if len(decided) < len(served) {
		t.Fatalf("parsed %d standings out of DealStatusCardVerdict.standing (%v) but the mapper names %d "+
			"(%v). The description's shape changed: this reads lines of the form \"`word` — what it "+
			"means\". Fix the parse here rather than letting the census pass on a smaller set.",
			len(decided), decided, len(served), served)
	}
}

// standingsTheMapperServes reads the case arms of attention/dealstanding.go's
// knownStanding out of the AST.
//
// The AST rather than a text scan, because a scan for the constant names would
// match them in the doc comment above the function and in any test that
// mentions them, and a gate that can match its own prose is not reading the
// code.
func standingsTheMapperServes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(),
		"internal/compose/attention/dealstanding.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the mapper: %v", err)
	}
	names := constantsInKnownStandingCases(file)
	if len(names) == 0 {
		return nil
	}
	// The switch names generated constants (WorklistStandingLive); the schemas
	// name the wire words (live). Resolved through the generated package rather
	// than by lowercasing the identifier, which would be this gate inventing a
	// fourth spelling of the mapping.
	words := make([]string, 0, len(names))
	for _, name := range names {
		word, ok := standingConstants[name]
		if !ok {
			t.Fatalf("knownStanding names %s, which this gate cannot resolve to a wire word. "+
				"Add it to standingConstants beside the others.", name)
		}
		words = append(words, string(word))
	}
	sort.Strings(words)
	return words
}

// standingConstants maps each generated constant to the word it carries.
//
// Written as the CONSTANTS rather than as string literals, so the values come
// from the generated contract and a renamed member is a compile error here.
var standingConstants = map[string]crmcontracts.WorklistDealVerdictStanding{
	"WorklistStandingLive":     crmcontracts.WorklistStandingLive,
	"WorklistStandingDrifting": crmcontracts.WorklistStandingDrifting,
	"WorklistStandingBlocked":  crmcontracts.WorklistStandingBlocked,
	"WorklistStandingCold":     crmcontracts.WorklistStandingCold,
}

// constantsInKnownStandingCases walks knownStanding's switch and collects the
// identifier named by each case arm.
func constantsInKnownStandingCases(file *ast.File) []string {
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "knownStanding" {
			return true
		}
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			clause, ok := inner.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if sel, ok := expr.(*ast.SelectorExpr); ok {
					found = append(found, sel.Sel.Name)
				}
			}
			return true
		})
		return false
	})
	sort.Strings(found)
	return found
}

// standingWord matches one described standing: a backticked word opening a line,
// followed by the em dash that introduces what it means. Anchored to the line
// start so a backticked word used mid-sentence — the description cites
// DealRoomParticipantCapability that way — is not mistaken for a fifth value.
var standingWord = regexp.MustCompile("(?m)^\\s*`([a-z_]+)`\\s+—")

// standingsTheCardDecides reads the four words out of the deal card verdict's
// prose, where they live because that property is deliberately a plain string.
func standingsTheCardDecides(t *testing.T) []string {
	t.Helper()
	described := crmSchemaProperty(t, "DealStatusCardVerdict", "standing").Description
	found := []string{}
	for _, match := range standingWord.FindAllStringSubmatch(described, -1) {
		found = append(found, match[1])
	}
	sort.Strings(found)
	return slices.Compact(found)
}

// standingsTheQueueCarries reads the enum the worklist row declares.
func standingsTheQueueCarries(t *testing.T) []string {
	t.Helper()
	carried := slices.Clone(crmSchemaProperty(t, "WorklistDealVerdict", "standing").Enum)
	sort.Strings(carried)
	return carried
}

// crmSchemaProperty answers one property of one schema in the public contract.
type crmProperty struct {
	Description string   `yaml:"description"`
	Enum        []string `yaml:"enum"`
}

func crmSchemaProperty(t *testing.T, schema, property string) crmProperty {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]crmProperty `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	found, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("the contract declares no schema %s", schema)
	}
	prop, ok := found.Properties[property]
	if !ok {
		t.Fatalf("%s declares no property %s", schema, property)
	}
	return prop
}
