// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Which RECORD TYPES a scrub verb is ever written against.
//
// scrubverbs_test.go asks the neighbouring question — which verbs certify a
// scrub — and derives its corpus from privacy's writers so a new one cannot
// arrive unjudged. This asks what those verbs are written ABOUT, because a
// reader outside privacy has to decide whether the boundary is live for the
// records it reads.
//
// The weekly review is such a reader. Its deal block reconstructs a deal's state
// as of the week's closing instant by reverse-applying audit images, and it
// stops at a scrub tombstone: the spine is append-only, so the images a scrub
// certified gone are still physically there, and folding one back would put
// erased data onto a rep's scorecard. Today no writer in the tree tombstones a
// `deal`, so that boundary excludes nothing and the reader's shortfall count is
// always zero.
//
// THAT IS THE FACT THIS GATE PINS, and the reason it is a gate rather than a
// comment. "No deal is ever scrubbed" is true today and invisible if it stops
// being true: deal erasure would land, the weekly's boundary would quietly start
// firing, and nobody would have decided that a rep's frozen week may now report
// a shortfall. The census fails on that day and asks for the decision.
//
// It fails in the useful direction. A record type ADDED to the scrub set breaks
// this; one removed does not, because a shrinking set cannot surprise a reader
// that already handles the boundary.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// scrubTombstoneWriters are the call sites that stamp a scrub tombstone.
//
// Named rather than pattern-matched on the verb, because the verb travels as a
// constant (actionErase) at some sites and a literal at others, and matching the
// FUNCTION is what makes both visible. A new writer that calls neither is the
// gap this cannot see; scrubverbs_test.go covers the verb side of that.
var scrubTombstoneWriters = map[string]bool{
	// Carries the verb itself; every call is a scrub.
	"tombstoneCollateralScrubs": true,
	// Generic audit doors, which are scrub sites only when the verb they are
	// handed is one.
	"AuditWithEvidence": true,
	"Audit":             true,
	"AuditEvent":        true,
}

// scrubbedEntityTypes is what the tree tombstones today, and the list a reader
// of the erasure boundary may rely on.
//
// `deal` is deliberately ABSENT and that absence is load-bearing — see the file
// comment. Adding it here is a product decision about frozen weekly reviews, not
// a test fix.
var scrubbedEntityTypes = map[string]bool{
	"person":                true,
	"lead":                  true,
	"activity":              true,
	"attachment":            true,
	"deal_room_participant": true,
	"scheduled_send":        true,
	// The review over a message the erasure just cancelled. It rides in beside
	// scheduled_send because it is the same act: the held message is emptied
	// and its review is closed in one transaction, so a reader that stops at
	// one stops at the other.
	//
	// THE NAMED READER IS UNAFFECTED. The weekly review's deal block
	// reconstructs a DEAL from its own audit images (compose/weekly), and a
	// review is not a deal and never appears in that reconstruction — so a
	// review tombstone cannot turn a frozen week into a shortfall it never
	// reported.
	//
	// The other readers of the boundary (compose/undoabilitypage.go,
	// recordrestoreseam.go, magic/done.go, export.go) are each parameterised by
	// the entity the caller named, so this tombstone is only ever seen by
	// somebody asking about that review — and no surface offers undo or restore
	// on one. The decider's queue and the review endpoints read the row
	// directly and never reverse-apply an image.
	"communication_review":  true,
	"voice_learning_signal": true,
	"ai_call":               true,
	"ai_call_payload":       true,
}

func TestEveryScrubbedRecordTypeIsOneAReaderOfTheBoundaryExpects(t *testing.T) {
	t.Parallel()
	found, opaque := scrubbedTypesInPrivacy(t)
	for _, site := range opaque {
		t.Errorf("%s stamps a scrub tombstone whose record type is not a literal, so "+
			"this census cannot read it and would pass without ever seeing it.\n"+
			"Spell the entity type at the call site, or extend scrubbedTypesInPrivacy "+
			"to resolve it.", site)
	}
	if len(found) == 0 {
		t.Fatal("no scrub tombstone writer found in the privacy module — this census " +
			"reads a smaller tree than it thinks, which is the one way it must not fail")
	}

	var unexpected []string
	for entity := range found {
		if !scrubbedEntityTypes[entity] {
			unexpected = append(unexpected, entity)
		}
	}
	sort.Strings(unexpected)
	for _, entity := range unexpected {
		t.Errorf("privacy writes a scrub tombstone against %q, which this census does "+
			"not list.\n\nEvery reader that reverse-applies audit images for %[1]q now "+
			"stops at that tombstone. Decide what each should do, then add %[1]q here.\n"+
			"The weekly review's deal block is one such reader "+
			"(compose/weekly/weeklyweekend.go): a %[1]q scrub makes a rep's frozen week "+
			"report a shortfall it never reported before.", entity)
	}
}

