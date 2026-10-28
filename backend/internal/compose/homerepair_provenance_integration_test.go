// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type legacyRepairFixture struct {
	e      *integration.Env
	now    time.Time
	task   ids.ActivityID
	source ids.UUID
	lead   ids.LeadID
}

func seedLegacyRepair(t *testing.T, old bool, locator ...string) legacyRepairFixture {
	t.Helper()
	e := integration.Setup(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	return seedLegacyRepairOn(t, e, now, old, locator...)
}

func seedLegacyRepairOn(t *testing.T, e *integration.Env, now time.Time, old bool, locator ...string) legacyRepairFixture {
	t.Helper()
	owner := ids.From[ids.UserKind](e.Rep2)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Demo lead"), Source: "manual", OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	at := now.Add(-time.Hour)
	if old {
		at = now.AddDate(0, 0, -90)
	}
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: new("Demo enquiry"), Direction: new("inbound"), Source: "manual", OccurredAt: &at,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer := newLeadWorkTasks(InstallationDB(e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	in := writer.repliedTaskInput(leadID, ids.UUID(source.Id), "Review the enquiry", &e.Rep2)
	in.SourceActivityID, in.DueAt = nil, &now
	if len(locator) > 0 {
		in.SourceID = &locator[0]
	}
	task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), leadRepliedActor), in)
	if err != nil {
		t.Fatal(err)
	}
	return legacyRepairFixture{e: e, now: now, task: ids.From[ids.ActivityKind](ids.UUID(task.Id)), source: ids.UUID(source.Id), lead: leadID}
}

func (f legacyRepairFixture) preview(t *testing.T) (datatask.ReviewRequest, homeRepairTaskRef) {
	t.Helper()
	var review datatask.ReviewRequest
	var selected homeRepairTaskRef
	err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		var err error
		review, err = previewHomeRepairAfterTx(f.e.Admin(), tx, ids.Nil, 100, f.now)
		if err != nil {
			return err
		}
		var scope homeRepairScope
		if err := json.Unmarshal(review.Scope, &scope); err != nil {
			return err
		}
		for _, item := range scope.Tasks {
			if item.ID == f.task.UUID {
				selected = item
				return nil
			}
		}
		return errors.New("legacy task missing from real preview")
	})
	if err != nil {
		t.Fatal(err)
	}
	return review, selected
}

func (f legacyRepairFixture) applyReviewed(t *testing.T, review datatask.ReviewRequest, item homeRepairTaskRef) bool {
	t.Helper()
	changed := false
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		var err error
		changed, err = applyHomeRepairTask(f.e.Admin(), tx, f.e.Activities, item, datatask.Activation{ReviewRequest: review}, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return changed
}

func TestReviewedLegacyCorrectionDoesNotAdoptWorkBeforeLaterRetirement(t *testing.T) {
	f := seedLegacyRepair(t, false)
	review, item := f.preview(t)
	if !f.applyReviewed(t, review, item) {
		t.Fatal("initial correction did not apply")
	}
	_, after := f.preview(t)
	if after.Action != repairLeaveCurrent {
		t.Fatalf("correction became adoption or repeat work: %+v", after)
	}
	if _, err := f.e.Activities.ArchiveActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.source), nil); err != nil {
		t.Fatal(err)
	}
	laterReview, after := f.preview(t)
	if after.Action != repairArchiveReminder {
		t.Fatalf("normalization hid later obsolescence: %+v", after)
	}
	if !f.applyReviewed(t, laterReview, after) {
		t.Fatal("normalization prevented the later reviewed archive")
	}
	if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND archived_at IS NOT NULL AND NOT is_done", pgx.NamedArgs{"id": f.task}) != 1 {
		t.Fatal("later archive lost the correction's pristine proof")
	}
}

