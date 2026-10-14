// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The refusals of a stored actor that need no database. Each one answers
// before the live authority is read, so a nil identity service never runs.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyAHumanOrAnAgentStoredActorIsActedAs(t *testing.T) {
	t.Parallel()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	for _, kind := range []principal.PrincipalType{principal.PrincipalSystem, principal.PrincipalConnector, ""} {
		stored := migration.StoredActor{Type: kind, ID: "system:import-worker", UserID: ids.NewV7()}
		_, err := storedActor{}.rebuild(ctx, stored)
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("rebuilding a stored %q actor answered %v, want a permission refusal", kind, err)
		}
	}
}

func TestAStoredActorIsNotRebuiltOutsideAWorkspace(t *testing.T) {
	t.Parallel()
	stored := migration.StoredActor{Type: principal.PrincipalHuman, UserID: ids.NewV7()}
	actor, err := storedActor{}.rebuild(context.Background(), stored)
	if err == nil || actor.Type != "" {
		t.Fatalf("rebuilding with no workspace bound answered %+v, %v; want no principal and an error", actor, err)
	}
}

func TestAnImportRunActsOnlyWithItsActorAndItsCorrelation(t *testing.T) {
	t.Parallel()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	stored := migration.StoredActor{Type: principal.PrincipalHuman, UserID: ids.NewV7()}
	correlation := ids.NewV7()

	noActor := migration.Run{ID: migration.RunID(ids.NewV7()), JobAttempt: 1, AttemptCorrelationID: &correlation}
	if _, err := (storedActor{}).actAsRun(ctx, noActor); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a run naming no actor answered %v, want a permission refusal", err)
	}
	noTrace := migration.Run{ID: migration.RunID(ids.NewV7()), JobAttempt: 2, Actor: &stored}
	bound, err := storedActor{}.actAsRun(ctx, noTrace)
	if err == nil || bound != nil {
		t.Errorf("a run attempt with no correlation answered a context %v, %v; want none and an error", bound, err)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a missing correlation answered a permission refusal %v; it is the run's defect, not the actor's", err)
	}
}

func TestAnAgentWithNoPassportIsBoundOnlyByItsHuman(t *testing.T) {
	t.Parallel()
	live, ok, err := storedActor{}.passportStillLive(context.Background(), ids.UUID{}, ids.NewV7())
	if err != nil || !ok {
		t.Fatalf("an agent with no passport answered live %v, %v; want live", ok, err)
	}
	asked := []principal.Scope{principal.ScopeWrite, principal.ScopeSend}
	if got := live.within(asked); !got.Has(principal.ScopeWrite) || !got.Has(principal.ScopeSend) || len(got) != len(asked) {
		t.Errorf("an agent with no passport keeps %v, want every verb asked for: %v", got, asked)
	}
}

func TestALivePassportKeepsOnlyTheVerbsItHolds(t *testing.T) {
	t.Parallel()
	live := livePassport{scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite)}
	got := live.within([]principal.Scope{principal.ScopeWrite, principal.ScopeSend})
	if !got.Has(principal.ScopeWrite) || got.Has(principal.ScopeSend) || len(got) != 1 {
		t.Errorf("a read and write passport asked for write and send keeps %v, want write alone", got)
	}
	if none := live.within(nil); none != nil {
		t.Errorf("asking for no verb keeps %v, want none", none)
	}
}
