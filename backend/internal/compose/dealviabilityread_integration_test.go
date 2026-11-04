// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The deal page's viability read agrees with the forecast. It keeps showing an
// open review after its task is done, because completing the task is no answer.

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// viabilityAs reads one deal's viability as the caller in ctx.
func (v *viabilityEnv) viabilityAs(ctx context.Context, deal ids.UUID) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/deals/"+deal.String()+"/viability", nil).WithContext(ctx)
	dealViabilityHandlers{pool: v.Pool}.GetDealViability(rec, req, crmcontracts.Id(deal))
	return rec
}

func (v *viabilityEnv) viabilityOf(t *testing.T, deal ids.UUID) crmcontracts.DealViability {
	t.Helper()
	rec := v.viabilityAs(v.Admin(), deal)
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

// forecastRep is a team rep who may read the forecast.
func forecastRep() principal.Permissions {
	perms := integration.AccountRepPerms
	perms.Objects = maps.Clone(integration.AccountRepPerms.Objects)
	perms.Objects["forecast"] = principal.ObjectGrant{Read: true}
	return perms
}

func TestDealViabilityReadAdmitsWhomTheDealPageAdmitsAndNeedsForecastRead(t *testing.T) {
	v := enforcedViability(t)
	// A deal is an identity table every seat with deal:read reads, so the deal
	// page shows it to a rep outside its team. The viability read admits the
	// same callers in its own transaction, and no more.
	ownOnly := forecastRep()
	ownOnly.RowScope = principal.RowScopeOwn
	outsider := v.As(v.Rep3, []ids.UUID{v.Team2}, ownOnly)
	_, pageErr := v.Deals.GetDeal(outsider, ids.From[ids.DealKind](v.silent), storekit.LiveOnly)
	if rec := v.viabilityAs(outsider, v.silent); (rec.Code == http.StatusOK) != (pageErr == nil) {
		t.Errorf("the deal page answers %v and the viability read %d for the same caller; they must agree", pageErr, rec.Code)
	}
	if _, err := v.Deals.ArchiveDeal(v.Admin(), ids.From[ids.DealKind](v.supported), nil); err != nil {
		t.Fatal(err)
	}
	if rec := v.viabilityAs(v.Admin(), v.supported); rec.Code != http.StatusNotFound {
		t.Errorf("an archived deal's viability answered %d, want 404 as the deal page does", rec.Code)
	}
	if rec := v.viabilityAs(v.As(v.Rep1, []ids.UUID{v.Team1}, integration.AccountRepPerms), v.silent); rec.Code != http.StatusForbidden {
		t.Errorf("the deal's owner without forecast read got %d %s, want 403", rec.Code, rec.Body)
	}
	if rec := v.viabilityAs(v.As(v.Rep1, []ids.UUID{v.Team1}, forecastRep()), v.silent); rec.Code != http.StatusOK {
		t.Errorf("the deal's owner with forecast read got %d %s, want 200", rec.Code, rec.Body)
	}
}

func TestDealViabilityShowsTheTaskOnlyWhileItIsLinkedToTheDeal(t *testing.T) {
	v := enforcedViability(t)
	if got := v.viabilityOf(t, v.silent); got.Review == nil || got.Review.Task == nil {
		t.Fatalf("before the relink the review reads %+v, want its task", got.Review)
	}
	v.WsExec(t, `UPDATE activity_link SET deal_id = $1 WHERE deal_id = $2 AND activity_id = (
		SELECT task_activity_id FROM assurance_exception WHERE type = $3 AND subject_id = $2)`,
		v.supported, v.silent, assurance.TypeDealViability)
	if got := v.viabilityOf(t, v.silent); got.Review == nil || got.Review.Task != nil {
		t.Errorf("after the task moved to another deal the review reads %+v, want it without the task", got.Review)
	}
}

func TestDealViabilityNamesAClosedDealAndAStaleCheck(t *testing.T) {
	v := enforcedViability(t)
	v.WsExec(t, `UPDATE assurance_deal_receipt SET valid_until = assessed_at WHERE deal_id = $1`, v.supported)
	stale := v.viabilityOf(t, v.supported)
	if stale.Qualification != crmcontracts.DealViabilityQualificationUnknown || stale.Receipt == nil || stale.Receipt.Current {
		t.Errorf("a lapsed check reads %s with receipt %+v, want unknown and a receipt that no longer counts",
			stale.Qualification, stale.Receipt)
	}
	if q := v.forecast(t).Qualified; q == nil || q.UnknownCount != 1 {
		t.Errorf("the forecast reads %+v, want the lapsed deal unknown too", q)
	}

	lost, err := ids.Parse(v.WsScalar(t, `SELECT s.id::text FROM stage s JOIN deal d ON d.pipeline_id = s.pipeline_id
		WHERE d.id = $1 AND s.semantic = 'lost' AND s.archived_at IS NULL LIMIT 1`, v.supported))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Deals.AdvanceDeal(v.Admin(), ids.From[ids.DealKind](v.supported), deals.AdvanceDealInput{
		ToStageID: ids.From[ids.StageKind](lost), LostReason: new("Chose a competitor"),
	}); err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
	if closed := v.viabilityOf(t, v.supported); closed.Qualification != crmcontracts.DealViabilityQualificationClosed {
		t.Errorf("a lost deal reads %s, want closed: no open reading counts it", closed.Qualification)
	}
}
