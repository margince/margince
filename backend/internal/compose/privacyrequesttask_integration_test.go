// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	pubjurisdiction "github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// requestTaskPack follows the recipient and sets the Decree 356/2025 objection
// deadline, as the Vietnamese pack does.
const requestTaskPack jurisdiction.Code = "xr"

var registerRequestTaskPack sync.Once

// TestAnObjectionUnderARecipientBoundPackBecomesADatedTask: the real task
// writer files the objection's task on the contact, for the privacy operator,
// due 15 days later.
func TestAnObjectionUnderARecipientBoundPackBecomesADatedTask(t *testing.T) {
	registerRequestTaskPack.Do(func() {
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: requestTaskPack, Version: 1, DataProtectionFollowsRecipient: true,
		})
		subjectrightsrules.Register(subjectrightsrules.Rules{
			Jurisdiction: requestTaskPack, Version: 1,
			Instruments: []messaging.Instrument{{Name: "Test Decree 2/2026"}},
			Deadlines: map[subjectrightsrules.RequestKind]subjectrightsrules.Deadline{
				"restriction": {Period: pubjurisdiction.Period{Days: 15}},
			},
			ReplyWithinWorkingDays: 2,
		})
	})
	l := setupConfirmLane(t)
	l.e.WsExec(t, `INSERT INTO role_assignment (user_id, role_id) SELECT $1, id FROM role WHERE key = 'admin'`, l.e.AdminUser)
	l.store = l.store.WithRequestTasks(privacyRequestTasks{tasks: activities.NewStore(InstallationDB(l.e.Pool))})
	l.livesIn(t, requestTaskPack)

	before := time.Now()
	l.object(t)

	var due time.Time
	var assignee string
	var linked bool
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT a.due_at, coalesce(a.assignee_id::text, ''),
		       EXISTS (SELECT 1 FROM activity_link k WHERE k.activity_id = a.id AND k.contact_id = $2)
		  FROM activity a
		 WHERE a.kind = 'task' AND a.source_system = $1`,
		provenance.PrivacyRequestTaskSource, l.contact).Scan(&due, &assignee, &linked); err != nil {
		t.Fatalf("reading the objection's task: %v", err)
	}
	want := 15 * 24 * time.Hour
	if got := due.Sub(before); got < want-time.Minute || got > want+time.Minute {
		t.Errorf("the task is due %s after the objection, want 15 days", got)
	}
	if assignee != l.e.AdminUser.String() {
		t.Errorf("the task is assigned to %q, want the privacy operator %s", assignee, l.e.AdminUser)
	}
	if !linked {
		t.Error("the task is not filed on the contact who objected")
	}
}
