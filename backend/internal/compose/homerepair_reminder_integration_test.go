// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A reviewed correction never leaves a reminder armed on a task whose deadline
// it cleared. A task somebody set a reminder on is no longer the product's
// untouched task, so the correction refuses it and both the deadline and the
// reminder stay. (The write itself also clears remind_at with due_at, so a
// later widening of what counts as untouched cannot orphan one.)
func TestARepairNeverLeavesAReminderOnAClearedDeadline(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	owner := ids.From[ids.UserKind](e.Rep2)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Demo lead"), Source: "manual", OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	at := now.Add(-time.Hour)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: new("Demo enquiry"), Direction: new("inbound"), Source: "manual", OccurredAt: &at,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer := newLeadWorkTasks(InstallationDB(e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	in := writer.repliedTaskInput(ids.From[ids.LeadKind](ids.UUID(lead.Id)), ids.UUID(source.Id), "Review the enquiry", &e.Rep2)
	in.SourceActivityID, in.DueAt, in.RemindAt = nil, &now, &now
	created, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), leadRepliedActor), in)
	if err != nil {
		t.Fatal(err)
	}
	operator := repairOperator(t, e)
	err = e.DB().Tx(operator, func(tx pgx.Tx) error {
		task, err := activities.HomeRepairTaskTx(operator, tx, ids.UUID(created.Id))
		if err != nil {
			return err
		}
		return e.Activities.RepairAutomaticTaskTx(operator, tx, task, now, "test-rule")
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a repair of a task carrying a reminder answered %v, want a conflict", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND due_at IS NOT NULL AND remind_at IS NOT NULL`, created.Id); n != 1 {
		t.Fatal("a refused repair changed the task's deadline or reminder")
	}
}
