// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Moving a held message back to scheduled sends it at the new moment, so the
// reschedule route asks the same question the held-message card does.
func TestRescheduleReleasesAHeldMessageOnlyForWhoMaySendIt(t *testing.T) {
	t.Parallel()
	e := setupSend(t)
	id := scheduleBerlinSend(t, e)
	store := draftStore(e)
	if err := store.tx(e.asSendWorker(), func(tx pgx.Tx) error {
		return store.holdInTx(e.asSendWorker(), tx, id, HeldRecipientReplied)
	}); err != nil {
		t.Fatalf("holding the message: %v", err)
	}
	human := e.as(principal.RowScopeAll)
	held, err := store.GetScheduledSend(human, id)
	if err != nil || held.Status != ScheduledStatusHeld {
		t.Fatalf("held row = %+v, %v", held, err)
	}

	rep, ok := principal.Actor(human)
	if !ok {
		t.Fatal("the rep context carries no actor")
	}
	writeOnly := rep
	writeOnly.Type, writeOnly.ID, writeOnly.OnBehalfOf = principal.PrincipalAgent, "agent:t", e.rep
	writeOnly.Scopes = principal.NewScopeSet(principal.ScopeWrite)
	later := SendSchedule{At: draftClock.Add(48 * time.Hour), TZ: "Europe/Berlin"}

	_, err = store.RescheduleScheduledSend(principal.WithActor(human, writeOnly), id, later, held.Version, &recordingTimer{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("write-only agent reschedule = %v, want ErrPermissionDenied", err)
	}
	moved, err := store.RescheduleScheduledSend(human, id, later, held.Version, &recordingTimer{})
	if err != nil || moved.Status != ScheduledStatusScheduled {
		t.Fatalf("the rep's reschedule = %+v, %v; want it scheduled again", moved, err)
	}
}
