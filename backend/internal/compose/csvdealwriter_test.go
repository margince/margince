// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRefusalTheStoreGivesADealRowIsASkipNotAFailedRun(t *testing.T) {
	fields := map[string]string{dealTargetCloseDate: " 2020-01-01 "}
	for name, tc := range map[string]struct {
		err  error
		want migration.ReasonCode
	}{
		"a past close date":               {&deals.PastCloseDateError{}, migration.ReasonCloseDatePast},
		"an amount below zero":            {&deals.NegativeAmountError{}, migration.ReasonAmountNegative},
		"another pipeline":                {&deals.StagePipelineMismatchError{}, migration.ReasonDealPipelineFixed},
		"no lost reason":                  {&deals.LostReasonRequiredError{}, migration.ReasonLostReasonMissing},
		"no exchange rate":                {&deals.MissingFxRateError{From: "VND", To: "EUR"}, migration.ReasonFxRateMissing},
		"a legal gate":                    {&deals.LegalReviewRequiredError{}, migration.ReasonDealLegalGate},
		"a gate at birth":                 {&deals.LegalGateOnCreateError{}, migration.ReasonDealLegalGate},
		"no open stage":                   {&deals.PipelineHasNoOpenStageError{}, migration.ReasonPipelineNoOpenStage},
		"a stage archived since the read": {fmt.Errorf("advance: %w", &deals.StageGoneError{}), migration.ReasonStageUnknown},
		"the write completed":             {fmt.Errorf("wrapped: %w", &deals.PastCloseDateError{}), migration.ReasonCloseDatePast},
	} {
		why, refused := dealRefusalOf(tc.err, fields, dealStages{})
		if !refused || why.Code != tc.want {
			t.Errorf("%s: reason = %q, refused = %v, want %q", name, why.Code, refused, tc.want)
		}
	}
	if _, refused := dealRefusalOf(errors.New("the database is down"), fields, dealStages{}); refused {
		t.Error("an error the store did not mean as a refusal was answered as a bad row, which would hide a failed run")
	}
	if why, _ := dealRefusalOf(&deals.PastCloseDateError{}, fields, dealStages{}); why.Params["value"] != "2020-01-01" {
		t.Errorf("params = %v, want the date as the file spelled it, trimmed", why.Params)
	}
}

func TestADealRowNamingADifferentCompanyIsAChange(t *testing.T) {
	company := ids.NewV7().String()
	stored := func(id string) []byte {
		raw, err := json.Marshal(map[string]any{"company_id": id, "name": "Deal"})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	same, err := changedFields(stored(company), map[string]string{csvTargetCompanyRef: "CO-1"}, map[string]any{csvTargetCompanyRef: company})
	if err != nil || len(same) != 0 {
		t.Fatalf("changed = %v, %v, want nothing for the company the deal already has", same, err)
	}
	moved, err := changedFields(stored(ids.NewV7().String()), map[string]string{csvTargetCompanyRef: "CO-1"}, map[string]any{csvTargetCompanyRef: company})
	if err != nil || moved[csvTargetCompanyRef] != "CO-1" {
		t.Fatalf("changed = %v, %v, want the company reported as changed", moved, err)
	}
	pending, err := changedFields(stored(company), map[string]string{csvTargetCompanyRef: "CO-1"}, map[string]any{})
	if err != nil || len(pending) != 0 {
		t.Fatalf("changed = %v, %v, want a link that resolved to nothing left alone", pending, err)
	}
	patch := dealUpdateFrom(moved, map[string]any{csvTargetCompanyRef: company})
	if patch.Deal.CompanyID == nil || patch.Deal.CompanyID.String() != company {
		t.Fatalf("patch company = %v, want the resolved company", patch.Deal.CompanyID)
	}
}

func TestAResolvedLinkReachesTheRowsTypedCells(t *testing.T) {
	company := ids.NewV7().String()
	typed := rowLink{links: map[string]any{csvTargetCompanyRef: company}}.into(nil)
	if got, ok := linkedRecord(typed, csvTargetCompanyRef); !ok || got.String() != company {
		t.Fatalf("typed = %v, want the company id carried", typed)
	}
	if _, ok := linkedRecord(typed, dealTargetContactRef); ok {
		t.Error("a contact nobody named was found")
	}
}
