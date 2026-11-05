// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAcceptingARequestRecoversAnUntouchedWronglyAssignedReminder(t *testing.T) {
	for _, state := range []string{"untouched", "human held", "pinned", "system retired", "owner archived", "system updated", "human withdrawn", "other reader pin", "expired pin", "second recipient"} {
		t.Run(state, func(t *testing.T) {
			e := setupLoad(t)
			store := storeKnowing(e)
			secondRecipient := ids.NewV7()
			address := "second-" + secondRecipient.String() + "@load.test"
			if _, err := e.owner.Exec(context.Background(), `INSERT INTO app_user (id,email,display_name) VALUES ($1,$2,'Reader')`, secondRecipient, address); err != nil {
				t.Fatal(err)
			}
			recipients := []string{"rep-" + e.rep.String() + "@load.test", address}
			if state == "second recipient" {
				recipients = append(recipients, "rep-"+e.other.String()+"@load.test")
			}
			sourceRow, _, err := store.LogActivity(asClassifier(e), LogActivityInput{Kind: "email", Subject: new("Please send the specification"), Direction: new("inbound"), Source: "test", OccurredAt: &requestInstant, EmailTo: recipients})
			if err != nil {
				t.Fatal(err)
			}
			source := ids.UUID(sourceRow.Id)
			if _, err := store.SetOwedVerdict(asClassifier(e), source, OwedVerdictAsksUs, "fixture", requestInstant); err != nil {
				t.Fatal(err)
			}
			sourceRow, err = store.GetActivity(e.asSeat(e.rep), ids.From[ids.ActivityKind](source), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			original, _, err := store.LogActivity(asClassifier(e), emailRequestTaskInput(sourceRow, nil, e.other, requestInstant, false))
			if err != nil {
				t.Fatal(err)
			}
			originalID := ids.From[ids.ActivityKind](ids.UUID(original.Id))
			prepareRecoveryOriginal(t, e, store, originalID, state)
			accepted, created, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source})
			if state == "human held" || state == "pinned" {
				if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("human-held request = %v, want conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !created || accepted.AssigneeId == nil || ids.UUID(*accepted.AssigneeId) != e.rep || accepted.SourceActivityId == nil || ids.UUID(*accepted.SourceActivityId) != source {
				t.Fatal("acceptance did not create the reader's source-linked reminder")
			}
			again, created, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source})
			if err != nil || created || again.Id != accepted.Id {
				t.Fatalf("acceptance replay changed identity: created=%v err=%v", created, err)
			}
			if _, _, err := store.LogActivity(e.asSeat(secondRecipient), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source}); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("second accepting reader = %v, want conflict", err)
			}
			retained, err := store.GetActivity(e.asSeat(e.other), originalID, storekit.IncludeArchived)
			if err != nil {
				t.Fatal(err)
			}
			if retained.AssigneeId == nil || ids.UUID(*retained.AssigneeId) != e.other || (retained.IsDone != nil && *retained.IsDone) {
				t.Fatal("acceptance changed the private original")
			}
			hidden, err := store.GetActivity(e.asSeat(e.rep), originalID, storekit.IncludeArchived)
			if err != nil {
				t.Fatal(err)
			}
			if hidden.Subject != nil || hidden.Body != nil {
				t.Fatal("acceptance exposed the private original")
			}
			if _, err := store.ArchiveActivity(e.asSeat(e.rep), ids.From[ids.ActivityKind](ids.UUID(accepted.Id)), nil); err != nil {
				t.Fatal(err)
			}
			if retained.ArchivedAt != nil {
				return
			}
			if _, err := store.UpdateActivity(e.asSeat(e.other), originalID, UpdateActivityInput{Subject: new("I have now taken this task")}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.LogActivity(e.asSeat(e.rep), LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &source}); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("archived replay bypassed colleague ownership: %v", err)
			}
		})
	}
}

func prepareRecoveryOriginal(t *testing.T, e *loadEnv, store *Store, id ids.ActivityID, state string) {
	t.Helper()
	if state == "human held" || state == "human withdrawn" {
		if _, err := store.UpdateActivity(e.asSeat(e.other), id, UpdateActivityInput{Subject: new("I will handle this request")}); err != nil {
			t.Fatal(err)
		}
	}
	if state == "system updated" {
		if _, err := store.UpdateActivity(asClassifier(e), id, UpdateActivityInput{Subject: new("The source still asks for the report")}); err != nil {
			t.Fatal(err)
		}
	}
	if state == "system retired" || state == "owner archived" || state == "human withdrawn" {
		actor := e.asSeat(e.other)
		if state == "system retired" {
			actor = asClassifier(e)
		}
		if _, err := store.ArchiveActivity(actor, id, nil); err != nil {
			t.Fatal(err)
		}
	}
	if state == "pinned" || state == "other reader pin" || state == "expired pin" {
		actor := e.asSeat(e.other)
		if state == "other reader pin" {
			actor = e.asSeat(e.rep)
		}
		if err := store.PinWorklistRow(actor, WorklistRowRef{Source: "task", RowID: id.String()}); err != nil {
			t.Fatal(err)
		}
	}
	if state == "expired pin" {
		for range 50 {
			task, _, err := store.LogActivity(e.asSeat(e.other), LogActivityInput{Kind: "task", Source: "manual", Subject: new("Keep this work"), AssigneeID: new(ids.From[ids.UserKind](e.other))})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PinWorklistRow(e.asSeat(e.other), WorklistRowRef{Source: "task", RowID: task.Id.String()}); err != nil {
				t.Fatal(err)
			}
		}
	}
}
