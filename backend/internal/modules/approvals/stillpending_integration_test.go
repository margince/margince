// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPendingMaintenanceHonorsExpiryAndHeldMessages(t *testing.T) {
	e := setupStaging(t)
	at := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	e.svc.now = func() time.Time { return at }
	ordinary, err := e.svc.Stage(e.forRep(), StageInput{
		Kind: "fx_rate_proposal", ProposedChange: []byte(`{}`), DiffHash: "rate",
	})
	if err != nil {
		t.Fatal(err)
	}
	heldContext := principal.WithActor(e.sweeping(), principal.Principal{
		Type: principal.PrincipalSystem, ID: ExpiryActor, OnBehalfOf: e.rep,
	})
	held, err := e.svc.Stage(heldContext, StageInput{
		Kind: KindScheduledSendHeld, ProposedChange: []byte(`{}`), DiffHash: "held",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Staging stamps the database clock; pin only the expiry column for this clock test.
	if _, err := e.owner.Exec(t.Context(), `UPDATE approval SET expires_at = @expiry WHERE id = ANY(@ids)`,
		pgx.StrictNamedArgs{"expiry": at, "ids": []ids.ApprovalID{ordinary, held}}); err != nil {
		t.Fatal(err)
	}
	at = at.Add(stagingTTL + time.Minute)
	missing := ids.From[ids.ApprovalKind](ids.NewV7())
	err = e.svc.db.Tx(e.sweeping(), func(tx pgx.Tx) error {
		pending, err := e.svc.StillPending(e.sweeping(), tx, []ids.ApprovalID{ordinary, held, missing})
		if err != nil {
			return err
		}
		if pending[ordinary] || !pending[held] || pending[missing] {
			t.Errorf("pending states: ordinary=%v held=%v missing=%v", pending[ordinary], pending[held], pending[missing])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.StillPending(e.as(), nil, []ids.ApprovalID{held}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("human maintenance read returned %v", err)
	}
	expired, err := e.svc.ExpireDue(e.sweeping())
	if err != nil || len(expired) != 1 || expired[0].ID != ordinary {
		t.Fatalf("expiry pass=%v err=%v", expired, err)
	}
}
