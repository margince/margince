// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package requesttask_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/requesttask"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// TestARequestBecomesADatedTaskForThePrivacyOperator: the task is filed on
// the contact, assigned to the privacy operator and due on the request's date.
func TestARequestBecomesADatedTaskForThePrivacyOperator(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO role_assignment (user_id, role_id) SELECT $1, id FROM role WHERE key = 'admin'`, e.AdminUser)
	contact := e.SeedContact(t, "Lan Nguyen", nil)
	due := time.Date(2026, time.December, 3, 9, 0, 0, 0, time.UTC)
	task := consent.RequestTask{
		SubjectType: "contact", SubjectID: contact, Right: consent.RequestTaskErasure, Key: "DSR-TEST",
		ReplyBy: time.Date(2026, time.November, 17, 9, 0, 0, 0, time.UTC), DueAt: due,
	}
	writer := requesttask.Writer{Tasks: e.Activities}
	db := database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS))
	if err := db.Tx(e.Admin(), func(tx pgx.Tx) error {
		return writer.OpenRequestTaskTx(e.Admin(), tx, task)
	}); err != nil {
		t.Fatalf("writing the task: %v", err)
	}

	var gotDue time.Time
	var assignee string
	var linked bool
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT a.due_at, coalesce(a.assignee_id::text, ''),
		       EXISTS (SELECT 1 FROM activity_link k WHERE k.activity_id = a.id AND k.contact_id = $2)
		  FROM activity a
		 WHERE a.kind = 'task' AND a.source_system = $1`,
		provenance.PrivacyRequestTaskSource, contact).Scan(&gotDue, &assignee, &linked); err != nil {
		t.Fatalf("reading the task: %v", err)
	}
	if !gotDue.Equal(due) {
		t.Errorf("the task is due %s, want %s", gotDue, due)
	}
	if assignee != e.AdminUser.String() {
		t.Errorf("the task is assigned to %q, want the privacy operator %s", assignee, e.AdminUser)
	}
	if !linked {
		t.Error("the task is not filed on the contact who asked")
	}
}
