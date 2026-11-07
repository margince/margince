// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What the stage list discloses, and how its last candidate stage holds when
// two administrators retire at once.

import (
	"errors"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheStageListCountsOnlyPartnersTheReaderMaySee(t *testing.T) {
	e := setupDedupe(t)
	admin := asStageAdmin(e, stageAdminGrants())
	companyID, _ := newPartnerCompany(admin, t, e, "Counted AG", UpsertPartnerInput{PartnerRole: "hosting"})
	if _, err := e.store.UpdateCompany(admin, companyID, UpdateCompanyInput{
		OwnerID: new(ids.From[ids.UserKind](e.rep)), Visibility: new("owner"),
	}); err != nil {
		t.Fatalf("making the partner company private: %v", err)
	}
	countFor := func(objects map[string]principal.ObjectGrant, user ids.UUID) *int {
		t.Helper()
		ctx := principal.WithActor(admin, principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
			Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
		})
		list, err := e.store.ListPartnerStages(ctx)
		if err != nil {
			t.Fatalf("listing the stages: %v", err)
		}
		for _, stage := range list.Data {
			if stage.Key == "research" {
				return stage.PartnerCount
			}
		}
		t.Fatal("no research stage")
		return nil
	}
	reader := map[string]principal.ObjectGrant{"custom_field": {Read: true}, "partner": {Read: true}, "company": {Read: true}}
	if got := countFor(reader, e.rep); got == nil || *got != 1 {
		t.Errorf("the owner's count on research: %v, want 1", got)
	}
	if got := countFor(reader, e.otherRep); got == nil || *got != 0 {
		t.Errorf("a colleague who cannot see the private partner counts %v, want 0", got)
	}
	for name, objects := range map[string]map[string]principal.ObjectGrant{
		"the data-model grant alone":     {"custom_field": {Read: true}},
		"partner read without companies": {"custom_field": {Read: true}, "partner": {Read: true}},
	} {
		if got := countFor(objects, e.rep); got != nil {
			t.Errorf("%s was shown partner_count %d", name, *got)
		}
	}
}

func TestTwoAdministratorsCannotRetireTheLastCandidateStagesTogether(t *testing.T) {
	e := setupDedupe(t)
	keepPartnerStagesAsSeeded(t, e)
	ctx := asStageAdmin(e, stageAdminGrants())
	retired := false
	for _, key := range []string{"contacted", "in_conversation", "fit_confirmed", "agreement_pending"} {
		if _, err := e.store.UpdatePartnerStage(ctx, ids.UUID(stageByKey(ctx, t, e, key).Id), UpdatePartnerStageInput{Active: &retired}); err != nil {
			t.Fatalf("retiring %s: %v", key, err)
		}
	}
	last := []ids.UUID{ids.UUID(stageByKey(ctx, t, e, "research").Id), ids.UUID(stageByKey(ctx, t, e, "identified").Id)}
	errs := make([]error, len(last))
	var start, done sync.WaitGroup
	start.Add(1)
	for i, id := range last {
		done.Go(func() {
			start.Wait()
			_, errs[i] = e.store.UpdatePartnerStage(asStageAdmin(e, stageAdminGrants()), id, UpdatePartnerStageInput{Active: &retired})
		})
	}
	start.Done()
	done.Wait()
	refused := 0
	for _, err := range errs {
		switch {
		case errors.Is(err, apperrors.ErrConflict):
			refused++
		case err != nil:
			t.Fatalf("a retirement failed for another reason: %v", err)
		}
	}
	if refused != 1 {
		t.Errorf("%d of the two concurrent retirements were refused, want exactly 1", refused)
	}
}
