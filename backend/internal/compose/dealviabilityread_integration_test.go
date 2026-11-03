// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The deal page's viability read agrees with the forecast. It keeps showing an
// open review after its task is done, because completing the task is no answer.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (v *viabilityEnv) viabilityOf(t *testing.T, deal ids.UUID) crmcontracts.DealViability {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/deals/"+deal.String()+"/viability", nil).WithContext(v.Admin())
	dealViabilityHandlers{pool: v.Pool}.GetDealViability(rec, req, crmcontracts.Id(deal))
	if rec.Code != http.StatusOK {
		t.Fatalf("viability read = %d: %s", rec.Code, rec.Body)
	}
	var out crmcontracts.DealViability
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDealViabilityReadShowsReviewAfterTaskDone(t *testing.T) {
	v := enforcedViability(t)
	task := v.WsScalar(t, `SELECT task_activity_id::text FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.silent)
	taskID, err := ids.Parse(task)
	if err != nil {
		t.Fatal(err)
	}
	done := true
	if _, err := v.Activities.UpdateActivity(v.Admin(), ids.From[ids.ActivityKind](taskID),
		activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatalf("completing the review task: %v", err)
	}

	silent := v.viabilityOf(t, v.silent)
	if silent.QualificationMode != crmcontracts.DealViabilityQualificationModeEnforcing ||
		silent.Qualification != crmcontracts.DealViabilityQualificationExcluded {
		t.Fatalf("the silent deal reads %s in mode %s, want excluded while enforcing",
			silent.Qualification, silent.QualificationMode)
	}
	if silent.Review == nil || silent.Review.Task == nil || !silent.Review.Task.Done {
		t.Fatalf("the open review reads %+v, want it shown with its task done", silent.Review)
	}
	if silent.Disposition == nil || *silent.Disposition != crmcontracts.DealViabilityDispositionOpenReview ||
		silent.Receipt == nil || silent.AssessedAt == nil {
		t.Errorf("the basis reads disposition %v receipt %+v assessed %v, want the open review and the check's receipt",
			silent.Disposition, silent.Receipt, silent.AssessedAt)
	}

	supported := v.viabilityOf(t, v.supported)
	if supported.Qualification != crmcontracts.DealViabilityQualificationEligible || supported.Review != nil {
		t.Errorf("the supported deal reads %s with review %+v, want eligible and none", supported.Qualification, supported.Review)
	}
	// The read and the forecast agree on which deal is out.
	if q := v.forecast(t).Qualified; q == nil || q.ExcludedCount != 1 {
		t.Errorf("the forecast excludes %+v, want the one deal the read calls excluded", q)
	}
}
