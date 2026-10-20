// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedRepairArchivesOnlyProvenUntouchedAutomaticWork(t *testing.T) {
	for _, state := range []string{"pristine", "human_created", "edited", "missing_creation_proof", "pinned"} {
		t.Run(state, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:repair-fixture")
			if state == "human_created" {
				ctx = e.Admin()
			}
			at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: new("Account discussion"), Source: "manual", OccurredAt: &at})
			if err != nil {
				t.Fatal(err)
			}
			task, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
				Kind: "task", Subject: new("Prepare an account review"), DueAt: &at, Source: "manual",
				SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)),
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(task.Id))
			switch state {
			case "edited":
				if _, err := e.Activities.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{Subject: new("My revised plan")}); err != nil {
					t.Fatal(err)
				}
			case "missing_creation_proof":
				legacyID := ids.NewV7()
				// An imported legacy row has no creation audit; ordinary writers cannot produce that state.
				if _, err := integration.OwnerConn(t).Exec(e.Admin(), `INSERT INTO activity
 (id,kind,subject,occurred_at,due_at,source_system,source_id,source,captured_by,source_activity_id,audience)
 SELECT @legacy_id,kind,subject,occurred_at,due_at,source_system,@locator,source,captured_by,source_activity_id,audience
 FROM activity WHERE id=@original_id`, pgx.NamedArgs{"legacy_id": legacyID, "locator": legacyID.String(), "original_id": id}); err != nil {
					t.Fatal(err)
				}
				if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
					return e.Activities.ArchiveActivityTx(e.Admin(), tx, id, nil)
				}); err != nil {
					t.Fatal(err)
				}
				id = ids.From[ids.ActivityKind](legacyID)
			case "pinned":
				if err := e.Activities.PinWorklistRow(e.Admin(), activities.WorklistRowRef{Source: "task", RowID: id.String()}); err != nil {
					t.Fatal(err)
				}
			}
			var candidates []activities.HomeRepairTask
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				var err error
				candidates, err = activities.HomeRepairTasksTx(e.Admin(), tx, ids.Nil, 10)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(candidates) != 1 || candidates[0].ID != id || candidates[0].Pristine != (state == "pristine") {
				t.Fatalf("repair candidates: %+v", candidates)
			}
			version := candidates[0].Version
			err = e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				return e.Activities.ArchiveActivityTx(e.Admin(), tx, id, &version, activities.OnlyPristineAutomaticTask(), activities.WithArchiveReason("historical_automatic_work"))
			})
			if state != "pristine" {
				if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("protected work error=%v, want conflict", err)
				}
				if e.WsCount(t, "SELECT count(*) FROM activity WHERE id=$1 AND archived_at IS NULL AND NOT is_done", id) != 1 {
					t.Fatal("repair changed protected work")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if e.WsCount(t, "SELECT count(*) FROM activity WHERE id=$1 AND archived_at IS NOT NULL AND NOT is_done", id) != 1 {
				t.Fatal("repair must archive without claiming completion")
			}
			if e.WsCount(t, "SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND action='archive' AND evidence->>'reason'='historical_automatic_work'", id) != 1 {
				t.Fatal("archive omitted its reviewed reason")
			}
		})
	}
}

func TestRepairPreviewUsesAdmissionWithoutChangingWork(t *testing.T) {
	e := integration.Setup(t)
	asOf := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, daysOld := range []int{1, 90} {
		occurred := asOf.AddDate(0, 0, -daysOld)
		source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: new("Account discussion"), Source: "manual", OccurredAt: &occurred})
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:repair-fixture"), activities.LogActivityInput{
			Kind: "task", Subject: new("Prepare the account review"), Source: "manual",
			SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	beforeAudit := e.WsCount(t, "SELECT count(*) FROM audit_log")
	var decisions []homeRepairTaskDecision
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var err error
		decisions, err = homeRepairTaskDecisionsTx(e.Admin(), tx, ids.Nil, 10, asOf)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 2 {
		t.Fatalf("preview=%+v", decisions)
	}
	if decisions[0].Action != repairLeaveCurrent || decisions[1].Action != repairArchiveReminder || decisions[1].Reason != string(activities.CommitmentHistorical) {
		t.Fatalf("admission dispositions=%+v", decisions)
	}
	if e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NULL AND NOT is_done") != 2 || e.WsCount(t, "SELECT count(*) FROM audit_log") != beforeAudit {
		t.Fatal("preview changed work or its audit trail")
	}
}