func TestReviewedLegacyRepairUsesCurrentSourceAdmission(t *testing.T) {
	for _, old := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "historic"}[old], func(t *testing.T) {
			f := seedLegacyRepair(t, old)
			review, item := f.preview(t)
			want := repairNormalizeTask
			if old {
				want = repairArchiveReminder
			}
			if item.Action != want {
				t.Fatalf("preview action=%s, want %s: %+v", item.Action, want, item)
			}
			if !f.applyReviewed(t, review, item) {
				t.Fatal("reviewed correction did not apply")
			}
			if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND is_done", pgx.NamedArgs{"id": f.task}) != 0 {
				t.Fatal("retirement claimed completion")
			}
			if old {
				if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND archived_at IS NOT NULL", pgx.NamedArgs{"id": f.task}) != 1 {
					t.Fatal("obsolete pristine advice remained live")
				}
			} else {
				if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND source_activity_id=@source AND due_at IS NULL AND archived_at IS NULL", pgx.NamedArgs{"id": f.task, "source": f.source}) != 1 {
					t.Fatal("proven source/date correction was lost")
				}
			}
			writer := newLeadWorkTasks(InstallationDB(f.e.Pool), slog.Default())
			writer.now = func() time.Time { return f.now }
			if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error { return writer.FileLeadReplyTx(f.e.Admin(), tx, f.lead, f.source) }); err != nil {
				t.Fatal(err)
			}
			if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE source_id=@source", pgx.NamedArgs{"source": f.source.String()}) != 1 {
				t.Fatal("repair replay duplicated historical work")
			}
		})
	}
}

func TestReviewedLegacyRepairPreservesHumanAdoptionAndRollsBack(t *testing.T) {
	for _, change := range []string{"subject", "due", "pin", "reminder", "rollback"} {
		t.Run(change, func(t *testing.T) {
			f := seedLegacyRepair(t, false)
			review, item := f.preview(t)
			if item.Action != repairNormalizeTask {
				t.Fatalf("fixture has no correction: %+v", item)
			}
			switch change {
			case "subject":
				if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), f.task, activities.UpdateActivityInput{Subject: new("My deliberate plan")}); err != nil {
					t.Fatal(err)
				}
			case "due":
				at := f.now.Add(48 * time.Hour)
				if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), f.task, activities.UpdateActivityInput{DueAt: &at}); err != nil {
					t.Fatal(err)
				}
			case "pin":
				owner := f.e.As(f.e.Rep2, []ids.UUID{f.e.Team2}, integration.AdminPerms)
				if err := f.e.Activities.PinWorklistRow(owner, activities.WorklistRowRef{Source: "task", RowID: f.task.String()}); err != nil {
					t.Fatal(err)
				}
			case "reminder":
				at := f.now.Add(time.Hour)
				if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), f.task, activities.UpdateActivityInput{RemindAt: &at}); err != nil {
					t.Fatal(err)
				}
			}
			rolledBack := errors.New("rollback demo batch")
			err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
				changed, err := applyHomeRepairTask(f.e.Admin(), tx, f.e.Activities, item, datatask.Activation{ReviewRequest: review}, ids.NewV7())
				if err != nil {
					return err
				}
				if changed != (change == "rollback") {
					t.Fatalf("changed=%v for %s", changed, change)
				}
				if change == "rollback" {
					return rolledBack
				}
				return nil
			})
			if err != nil && !errors.Is(err, rolledBack) {
				t.Fatal(err)
			}
			if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND source_activity_id IS NULL AND due_at IS NOT NULL AND archived_at IS NULL AND NOT is_done", pgx.NamedArgs{"id": f.task}) != 1 {
				t.Fatal("protected or rolled-back task changed")
			}
		})
	}
}

func TestReviewedLegacyRepairCannotAdmitAnUnresolvableSource(t *testing.T) {
	f := seedLegacyRepair(t, true, "unresolved-legacy-reference")
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		review, err := previewHomeRepairAfterTx(f.e.Admin(), tx, ids.Nil, 100, f.now)
		if err != nil {
			return err
		}
		var scope homeRepairScope
		if err := json.Unmarshal(review.Scope, &scope); err != nil {
			return err
		}
		for _, item := range scope.Tasks {
			if item.ID == f.task.UUID {
				t.Fatal("unresolvable task bypassed source access")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND source_activity_id IS NULL AND due_at IS NOT NULL AND archived_at IS NULL AND NOT is_done", pgx.NamedArgs{"id": f.task}) != 1 {
		t.Fatal("unresolved task changed")
	}
}
