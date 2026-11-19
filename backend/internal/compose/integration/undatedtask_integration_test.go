// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A task a person writes with no due date waits under Upcoming and backlog.
// It sits in a No due date group after the dated work, and never on Today.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// undatedBacklog is one more undated task than the upcoming lane holds. The
// rest arrive in the lazy batches, and the total is the store's count.
const undatedBacklog = 7

// taskSpec is one task to write. An empty sourceSystem writes it as a person
// does; a named one writes it as that automatic source does.
type taskSpec struct {
	subject       string
	due, remindAt *time.Time
	sourceSystem  string
}

func writeTask(t *testing.T, e *Env, contact ids.UUID, spec taskSpec) string {
	t.Helper()
	in := activities.LogActivityInput{
		Kind: "task", Subject: &spec.subject, Source: provenance.RecordSourceManual, DueAt: spec.due, RemindAt: spec.remindAt,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}
	if spec.sourceSystem != "" {
		in.SourceSystem, in.SourceID = &spec.sourceSystem, new(ids.NewV7().String())
	}
	task, _, err := e.Activities.LogActivity(e.Admin(), in)
	if err != nil {
		t.Fatalf("writing %q: %v", spec.subject, err)
	}
	return ids.UUID(task.Id).String()
}

func worklistAt(t *testing.T, e *Env, asOf time.Time, scope, filter string) crmcontracts.Worklist {
	t.Helper()
	page, err := compose.NewAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return asOf }).
		Worklist(e.Admin(), scope, filter, ids.UUID{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestAnUndatedTaskWaitsUnderUpcomingAndNeverOnToday(t *testing.T) {
	e := Setup(t)
	asOf := clocktest.Now(t)
	contact := e.SeedContact(t, "Renewal Contact", &e.AdminUser)
	dated := writeTask(t, e, contact, taskSpec{subject: "Send the proposal", due: new(asOf.AddDate(0, 0, 3))})
	undated := map[string]bool{}
	for range undatedBacklog {
		undated[writeTask(t, e, contact, taskSpec{subject: "Call back about the renewal"})] = true
	}
	owedNow := writeTask(t, e, contact, taskSpec{subject: "Follow up as suggested", sourceSystem: provenance.AcceptedRecommendationSource})
	// An automatic source's undated task is never a person's backlog.
	automatic := writeTask(t, e, contact, taskSpec{subject: "Reach out again", sourceSystem: provenance.NoActivityReminderSource})

	for _, scope := range []string{"mine", "all"} {
		today := false
		for _, row := range worklistAt(t, e, asOf, scope, "").Queue {
			if undated[row.Id] {
				t.Errorf("the %s Today queue holds the undated task %s", scope, row.Id)
			}
			today = today || row.Id == owedNow
		}
		if !today {
			t.Errorf("the %s Today queue lost the undated task an accepted suggestion owes now", scope)
		}
		page := worklistAt(t, e, asOf, scope, string(crmcontracts.WorklistFilterUpcoming))
		if got := countTotal(page, crmcontracts.WorklistCountCategoryUpcoming); got != undatedBacklog+1 {
			t.Errorf("the %s upcoming total is %d, want the dated task and %d undated ones", scope, got, undatedBacklog)
		}
		var order []string
		for _, row := range page.Queue {
			if row.Id == owedNow || row.Id == automatic {
				t.Errorf("the %s upcoming queue holds the automatic undated task %s", scope, row.Id)
			}
			if row.Id != dated && !undated[row.Id] {
				continue
			}
			order = append(order, row.Id)
			if undated[row.Id] && (row.DueGroup == nil || *row.DueGroup != crmcontracts.WorklistItemDueGroupWorklistDueGroupNone) {
				t.Errorf("the %s undated task %s carries group %v, want none", scope, row.Id, row.DueGroup)
			}
		}
		if len(order) != undatedBacklog+1 || order[0] != dated {
			t.Errorf("the %s upcoming queue lists %v, want the dated task first and then all %d undated ones",
				scope, order, undatedBacklog)
		}
	}
}

// A reminder the reader asked for makes an undated task today's work: it is on
// Today once, and the upcoming total leaves it out.
func TestAnUndatedTaskWithADueReminderIsTodaysWorkOnce(t *testing.T) {
	e := Setup(t)
	asOf := clocktest.Now(t).Add(2 * time.Hour)
	contact := e.SeedContact(t, "Renewal Contact", &e.AdminUser)
	reminded := writeTask(t, e, contact, taskSpec{subject: "Call back about the renewal", remindAt: new(asOf.Add(-time.Hour))})
	writeTask(t, e, contact, taskSpec{subject: "Send the price list"})

	seen := 0
	for _, row := range worklistAt(t, e, asOf, "mine", "").Queue {
		if row.Id == reminded {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("the Today queue lists the reminded task %d times, want once", seen)
	}
	upcoming := worklistAt(t, e, asOf, "mine", string(crmcontracts.WorklistFilterUpcoming))
	for _, row := range upcoming.Queue {
		if row.Id == reminded {
			t.Error("the upcoming queue also holds the task whose reminder is due")
		}
	}
	if got := countTotal(upcoming, crmcontracts.WorklistCountCategoryUpcoming); got != 1 {
		t.Errorf("the upcoming total is %d, want only the undated task with no reminder", got)
	}
}
