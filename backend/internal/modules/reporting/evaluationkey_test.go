// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"encoding/json"
	"reflect"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// receiptEvaluation is one forecast reading with a target, the shape every case
// below changes one part of.
func receiptEvaluation() Evaluation {
	value, target, money := float64(21600000), float64(30000000), int64(21600000)
	revision := int64(1)
	language := "en"
	deal := ids.MustParse("0192f7a0-0000-7000-8000-000000000001")
	return Evaluation{
		Result: crmcontracts.ReportingEvaluation{
			Context: crmcontracts.ReportingContext{FrameworkRevision: 1, Scope: crmcontracts.ReportingScope{Kind: "workspace"}, ProseLanguage: &language},
			Metrics: []crmcontracts.ReportingMetric{{Id: "forecast_landing", Version: "1", Unit: "money", Value: &value, Target: &target, TargetRevision: &revision}},
			Charts:  []crmcontracts.ReportingChart{{Metric: "forecast_landing", Kind: "forecast_support", Points: []crmcontracts.ReportingPoint{{Key: "forecast_won", Label: "Won", Value: &value}}}},
		},
		Facts: []Fact{{Metric: "forecast_landing", ContextID: "forecast_won", SourceType: "deal", SourceID: deal, Money: &money, Row: crmcontracts.ReportingEvidenceRow{Key: deal.String(), Label: "Forecast contribution", Value: &value}}},
	}
}

func receiptKey(t *testing.T, evaluation Evaluation) string {
	t.Helper()
	key, err := evaluationKey(evaluation)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAReceiptChangesWithEveryFigureScopeTargetAndDefinition(t *testing.T) {
	original := receiptKey(t, receiptEvaluation())
	for name, change := range map[string]func(*Evaluation){
		"contribution": func(e *Evaluation) { more := int64(1); e.Facts[0].Money = &more },
		"scope": func(e *Evaluation) {
			owner := openapi_types.UUID(ids.NewV7())
			e.Result.Context.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: &owner}
		},
		"target":            func(e *Evaluation) { other := float64(1); e.Result.Metrics[0].Target = &other },
		"target revision":   func(e *Evaluation) { next := int64(2); e.Result.Metrics[0].TargetRevision = &next },
		"metric definition": func(e *Evaluation) { e.Result.Metrics[0].Version = "2" },
		"framework":         func(e *Evaluation) { e.Result.Context.FrameworkRevision = 2 },
	} {
		changed := receiptEvaluation()
		change(&changed)
		if receiptKey(t, changed) == original {
			t.Errorf("a changed %s kept the receipt, so evidence would mix two readings", name)
		}
	}
}

func TestAReceiptIgnoresTheLanguageItIsWorded(t *testing.T) {
	original := receiptKey(t, receiptEvaluation())
	german := receiptEvaluation()
	language := "de"
	german.Result.Context.ProseLanguage = &language
	german.Result.Charts[0].Points[0].Label = "Gewonnen"
	german.Facts[0].Row.Label = "Prognosebeitrag"
	if receiptKey(t, german) != original {
		t.Fatal("the same figures worded in German changed the receipt")
	}
}

// A reader may return its rows in any order, so the receipt hashes facts and
// points as sets. The response keeps the order a chart draws.
func TestAReceiptIgnoresTheOrderOfFactsPointsAndMembers(t *testing.T) {
	ordered := receiptEvaluation()
	second := ordered.Facts[0]
	second.SourceID, second.Row.Key = ids.MustParse("0192f7a0-0000-7000-8000-000000000002"), "0192f7a0-0000-7000-8000-000000000002"
	ordered.Facts = append(ordered.Facts, second)
	upside := float64(5)
	ordered.Result.Charts[0].Points = append(ordered.Result.Charts[0].Points, crmcontracts.ReportingPoint{Key: "forecast_upside", Value: &upside})
	ordered.Result.Context.MemberIds = []openapi_types.UUID{openapi_types.UUID(ids.NewV7()), openapi_types.UUID(ids.NewV7())}
	reversed := ordered
	reversed.Facts = []Fact{ordered.Facts[1], ordered.Facts[0]}
	reversed.Result.Charts = []crmcontracts.ReportingChart{ordered.Result.Charts[0]}
	reversed.Result.Charts[0].Points = []crmcontracts.ReportingPoint{ordered.Result.Charts[0].Points[1], ordered.Result.Charts[0].Points[0]}
	reversed.Result.Context.MemberIds = []openapi_types.UUID{ordered.Result.Context.MemberIds[1], ordered.Result.Context.MemberIds[0]}
	if receiptKey(t, reversed) != receiptKey(t, ordered) {
		t.Fatal("the same facts, points and members in another order changed the receipt")
	}
	if ordered.Result.Charts[0].Points[0].Key != "forecast_won" {
		t.Fatal("hashing reordered the points of the response")
	}
}

// Two facts on one source tie on every id; the receipt still has one answer
// whichever comes first, and still tells their amounts apart.
func TestAReceiptOrdersFactsThatTieOnTheirIds(t *testing.T) {
	tied := receiptEvaluation()
	other := tied.Facts[0]
	more := int64(99)
	other.Money = &more
	tied.Facts = append(tied.Facts, other)
	swapped := tied
	swapped.Facts = []Fact{tied.Facts[1], tied.Facts[0]}
	if receiptKey(t, swapped) != receiptKey(t, tied) {
		t.Fatal("two facts tied on their ids hashed by arrival order")
	}
	changed := receiptEvaluation()
	changed.Facts = append(changed.Facts, changed.Facts[0])
	if receiptKey(t, changed) == receiptKey(t, tied) {
		t.Fatal("tied facts with different amounts gave one receipt")
	}
}

func TestAReceiptReadsANilListAsAnEmptyOne(t *testing.T) {
	empty := receiptEvaluation()
	empty.Result.Context.MemberIds = []openapi_types.UUID{}
	empty.Result.Selection.Blocks = []crmcontracts.ReportingBlockKind{}
	empty.Result.Charts[0].Points = []crmcontracts.ReportingPoint{}
	none := receiptEvaluation()
	none.Result.Context.MemberIds, none.Result.Selection.Blocks, none.Result.Charts[0].Points = nil, nil, nil
	if receiptKey(t, none) != receiptKey(t, empty) {
		t.Fatal("a nil list and an empty one gave two receipts")
	}
}

// The receipt hashes a fact's plain encoding. That is canonical only while a
// fact holds no list or map, since a nil and an empty one encode two ways.
func TestAFactHoldsNoListTheReceiptWouldHashTwoWays(t *testing.T) {
	var walk func(reflect.Type, string)
	walk = func(shape reflect.Type, path string) {
		switch shape.Kind() {
		case reflect.Pointer:
			walk(shape.Elem(), path)
		case reflect.Slice, reflect.Map:
			if shape != reflect.TypeFor[json.RawMessage]() {
				t.Errorf("%s is a %s; hash facts through canonicalResult's pruning first", path, shape.Kind())
			}
		case reflect.Struct:
			for field := range shape.Fields() {
				if field.IsExported() {
					walk(field.Type, path+"."+field.Name)
				}
			}
		}
	}
	walk(reflect.TypeFor[Fact](), "Fact")
}
