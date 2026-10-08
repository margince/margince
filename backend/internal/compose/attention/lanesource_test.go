// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A withheld lane is named by a source the queue knows, and this is what holds
// it. The lanes are read off the contract's own enum rather than listed here,
// so a lane added to the contract meets this test before it reaches a page.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// doneForYou is a receipt of work already finished: it puts no row on the
// queue, so it has no queue source to be named by.
const doneForYou = crmcontracts.AttentionLanesOmittedDoneForYou

func TestEveryWithheldLaneIsNamedByAQueueSource(t *testing.T) {
	t.Parallel()

	lanes := declaredLanes(t)
	if len(lanes) < 20 {
		t.Fatalf("read %d lanes off the contract, want every AttentionLanesOmitted value — the scan has gone blind", len(lanes))
	}
	for _, lane := range lanes {
		if lane == doneForYou {
			continue
		}
		source := crmcontracts.WorklistItemSource(sourceOfLane(lane))
		if !source.Valid() {
			t.Errorf("a withheld %q lane is named %q, which no queue row carries — the readings and categories "+
				"match on the row's source, so its zero would read as a measurement", lane, source)
		}
	}
}

// declaredLanes reads every constant the generated contract declares of type
// AttentionLanesOmitted.
func declaredLanes(t *testing.T) []crmcontracts.AttentionLanesOmitted {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../../contracts/api_gen.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the generated contract: %v", err)
	}
	var lanes []crmcontracts.AttentionLanesOmitted
	for _, decl := range file.Decls {
		group, isGroup := decl.(*ast.GenDecl)
		if !isGroup || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			if typed, isIdent := value.Type.(*ast.Ident); !isIdent || typed.Name != "AttentionLanesOmitted" {
				continue
			}
			for _, literal := range value.Values {
				text, isString := literal.(*ast.BasicLit)
				if !isString || text.Kind != token.STRING {
					continue
				}
				word, err := strconv.Unquote(text.Value)
				if err != nil {
					t.Fatalf("unquoting %s: %v", text.Value, err)
				}
				lanes = append(lanes, crmcontracts.AttentionLanesOmitted(word))
			}
		}
	}
	return lanes
}
