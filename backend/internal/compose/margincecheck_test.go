// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// droppedOutcomes reads every "dropped:" string constant the verdict can
// store, from the packages that write them.
func droppedOutcomes(t *testing.T) []string {
	t.Helper()
	var found []string
	for _, dir := range []string{".", "dealverdict", "../modules/deals"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err == nil && strings.HasPrefix(value, "dropped:") && !strings.ContainsAny(value, " \"") {
					found = append(found, value)
				}
				return true
			})
		}
	}
	return found
}

func TestEveryStoredDropReachesTheOwnerAsAReasonTheyCanRead(t *testing.T) {
	outcomes := droppedOutcomes(t)
	if len(outcomes) < 20 {
		t.Fatalf("found %d stored drops, want every one the verdict writes: the scan read too little", len(outcomes))
	}
	for _, outcome := range outcomes {
		if checkWhy(outcome) == crmcontracts.DealMarginceCheckOutcomeWhyOther {
			t.Errorf("%s reads as other on the deal card; name it in checkWhys", outcome)
		}
	}
}

func TestAReceiptOfEitherShapeDecodesItsActions(t *testing.T) {
	for name, raw := range map[string]string{
		"bare list": `[{"kind":"set_amount","outcome":"dropped:low_confidence"}]`,
		"record":    `{"for":"x","sources":[],"actions":[{"kind":"set_amount","outcome":"dropped:low_confidence"}]}`,
	} {
		got, err := decodeVerdictRecord([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got.Actions) != 1 || got.Actions[0].Outcome != "dropped:low_confidence" {
			t.Errorf("%s decoded to %+v, want the one dropped amount", name, got)
		}
	}
}
