// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A rule whose owner left is dead until someone else owns it. The owner change
// moves the owner and counts as a change of the rule. It refuses a seat the
// caller could not make an owner.
func TestAnAutomationCanBeHandedToAnotherMember(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	store := NewAutomationStore(db)
	created, err := store.Create(ctx, CreateAutomationInput{Key: assignLeadOwnerName, Name: "Assign leads", Params: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	next := ids.From[ids.UserKind](fx.rep2)
	handed, err := store.Update(ctx, created.ID, UpdateAutomationInput{OwnerID: &next})
	if err != nil {
		t.Fatal(err)
	}
	if handed.OwnerID == nil || *handed.OwnerID != next {
		t.Fatalf("owner after the hand-over = %v, want %v", handed.OwnerID, next)
	}
	if handed.Version != created.Version+1 {
		t.Fatalf("version = %d, want %d: an owner change is a change of the rule", handed.Version, created.Version+1)
	}

	again, err := store.Update(ctx, created.ID, UpdateAutomationInput{OwnerID: &next})
	if err != nil || again.Version != handed.Version {
		t.Fatalf("handing to the current owner gave version %d, err %v; want no change", again.Version, err)
	}

	stranger := ids.From[ids.UserKind](ids.NewV7())
	var refused *auth.AssigneeNotAllowedError
	if _, err := store.Update(ctx, created.ID, UpdateAutomationInput{OwnerID: &stranger}); !errors.As(err, &refused) {
		t.Fatalf("handing to a seat that does not exist gave %v, want owner_not_assignable", err)
	}
}

// A rule saved before its kind required a param is still handed on. The new
// owner's grants decide the hand-on; the stored params are not judged again.
func TestAnEnabledRuleSavedUnderOlderParamRulesCanStillBeHandedOn(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	fx.seedAutomationWithOwner(t, noActivityReminderName, fx.rep1)
	var rule ids.AutomationID
	if err := fx.owner.QueryRow(context.Background(), `SELECT id FROM automation WHERE key = $1`, noActivityReminderName).Scan(&rule); err != nil {
		t.Fatal(err)
	}
	store := NewAutomationStore(db).WithPreviewAuthority(fullyPermittedOwner())
	next := ids.From[ids.UserKind](fx.rep2)
	handed, err := store.Update(fx.humanCtx(fx.rep1, principal.RowScopeAll), rule, UpdateAutomationInput{OwnerID: &next})
	if err != nil {
		t.Fatalf("handing on a rule whose stored params predate its kind's rules: %v", err)
	}
	if handed.OwnerID == nil || *handed.OwnerID != next {
		t.Fatalf("owner after the hand-on = %v, want %v", handed.OwnerID, next)
	}
}

// stagesHiddenFrom answers every stage in pipeline, except to hidden, who finds none.
type stagesHiddenFrom struct {
	hidden, pipeline ids.UUID
}

func (s stagesHiddenFrom) StageSemantic(ctx context.Context, _ ids.UUID) (string, ids.UUID, error) {
	if actor, ok := principal.Actor(ctx); ok && actor.UserID == s.hidden {
		return "", ids.Nil, apperrors.ErrNotFound
	}
	return "open", s.pipeline, nil
}

// The new owner of a handed-on rule must be able to use every record it names.
func TestHandingOnAStageRuleToSomeoneWhoCannotSeeItsStagesIsRefused(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	pipeline, stage := ids.NewV7(), ids.NewV7()
	params := fmt.Sprintf(`{"pipeline_id": %q, "stage_ids": [%q]}`, pipeline, stage)
	fx.exec(t, `INSERT INTO automation (id, key, name, trigger, action, params, owner_id, enabled, tier)
		VALUES ($1, $2, $2, '{"event_type":"test"}', '{"kind":"test"}', $3::jsonb, $4, true, 'auto_execute')`,
		ids.New[ids.AutomationKind](), stageChangeNotifyName, params, fx.rep1)
	var rule ids.AutomationID
	if err := fx.owner.QueryRow(context.Background(), `SELECT id FROM automation WHERE key = $1`, stageChangeNotifyName).Scan(&rule); err != nil {
		t.Fatal(err)
	}
	store := NewAutomationStore(db).WithPreviewAuthority(fullyPermittedOwner()).
		WithStageLookup(stagesHiddenFrom{hidden: fx.rep2, pipeline: pipeline})
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)
	next := ids.From[ids.UserKind](fx.rep2)
	if _, err := store.Update(ctx, rule, UpdateAutomationInput{OwnerID: &next}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("handing on to someone who cannot see the rule's stage answered %v, want not found", err)
	}
	kept, err := store.Get(ctx, rule)
	if err != nil || kept.OwnerID == nil || kept.OwnerID.UUID != fx.rep1 {
		t.Fatalf("the refused hand-on moved the rule: %+v, %v", kept.OwnerID, err)
	}
}
