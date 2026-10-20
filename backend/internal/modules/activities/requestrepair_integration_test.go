// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReviewedRequestRepairUsesTheRetirementHorizon(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Send the signed proposal", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ctx := e.asSeat(e.rep)
	var task HomeRepairTask
	if err := store.tx(ctx, func(tx pgx.Tx) error {
		tasks, err := HomeRepairTasksTx(ctx, tx, ids.Nil, 100)
		if err != nil {
			return err
		}
		for _, candidate := range tasks {
			if candidate.SourceID != nil && *candidate.SourceID == source {
				task = candidate
			}
		}
		if task.ID.UUID == ids.Nil || !task.Pristine {
			t.Fatalf("automatic task not available for repair: %+v", task)
		}
		for _, check := range []struct {
			at   time.Time
			past bool
		}{
			{requestInstant.Add(time.Hour), false}, {pastTheHorizon, true},
		} {
			past, err := RequestReminderPastHorizonTx(ctx, tx, task, check.at)
			if err != nil {
				return err
			}
			if past != check.past {
				t.Fatalf("past horizon at %s: got %t, want %t", check.at, past, check.past)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), pastTheHorizon); err != nil {
		t.Fatal(err)
	}
	var archived, done bool
	if err := e.owner.QueryRow(ctx, `SELECT archived_at IS NOT NULL,is_done FROM activity WHERE id=@id`, pgx.NamedArgs{"id": task.ID}).Scan(&archived, &done); err != nil {
		t.Fatal(err)
	}
	if !archived || done {
		t.Fatalf("retirement differs from reviewed repair: archived=%t done=%t", archived, done)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), pastTheHorizon.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM activity WHERE source_activity_id=@id`, pgx.NamedArgs{"id": source}).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retired reminder recreated: count=%d", count)
	}
}
