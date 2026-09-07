// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package gates

// Every door that moves a deal carries the caller's win-evidence claim.
//
// The win gate refuses a close on a stage whose semantic is won when the deal
// has no agreement behind it and the caller named no reason
// (deals/win_evidence.go, win_evidence_required). The refusal is correct, and
// it dead-ends unless the door that staged the move offered the field: an
// assistant told "a win claiming neither is refused" by one tool and given no
// argument for it by another has to know to abandon the tool and re-issue
// through a different one.
//
// This is the shape the seam predicted. datasource.AdvanceDealInput's own
// comment says that without the field on it "an assistant could not answer the
// win gate at all" — and progress_deal, the intent-level composition of
// advance_deal, shipped without it for exactly as long as nothing derived the
// obligation from the type.
//
// Derived from the input type rather than from a list of doors: a function that
// constructs an AdvanceDealInput IS a deal-move door, whatever it is called and
// wherever it lives, so one added tomorrow is judged the day it is written.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// buildsAMove reports whether a composite literal constructs the deal-move
// input, under either spelling — the port's (datasource.AdvanceDealInput) or
// the module's own. The package qualifier differs by caller and carries no
// information this gate needs, so only the type name is matched.
func buildsAMove(lit *ast.CompositeLit) bool {
	name := literalTypeName(lit)
	return name == "AdvanceDealInput" || strings.HasSuffix(name, ".AdvanceDealInput")
}

// winEvidenceDoors reports every function in one file that builds a deal-move
// input, and whether that function carries the caller's win-evidence claim.
//
// The claim is looked for anywhere in the ENCLOSING function rather than inside
// the literal, because a door may set the field after the literal or through a
// helper it names. A gate keyed on the literal's own keys would defend the
// shape of today's code: the send-door census learned that the hard way, and
// three probes walked past its first version.
func winEvidenceDoors(path string, file *ast.File) map[string]bool {
	doors := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		builds, carries := false, false
		ast.Inspect(fn, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CompositeLit:
				if buildsAMove(v) {
					builds = true
				}
			case *ast.Ident:
				if strings.HasPrefix(v.Name, "WonWithoutContract") {
					carries = true
				}
			}
			return true
		})
		if builds {
			doors[path+":"+receiverQualified(fn)] = carries
		}
	}
	return doors
}

func TestEveryDealMoveCarriesTheWinEvidenceClaim(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	doors := map[string]bool{}
	// The whole backend tree: a deal move is wherever somebody builds the
	// input, and the estate import already owns one outside internal/modules.
	for _, root := range []string{"internal", "cmd"} {
		for path, file := range parseTreeFiles(t, fset, root) {
			for door, carries := range winEvidenceDoors(path, file) {
				doors[door] = carries
			}
		}
	}

	// Under-recognition is the one way this gate must not break: a scan that
	// found nothing would report PASS over an empty corpus.
	if len(doors) == 0 {
		t.Fatal("found no deal-move door — the scan is looking in the wrong place")
	}
	for door, carries := range doors {
		if !carries {
			t.Errorf("%s moves a deal and never carries the caller's win-evidence claim — "+
				"a paperless win through this door is refused with no argument that answers it", door)
		}
	}
}

// The bound is only worth having if it fires, and no shipped door breaks it —
// so the failing case has to be built rather than borrowed.
func TestTheWinEvidenceCensusReportsADoorThatDropsTheClaim(t *testing.T) {
	t.Parallel()

	const door = `package p
func moveIt(args a) error {
	_, err := p.AdvanceDeal(ctx, datasource.AdvanceDealInput{
		DealID: args.DealID, ToStageID: args.ToStageID, LostReason: args.LostReason,
	})
	return err
}`
	file, err := parser.ParseFile(token.NewFileSet(), "probe.go", door, 0)
	if err != nil {
		t.Fatalf("parsing the probe: %v", err)
	}
	found := winEvidenceDoors("probe.go", file)
	carries, seen := found["probe.go:moveIt"]
	if !seen {
		t.Fatal("a function building an AdvanceDealInput was not recognised as a deal-move door")
	}
	if carries {
		t.Error("a door that never names the win evidence was reported as carrying the claim, " +
			"so this census would not have found progress_deal")
	}
}

// The tool surface declares the win-evidence arguments in exactly one place.
//
// A second copy is how one deal-move tool starts advertising a reason, a bound
// or a description the other does not — and the two doors then disagree about
// one behaviour while both still pass the census above, which only asks whether
// a door carries the claim at all.
//
// Held for the claim on winEvidenceProperties.
func TestTheToolSurfaceSpellsTheWinEvidenceOnce(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	var declarations []string
	for path, file := range parseTreeFiles(t, fset, "internal/modules/agents") {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || !strings.Contains(literalOf(value), `"won_without_contract_reason"`) {
					continue
				}
				for _, name := range value.Names {
					declarations = append(declarations, path+":"+name.Name)
				}
			}
		}
	}
	if len(declarations) == 0 {
		t.Fatal("found no win-evidence schema in the tool surface — the scan is looking in the wrong place")
	}
	if len(declarations) > 1 {
		t.Errorf("the win evidence is declared %d times (%s) — a second copy is how two deal-move "+
			"tools start advertising different reasons", len(declarations), strings.Join(declarations, ", "))
	}
}
