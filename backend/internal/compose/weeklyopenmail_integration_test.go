// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A week read on its Wednesday still owes its Friday. The Worklist files that
// task as upcoming, and the weekly mail must read it from there too.
func TestTheWeeklyMailNamesATaskDueLaterInTheReportedWeek(t *testing.T) {
	e := setupWeeklyMail(t)
	wednesday := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	friday := wednesday.AddDate(0, 0, 2)
	rep := ids.From[ids.UserKind](e.Rep1)
	subject := "Send the Friday pricing sheet"
	if _, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: &rep, DueAt: &friday,
	}); err != nil {
		t.Fatal(err)
	}
	review := weekly.Review{LocalWeekStart: time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)}

	body, err := e.worker.openTasksMail(e.repCtx, review, wednesday, mailcopy.For("en"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), subject) {
		t.Fatalf("the weekly mail left out the task due Friday:\n%s", body.String())
	}
}
