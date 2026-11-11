// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package credentialrepair

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// noTx is a nil transaction: any statement a guard lets through panics on it.
var noTx pgx.Tx

// Every step of the repair refuses a principal that is not the system, and
// refuses it before a statement reaches the database.
//
// The steps rewrite stored mail text, audit images and provider payloads, none
// of which any seat reaches through an ordinary write path. noTx panics
// on any statement, so a guard that ran too late fails here.
func TestEveryStepOfTheCredentialRepairRefusesASeat(t *testing.T) {
	seat := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "user:a-seat"})
	for _, step := range DataTask(nil).Steps {
		t.Run(step.Name, func(t *testing.T) {
			if _, err := step.Batch(seat, noTx, datatask.BatchIn{Limit: 10}); err == nil {
				t.Error("a human seat ran this step")
			}
		})
	}
}

// A cursor the step cannot read stops it, rather than silently restarting the
// page from the beginning and reporting the work as done.
func TestACredentialRepairStepRefusesACursorItCannotRead(t *testing.T) {
	system := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})
	for _, step := range DataTask(nil).Steps {
		t.Run(step.Name, func(t *testing.T) {
			_, err := step.Batch(system, noTx, datatask.BatchIn{
				Limit: 10, Cursor: []byte("not a uuid"),
			})
			if err == nil {
				t.Error("an unreadable cursor was accepted")
			}
		})
	}
}

// The task names every step the repair owes, in order. The row comes first,
// because the steps after it read what it left.
func TestTheCredentialRepairReachesAllThreeStoresInOrder(t *testing.T) {
	want := []string{"mask_mail_text", "mask_audit_images", "purge_masked_mail_originals"}
	steps := DataTask(nil).Steps
	if len(steps) != len(want) {
		t.Fatalf("the repair has %d steps, want %d", len(steps), len(want))
	}
	for i, name := range want {
		if steps[i].Name != name {
			t.Errorf("step %d is %q, want %q", i, steps[i].Name, name)
		}
	}
}

// Every estimate refuses a seat too. An estimate reads the same rows its batch
// writes, so a count reachable by a seat says how many mails hold a credential.
func TestEveryEstimateOfTheCredentialRepairRefusesASeat(t *testing.T) {
	seat := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "user:a-seat"})
	for _, step := range DataTask(nil).Steps {
		t.Run(step.Name, func(t *testing.T) {
			if _, _, err := step.Estimate(seat, noTx, 1000); err == nil {
				t.Error("a human seat counted the rows this step would reach")
			}
		})
	}
}
