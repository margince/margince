// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestStagedToolsUseTheirExecutionScope(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	count := 0
	for _, spec := range registry.Specs() {
		if _, stages := registry.StagerFor(spec.Name); !stages {
			continue
		}
		count++
		if !approvals.KindHasDecisionGrants(spec.Name) {
			t.Errorf("stageable tool %s has no decision grants", spec.Name)
		}
		staged, err := stagedCallSpec(spec.Name)
		if err != nil {
			t.Errorf("stageable tool %s has no admission: %v", spec.Name, err)
			continue
		}
		if !slices.Equal(staged.Areas, spec.Areas) {
			t.Errorf("%s staging areas differ from execution", spec.Name)
		}
		if staged.RequiredScope != spec.RequiredScope {
			t.Errorf("%s staging requires %s; execution requires %s", spec.Name, staged.RequiredScope, spec.RequiredScope)
		}
	}
	if count == 0 {
		t.Fatal("registry exposes no stageable tools")
	}
}

func TestAgentApprovalAdmissionRequiresScopeAndAFullSeat(t *testing.T) {
	actor := principal.Principal{Type: principal.PrincipalAgent}
	ctx := principal.WithActor(context.Background(), actor)
	if err := stageCallAdmission(ctx, approvals.StageInput{Kind: "merge_tags"}); err == nil {
		t.Fatal("scope-less agent admitted")
	}
	actor.Scopes = principal.ScopeSet{principal.ScopeWrite: {}}
	if err := stageCallAdmission(principal.WithActor(ctx, actor), approvals.StageInput{Kind: "merge_tags"}); !errors.Is(err, apperrors.ErrSeatTierInsufficient) {
		t.Fatalf("read seat admitted: %v", err)
	}
}

func TestReplayGrantsAreIncludedInStagingAuthority(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	checked := 0
	for _, spec := range registry.Specs() {
		if spec.ReplayGrant == nil {
			continue
		}
		if _, stages := registry.StagerFor(spec.Name); !stages {
			continue
		}
		checked++
		grants, err := approvals.DecisionGrants(spec.Name, spec.ReplayGrant.Object)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(grants, func(g approvals.DecisionGrant) bool {
			return g.Object == spec.ReplayGrant.Object && g.Action == spec.ReplayGrant.Action
		}) {
			t.Errorf("%s replay grant %s.%s is absent from staging", spec.Name, spec.ReplayGrant.Object, spec.ReplayGrant.Action)
		}
	}
	if checked == 0 {
		t.Fatal("no stageable replay-grant tool checked")
	}
}

func TestStagingChecksThePassportAreaOfTheTargetRecord(t *testing.T) {
	actor := principal.Principal{Type: principal.PrincipalAgent, SeatType: principal.SeatFull, Scopes: principal.ScopeSet{principal.ScopeWrite: {}}, Areas: principal.WorkAreas{Limited: true}}
	area, found := workarea.OfRecordCall("update_record", "contact")
	if !found {
		t.Fatal("contact update has no area")
	}
	key, _ := workarea.SwitchOf(area)
	in := approvals.StageInput{Kind: "update_record", TargetType: "contact"}
	if err := stageCallAdmission(principal.WithActor(context.Background(), actor), in); err == nil {
		t.Fatal("passport with no areas staged a contact update")
	}
	actor.Areas.Chosen = []string{key}
	if err := stageCallAdmission(principal.WithActor(context.Background(), actor), in); err != nil {
		t.Fatalf("passport with the contact area refused: %v", err)
	}
}
