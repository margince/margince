// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func TestRenderWritesEveryStatedField(t *testing.T) {
	src, err := Render("aa", messaging.Rules{
		Jurisdiction: "aa", Version: 2, ReplyWindow: 90 * time.Minute,
		Instruments:  []messaging.Instrument{{Name: "Act A", EffectiveFrom: time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)}},
		Citations:    []messaging.Citation{{Code: "frequency_cap_reached", Territory: messaging.SenderHere, Instrument: "Act A"}},
		FrequencyCap: &messaging.FrequencyCap{Messages: 3, Window: 24 * time.Hour},
		Disclosures:  []messaging.Disclosure{{Kind: messaging.ObjectionRoute, Means: []messaging.ContactMeans{messaging.MeansWeb}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package aa", `Jurisdiction: "aa"`, "ReplyWindow: 90 * time.Minute",
		"time.Date(2020, time.October, 1, 0, 0, 0, 0, time.UTC)", `Territory:  "sender_here"`,
		"&messaging.FrequencyCap{", `[]messaging.ContactMeans{"web"}`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("the generated source lacks %q:\n%s", want, src)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "gen.go", src, 0)
	if err != nil {
		t.Fatalf("the generated source does not parse: %v", err)
	}
	var citations int
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, isLit := n.(*ast.CompositeLit); isLit {
			if array, isArray := lit.Type.(*ast.ArrayType); isArray {
				if sel, isSel := array.Elt.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Citation" {
					citations += len(lit.Elts)
				}
			}
		}
		return true
	})
	if citations != 1 {
		t.Errorf("the source holds %d citation literals a scan can read, want 1", citations)
	}
}

func TestRenderNeedsNoTimeImportForRulesWithoutTime(t *testing.T) {
	src, err := Render("aa", messaging.Rules{Jurisdiction: "aa", Version: 1, ConsentOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `"time"`) {
		t.Errorf("the source imports time and uses none:\n%s", src)
	}
}