// scrubbedTypesInPrivacy reads the entity-type argument of every scrub
// tombstone writer in the privacy module.
//
// The entity type is the writer's third argument at the storekit sites and its
// third at tombstoneCollateralScrubs. A non-literal argument is skipped and
// reported by the emptiness check above rather than guessed at.
func scrubbedTypesInPrivacy(t *testing.T) (map[string]bool, []string) {
	t.Helper()
	found := map[string]bool{}
	var opaque []string
	dir := filepath.Join(moduleRoot(t), "internal", "modules", "privacy")
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the privacy module: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		var insideCollateralWriter bool
		ast.Inspect(file, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				insideCollateralWriter = fn.Name.Name == "tombstoneCollateralScrubs"
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || !isScrubTombstoneCall(call) {
				return true
			}
			// A tombstone site whose entity type is not a literal cannot be read
			// here, and skipping it silently is the way this census fails short:
			// it would report PASS having never seen the argument. Such a site is
			// reported instead, so the choice is explicit — spell the type at the
			// call, or teach this walk to follow it.
			// The forwarding call INSIDE tombstoneCollateralScrubs passes its own
			// parameter, which is opaque here by construction; its callers name
			// the type and those are the sites this census reads. Reporting the
			// definition would be a permanent false positive.
			if !insideCollateralWriter && !hasLiteralEntityArgument(call) {
				opaque = append(opaque, fset.Position(call.Pos()).String())
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, unquoteErr := strconv.Unquote(lit.Value)
				// Recorded whether or not the census expects it. Filtering to the
				// known set here would make an unexpected type impossible to find,
				// which is a census reporting PASS by reading a smaller tree than
				// its subject.
				if unquoteErr == nil && looksLikeAnEntityType(value) {
					found[value] = true
				}
			}
			return true
		})
	}
	return found, opaque
}

// hasLiteralEntityArgument reports whether this tombstone call names its record
// type as a string literal — the only shape this walk can read.
func hasLiteralEntityArgument(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		if value, err := strconv.Unquote(lit.Value); err == nil && looksLikeAnEntityType(value) {
			return true
		}
	}
	return false
}

// scrubVerbConstants are privacy's own names for the three verbs that certify a
// scrub. scrubverbs_test.go holds the list itself against the module's writers;
// this names the identifiers so a call passing one is recognised.
var scrubVerbConstants = map[string]bool{
	"actionErase": true, "actionAnonymize": true, "actionRestrict": true,
}

// looksLikeAnEntityType separates a record type from the other string literals
// a tombstone writer carries — the verb itself, and the reason and cause the
// evidence map holds.
//
// Shape rather than an allowlist: a lowercase snake_case word IS how every
// entity type in this tree is spelled, and matching the shape means a type
// nobody anticipated still lands in the census. The verbs are excluded by name
// because they share that shape.
func looksLikeAnEntityType(value string) bool {
	if value == "" || value == "erase" || value == "anonymize" || value == "restrict" {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && r != '_' {
			return false
		}
	}
	return true
}

// isScrubTombstoneCall reports whether this call stamps a scrub tombstone.
//
// Two shapes, and the second is why this is not simply "a call carrying a scrub
// verb". tombstoneCollateralScrubs holds the verb INSIDE itself — every call to
// it is a scrub by construction, and its arguments name only the record type —
// so a check that demanded a verb argument would skip precisely the writer that
// stamps five of the tree's record types.
func isScrubTombstoneCall(call *ast.CallExpr) bool {
	name := calleeName(call)
	if !scrubTombstoneWriters[name] {
		return false
	}
	if name == "tombstoneCollateralScrubs" {
		return true
	}
	for _, arg := range call.Args {
		switch a := arg.(type) {
		case *ast.Ident:
			// The three scrub verb constants BY NAME. A prefix match on "action"
			// would also admit actionArchive, which certifies no scrub — and a
			// retention policy's own archive row would then be read as one.
			if scrubVerbConstants[a.Name] {
				return true
			}
		case *ast.BasicLit:
			if a.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(a.Value)
			if err == nil && (value == "erase" || value == "anonymize" || value == "restrict") {
				return true
			}
		}
	}
	return false
}
