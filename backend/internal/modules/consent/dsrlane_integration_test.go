// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The worklist's thin read of the case queue: the open cases in deadline
// order, behind exactly the gate the queue itself stands behind.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOpenDSRsComeBackSoonestDeadlineFirst(t *testing.T) {
	e := setupDSR(t)
	later, err := e.store.CreateDSR(e.ctx, CreateDSRInput{
		Kind: "access", SubjectRef: "later@dsr.test",
		DueAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	soon, err := e.store.CreateDSR(e.ctx, CreateDSRInput{
		Kind: "erasure", SubjectRef: "soon@dsr.test",
		DueAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	answered, err := e.store.CreateDSR(e.ctx, CreateDSRInput{
		Kind: "access", SubjectRef: "done@dsr.test",
		DueAt: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolution := "handled"
	if _, err := e.store.UpdateDSR(e.ctx, answered.ID, UpdateDSRInput{
		Status: strptr("fulfilled"), Resolution: &resolution,
	}); err != nil {
		t.Fatalf("closing the answered request: %v", err)
	}

	owed, err := e.store.OpenDSRsDueSoonest(e.ctx, 8)
	if err != nil {
		t.Fatalf("reading the open cases: %v", err)
	}
	if len(owed) != 2 {
		t.Fatalf("open cases = %d, want the two unresolved ones", len(owed))
	}
	if owed[0].ID != soon.ID || owed[1].ID != later.ID {
		t.Errorf("order = %v then %v, want the soonest deadline first", owed[0].ID, owed[1].ID)
	}
	if owed[0].Kind != "erasure" || owed[0].DueAt.IsZero() {
		t.Errorf("the case lost its kind or deadline: %+v", owed[0])
	}
}

// The queue names who exercised a legal right; the lane must reach exactly
// as far as the queue's own admin gate and no further.
func TestTheLaneReadIsRefusedForANonAdmin(t *testing.T) {
	e := setupDSR(t)
	rep := principal.WithWorkspaceID(context.Background(), e.ws)
	rep = principal.WithCorrelationID(rep, ids.NewV7())
	rep = principal.WithActor(rep, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"person": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := e.store.OpenDSRsDueSoonest(rep, 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a non-admin's read answered %v, want the refusal the queue gives", err)
	}
}
