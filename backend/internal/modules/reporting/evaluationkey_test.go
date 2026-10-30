// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
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
