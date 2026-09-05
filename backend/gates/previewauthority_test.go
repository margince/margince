// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The authority levels a composer can receive are the ones the engine can send.
//
// A send preview tells a surface WHOSE decision a refusal is, and the surface
// decides from it whether to offer an override at all. That answer crosses a
// wire: commsauthz.AuthorityLevel on one side, the contract's `decided_by` enum
// on the other.
//
// Two spellings of one vocabulary drift in a way nothing else catches. Add a
// level in Go and the contract keeps validating the old set, so the new level
// reaches a composer as an unrecognised string and whatever the surface does
// with an unknown value becomes the product's answer to "may a rep overrule
// this". Add one to the contract alone and the server can never send it.
//
// The failure direction that matters is the quiet one: a surface treating an
// unrecognised level as overrulable would offer a button on a refusal nobody
// may lift. So this fails in BOTH directions rather than checking containment.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// contractDecidedBy reads the enum the contract publishes for decided_by.
var contractDecidedBy = regexp.MustCompile(
	`(?s)decided_by:\s*\n\s*type: string\s*\n\s*enum: \[([^\]]+)\]`)

// authorityLevelSource declares the levels. Read rather than restated: a list
// here would be a second copy of the vocabulary, and the copy that stopped
// matching is the one nobody notices.
const authorityLevelSource = "internal/shared/ports/commsauthz/authority.go"

// declaredAuthorityLevels collects every AuthorityLevel constant the package
// declares, so a fifth added in Go enters this set without anybody editing the
// gate — which is the whole point, because the failure this file exists to
// catch is a level the contract does not publish.
func declaredAuthorityLevels(t *testing.T) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), authorityLevelSource, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", authorityLevelSource, err)
	}
	levels := map[string]bool{}
	for _, decl := range parsed.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			// Names and values read one-to-one only when they pair up; an iota
			// run or a multi-assign pairs a name with an expression that is not
			// its own, and reading by index would hand this gate the wrong text.
			if !isValue || len(value.Names) != len(value.Values) {
				continue
			}
			named, isNamed := value.Type.(*ast.Ident)
			if !isNamed || named.Name != "AuthorityLevel" {
				continue
			}
			for _, v := range value.Values {
				// gatekit.LiteralText rather than the node's own Value, which is
				// the SOURCE text: quotes still on it and escapes undecoded, so a
				// level written with one would enter this set under a spelling no
				// send ever carries and agree with nothing.
				if text, ok := gatekit.LiteralText(v); ok {
					levels[text] = true
				}
			}
		}
	}
	return levels
}

// TestTheAuthorityLevelsAgreeAcrossTheWire holds the Go vocabulary and the
// published one to each other.
func TestTheAuthorityLevelsAgreeAcrossTheWire(t *testing.T) {
	t.Parallel()

	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	match := contractDecidedBy.FindSubmatch(contract)
	if match == nil {
		t.Fatal("the contract publishes no decided_by enum: either the field was renamed and " +
			"this gate now checks nothing, or the preview stopped saying whose decision a " +
			"refusal is, which is what a surface needs to know whether to offer an override")
	}

	published := map[string]bool{}
	for _, raw := range strings.Split(string(match[1]), ",") {
		if v := strings.TrimSpace(raw); v != "" {
			published[v] = true
		}
	}

	known := declaredAuthorityLevels(t)

	// Under-recognition is the failure that reports PASS: a walk that found no
	// constants, or a regex that matched an empty enum, would agree with each
	// other about nothing. Both sides are floored against the four levels the
	// model shipped with rather than against each other, so an empty pair
	// cannot agree its way to green.
	const shipped = 4
	if len(known) < shipped {
		t.Fatalf("read %d AuthorityLevel constants from %s, want at least the %d the model "+
			"shipped with: the walk has stopped seeing its subject",
			len(known), authorityLevelSource, shipped)
	}
	if len(published) < shipped {
		t.Fatalf("read %d values from the contract's decided_by enum, want at least the %d "+
			"the model shipped with: the pattern has stopped seeing its subject",
			len(published), shipped)
	}

	for level := range known {
		if !published[level] {
			t.Errorf("the engine can answer %q but the contract does not publish it: a composer "+
				"receives a level it cannot interpret, and whatever it does with an unknown "+
				"value becomes the answer to whether a rep may overrule that refusal", level)
		}
	}
	for level := range published {
		if !known[level] {
			t.Errorf("the contract publishes %q but the engine cannot answer it: a surface may "+
				"branch on a level no send will ever carry", level)
		}
	}

	if t.Failed() {
		t.Logf("engine: %v", sortedKeys(known))
		t.Logf("contract: %v", sortedKeys(published))
	}
}
