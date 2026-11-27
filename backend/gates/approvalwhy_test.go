// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// Every approval says why it was staged: the reason in words, or
// the quoted evidence it rests on. A card with neither leads with its headline alone.
//
// It reads each approvals.StageInput literal that names a Summary, the shape
// every producer stages through. The literal itself sets Why or Evidence.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// approvalWhyFloor is fewer producers than the tree holds, so a renamed input
// fails here rather than reading clean over nothing.
const approvalWhyFloor = 20

// noticeWhyFloor is the same guard for the notices raised.
const noticeWhyFloor = 12

// approvalWhyWaived names the producers that cannot say why yet.
var approvalWhyWaived = gatekit.Waive(map[string]string{})

// TestEveryNoticeNamesWhatItIsAbout holds notices to the same rule: a notice
// links its record or says in its body what happened.
func TestEveryNoticeNamesWhatItIsAbout(t *testing.T) {
	t.Parallel()
	var raised int
	err := gatekit.WalkDir("internal", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, parseErr := gatekit.ParseFile(p, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		for _, lit := range compositesNamed(file, "NewNotice") {
			raised++
			if keys := literalKeys(lit); !keys["Target"] && !keys["Body"] {
				t.Errorf("%s: this notice has neither a Target nor a Body, so its reader learns neither the record "+
					"nor what happened to it", gatekit.SourceFileSet().Position(lit.Pos()))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	if raised < noticeWhyFloor {
		t.Fatalf("found %d notices raised, below the floor of %d: the census lost its subject", raised, noticeWhyFloor)
	}
}

func TestEveryStagedApprovalCarriesAReason(t *testing.T) {
	t.Parallel()
	defer approvalWhyWaived.AssertAllMatched(t)
	var producers int
	err := gatekit.WalkDir("internal", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") ||
			slices.Contains(strings.Split(filepath.ToSlash(p), "/"), "testdata") {
			return nil
		}
		file, parseErr := gatekit.ParseFile(p, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		staged, unreasoned := stagedWithoutReason(file)
		producers += staged
		for _, lit := range unreasoned {
			if !approvalWhyWaived.Waived(t, filepath.ToSlash(p)) {
				t.Errorf("%s: this approval is staged with neither Why nor Evidence, so its card cannot say "+
					"why it exists. Set Why to the trigger in words that name who and which record, or "+
					"Evidence to the quote it rests on", gatekit.SourceFileSet().Position(lit.Pos()))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	if producers < approvalWhyFloor {
		t.Fatalf("found %d staged approvals, below the floor of %d: the census lost its subject", producers, approvalWhyFloor)
	}
}

// compositesNamed finds the composite literals of the type named name.
func compositesNamed(node ast.Node, name string) []*ast.CompositeLit {
	var found []*ast.CompositeLit
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.CompositeLit); ok {
			if sel, ok := lit.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
				found = append(found, lit)
			} else if ident, ok := lit.Type.(*ast.Ident); ok && ident.Name == name {
				found = append(found, lit)
			}
		}
		return true
	})
	return found
}

// stagedWithoutReason counts the StageInput literals in file that stage
// something; an empty one is an error return. It lists those that set neither
// Why nor Evidence.
func stagedWithoutReason(file *ast.File) (int, []*ast.CompositeLit) {
	var staged int
	var unreasoned []*ast.CompositeLit
	for _, lit := range compositesNamed(file, "StageInput") {
		keys := literalKeys(lit)
		if !keys["Summary"] {
			continue
		}
		staged++
		if !keys["Why"] && !keys["Evidence"] {
			unreasoned = append(unreasoned, lit)
		}
	}
	return staged, unreasoned
}

// A reason set on one input does not cover another staged beside it.
func TestEachStagedApprovalCarriesItsOwnReason(t *testing.T) {
	t.Parallel()
	const planted = `package planted

func stage() {
	first := approvals.StageInput{Summary: "a", Why: approvals.HeadlineSaysWhy}
	second := approvals.StageInput{Summary: "b"}
	second.Why = first.Why
	_ = approvals.StageInput{}
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", planted, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	staged, unreasoned := stagedWithoutReason(file)
	if staged != 2 || len(unreasoned) != 1 || literalKeys(unreasoned[0])["Why"] {
		t.Errorf("staged %d with %d unreasoned, want 2 with the second alone unreasoned", staged, len(unreasoned))
	}
}

func literalKeys(lit *ast.CompositeLit) map[string]bool {
	keys := map[string]bool{}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok {
				keys[ident.Name] = true
			}
		}
	}
	return keys
}
