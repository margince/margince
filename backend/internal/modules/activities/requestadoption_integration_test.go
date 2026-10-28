// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAcceptingHistoricalOwnRequestRecordsDurableAdoption(t *testing.T) {
	for _, retired := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "retired"}[retired], func(t *testing.T) {
			e := setupLoad(t)
			source := seedEmailRequest(t, e, "Send the old report", "commitment", OwedVerdictAsksUs)
			store := storeKnowing(e).WithClock(func() time.Time { return pastTheHorizon })
			if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if retired {
				if err := store.CaptureEmailRequests(asClassifier(e), pastTheHorizon); err != nil {
					t.Fatal(err)
				}
			}
			task, created, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source})
			if err != nil {
				t.Fatal(err)
			}
			if created || task.ArchivedAt != nil {
				t.Fatal("acceptance did not restore the existing reminder")
			}
			rows, total, err := store.ListActivities(e.asSeat(e.rep), ListActivitiesInput{Kind: new("task"), AssigneeID: new(ids.From[ids.UserKind](e.rep)), TaskAsOf: &pastTheHorizon})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || (total.Total != nil && *total.Total != 1) || rows[0].Id != task.Id {
				t.Fatalf("adopted old task missing from current list: %d/%v", len(rows), total.Total)
			}
			var adoption, events int
			if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND evidence->>'accepted_request'='true'`, task.Id).Scan(&adoption); err != nil {
				t.Fatal(err)
			}
			if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM event_outbox e JOIN audit_log a ON a.id::text=e.envelope->'trace'->>'audit_log_id' WHERE a.entity_id=$1 AND a.evidence->>'accepted_request'='true'`, task.Id).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if adoption != 1 || events != 1 {
				t.Fatalf("adoption audit/outbox=%d/%d", adoption, events)
			}
			if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequestReplayCannotAdoptWithdrawnOrReassignedWork(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	source := seedEmailRequest(t, e, "Send the report", "commitment", OwedVerdictAsksUs)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var id ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE source_system='email_request' AND source_activity_id=$1`, source).Scan(&id); err != nil {
		t.Fatal(err)
	}
	taskID := ids.From[ids.ActivityKind](id)
	stale, err := store.GetActivity(e.asSeat(e.rep), taskID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := store.SetOwedVerdict(asClassifier(e), source, OwedVerdictInformsUs, "withdrawn", dbNow(t, e).Add(time.Hour)); err != nil || !changed {
		t.Fatalf("withdrawing source: changed=%v err=%v", changed, err)
	}
	if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("withdrawn automatic request accepted: %v", err)
	}
	if _, err := store.UpdateActivity(e.asSeat(e.rep), taskID, UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](e.other))}); err != nil {
		t.Fatal(err)
	}
	err = store.tx(e.asSeat(e.rep), func(tx pgx.Tx) error {
		_, _, err := store.restoreRequestTask(e.asSeat(e.rep), tx, stale, true, e.rep, LogActivityInput{})
		return err
	})
	if !errors.Is(err, apperrors.ErrConflict) && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("stale snapshot adopted reassigned task: %v", err)
	}
}

func TestTwoRecipientsCannotBothAdoptTheSameAutomaticRequest(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	source, _, err := store.LogActivity(asClassifier(e), LogActivityInput{Kind: "email", Source: "test", Subject: new("Please choose who replies"), Direction: new("inbound"), OccurredAt: &requestInstant, EmailTo: []string{"rep-" + e.rep.String() + "@load.test", "rep-" + e.other.String() + "@load.test"}})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	if _, err := store.SetOwedVerdict(asClassifier(e), sourceID, OwedVerdictAsksUs, "fixture", requestInstant); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LogActivity(asClassifier(e), emailRequestTaskInput(source, nil, e.other, requestInstant, false)); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, reader := range []ids.UUID{e.rep, e.other} {
		go func() {
			<-start
			_, _, err := store.LogActivity(e.asSeat(reader), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &sourceID})
			results <- err
		}()
	}
	close(start)
	accepted, conflicted := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, apperrors.ErrConflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 1 || conflicted != 1 {
		t.Fatalf("acceptance results: %d accepted, %d conflicts", accepted, conflicted)
	}
}
