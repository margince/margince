// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// riskBacklog is at-risk deals past both sweeps of slippingScanLimit, beside
// the waiting backlog. atRisk is each seat's deals the list must serve as at
// risk; never is what it must never serve as at risk.
type riskBacklog struct {
	waitingBacklog
	pipeline ids.PipelineID
	stage    ids.StageID
	atRisk   map[ids.UUID]map[string]bool
	never    map[string]bool
	// done is the deal Rep1 marked done; back is one that came back since.
	done, back string
}

func seedRiskBacklog(t *testing.T) riskBacklog {
	t.Helper()
	w := seedWaitingBacklog(t)
	b := riskBacklog{waitingBacklog: w, atRisk: map[ids.UUID]map[string]bool{}, never: map[string]bool{}}
	b.pipeline, b.stage, _ = integration.DealFixture(t, w.r.e)
	rep1 := w.r.e.Rep1
	// Quiet past the sweep's fifty, half undated so they close the tail.
	for i := range slippingScanLimit + 10 {
		id := b.deal(t, rep1, fmt.Sprintf("Quiet %d", i))
		b.quiet(t, id, 40+i)
		if i%2 == 0 {
			b.closes(t, id, 30+i)
		}
	}
	// Past their close, recently touched, also past the sweep's fifty.
	for i := range slippingScanLimit + 5 {
		id := b.deal(t, rep1, fmt.Sprintf("Overdue %d", i))
		b.closes(t, id, -(6 + i))
		switch i {
		case 0:
			b.taskOnDeal(t, id, -20)
			delete(b.atRisk[rep1], id.String())
			b.never[id.String()] = true
		case 1:
			b.taskOnDeal(t, id, 8)
		}
	}
	for i := range 8 {
		b.closes(t, b.deal(t, w.r.e.Rep2, fmt.Sprintf("Rep2 overdue %d", i)), -(2 + i))
	}
	// Another team's deal, and a healthy one, are never at risk here.
	b.closes(t, b.healthy(t, w.r.e.Rep3, "Rep3 overdue"), -3)
	b.healthy(t, rep1, "Healthy")
	// A customer waits on a swept deal, inside the waiting lane's scan. Another
	// waits on a deal past the sweep, older than the scan reaches.
	swept := b.healthy(t, rep1, "Waited on, swept")
	b.closes(t, swept, -200)
	w.mailOnDeal(t, swept, "Waited on, swept", w.r.now.Add(-5*time.Minute))
	past := b.healthy(t, rep1, "Waited on, past the sweep")
	b.closes(t, past, -3)
	w.mailOnDeal(t, past, "Waited on, past the sweep", w.older().AddDate(0, 0, -1))
	done := b.deal(t, rep1, "Marked done")
	b.closes(t, done, -2)
	back := b.deal(t, rep1, "Came back")
	b.closes(t, back, -4)
	b.done, b.back = done.String(), back.String()
	return b
}

// deal opens one deal for seat through the deals writer, expected at risk.
func (b *riskBacklog) deal(t *testing.T, seat ids.UUID, name string) ids.UUID {
	t.Helper()
	id := b.r.e.SeedDeal(t, name, b.pipeline, b.stage, &seat)
	if b.atRisk[seat] == nil {
		b.atRisk[seat] = map[string]bool{}
	}
	b.atRisk[seat][id.String()] = true
	return id
}

// healthy opens one deal the list never serves as at risk on Rep1's pages.
func (b *riskBacklog) healthy(t *testing.T, seat ids.UUID, name string) ids.UUID {
	t.Helper()
	id := b.r.e.SeedDeal(t, name, b.pipeline, b.stage, &seat)
	// Active at the list's own instant, which the writer's clock need not share.
	b.quiet(t, id, 0)
	b.never[id.String()] = true
	return id
}

func (b *riskBacklog) quiet(t *testing.T, deal ids.UUID, days int) {
	t.Helper()
	b.r.e.WsExec(t, `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, deal, b.r.now.AddDate(0, 0, -days))
}

func (b *riskBacklog) closes(t *testing.T, deal ids.UUID, days int) {
	t.Helper()
	b.r.e.WsExec(t, `UPDATE deal SET expected_close_date = $2 WHERE id = $1`, deal, installationDay(t, b.r.e, b.r.now).AddDate(0, 0, days).Format(time.DateOnly))
}

// taskOnDeal gives Rep1 a task due in days, filed under a deal past the sweep.
// Due within the near-task window, the task stands in for the deal, which the
// tail then leaves out. Due later, the deal stays at risk below the task.
func (b *riskBacklog) taskOnDeal(t *testing.T, deal ids.UUID, days int) {
	t.Helper()
	subject, due := fmt.Sprintf("Chase the deal in %d days", days), b.r.now.AddDate(0, 0, days)
	rep1 := ids.From[ids.UserKind](b.r.e.Rep1)
	if _, _, err := b.r.e.Activities.LogActivity(b.r.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual", AssigneeID: &rep1,
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatalf("logging the task on the deal: %v", err)
	}
}

// mailOnDeal logs one waiting request to Rep1, filed under the deal.
func (w *waitingBacklog) mailOnDeal(t *testing.T, deal ids.UUID, subject string, at time.Time) {
	t.Helper()
	inbound := "inbound"
	to := w.r.e.WsScalar(t, `SELECT lower(email) FROM app_user WHERE id = $1`, w.r.e.Rep1)
	row, _, err := w.r.e.Activities.LogActivity(w.r.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		ThreadKey: "deal-" + subject, CounterpartyEmail: "dana@buyer.example", EmailFrom: "dana@buyer.example",
		EmailTo: []string{to},
		Links: []activities.ActivityLinkInput{
			{EntityType: "contact", EntityID: w.contact}, {EntityType: "deal", EntityID: deal},
		},
	})
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	confirmWaitingRequest(t, w.r.e, ids.UUID(row.Id), at)
	w.cards[w.r.e.Rep1][ids.UUID(row.Id).String()] = true
}

// Each scope's at-risk deals are reached once each by walking the list to its
// end, in the tail's shared order with tasks and waiting cards. A deal a
// customer waits on is never served as at risk. A Done deal stays off its
// reader's list, and one that came back since its Done returns.
func TestTheAtRiskTailReachesEveryDealOnce(t *testing.T) {
	b := seedRiskBacklog(t)
	r := b.r
	for i := range plannedCapForTest + 6 {
		r.task(t, fmt.Sprintf("Rep1 due %d", i), r.now.Add(-time.Duration(i+1)*3*time.Hour), nil, &r.e.Rep1)
	}
	b.markDone(t)
	rep1 := b.atRisk[r.e.Rep1]
	mine := map[string]bool{}
	team := map[string]bool{}
	for id := range rep1 {
		if id != b.done {
			team[id], mine[id] = true, true
		}
	}
	for id := range b.atRisk[r.e.Rep2] {
		team[id] = true
	}
	checks := []struct {
		name, scope string
		ctx         context.Context
		owner       ids.UUID
		deals       map[string]bool
	}{
		{"mine", "mine", b.seat(r.e.Rep1), ids.Nil, mine},
		{"named owner", "mine", r.e.Admin(), r.e.Rep1, rep1},
		{"team", "team", b.seat(r.e.Rep1), ids.Nil, team},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			served, first, frozen := walkWorklist(check.ctx, t, r, check.scope, check.owner)
			b.assertDealsReached(t, served, check.deals)
			if total, stated := totalOf(first, "deals_at_risk"); stated && total != len(check.deals) {
				t.Errorf("deals_at_risk states %d, the walk holds %d", total, len(check.deals))
			}
			assertTailOrder(t, served, frozen)
			tailed := 0
			for _, row := range served {
				if row.source == crmcontracts.WorklistItemSourceDealAtRisk && !frozen[row.id] {
					tailed++
				}
			}
			if tailed == 0 {
				t.Error("no at-risk deal was served below the ranked top, so the tail proves nothing")
			}
		})
	}
}

// markDone has Rep1 mark two deals below the ranked top done, through the
// verb the page offers. One of them closes after the press, so it came back.
func (b *riskBacklog) markDone(t *testing.T) {
	t.Helper()
	r := b.r
	svc := newAttentionService(r.e.Pool, approvals.NewService(r.e.DB()), func() time.Time { return r.now })
	rows := []attention.RowRef{{Source: "deal_at_risk", RowID: b.done}, {Source: "deal_at_risk", RowID: b.back}}
	if err := svc.MarkRowsDone(b.seat(r.e.Rep1), rows); err != nil {
		t.Fatalf("marking deals below the ranked top done: %v", err)
	}
	r.e.WsExec(t, `UPDATE worklist_done SET done_at = $2 WHERE row_id = $1`, b.back, r.now.AddDate(0, 0, -10))
}

// assertDealsReached holds that the walk served these at-risk deals and no others,
// each once, and nothing it must never serve.
func (b *riskBacklog) assertDealsReached(t *testing.T, served []servedRow, want map[string]bool) {
	t.Helper()
	seen := map[string]bool{}
	for _, row := range served {
		if seen[string(row.source)+row.id] {
			t.Fatalf("%s %s was served twice", row.source, row.id)
		}
		seen[string(row.source)+row.id] = true
		if row.source != crmcontracts.WorklistItemSourceDealAtRisk {
			continue
		}
		if b.never[row.id] {
			t.Errorf("deal %s was served as at risk, and it never is", row.id)
		} else if !want[row.id] {
			t.Errorf("deal %s was served as at risk on a page it is not on", row.id)
		}
	}
	for id := range want {
		if !seen[string(crmcontracts.WorklistItemSourceDealAtRisk)+id] {
			t.Errorf("at-risk deal %q was never reached", b.r.e.WsScalar(t, `SELECT name FROM deal WHERE id = $1`, ids.MustParse(id)))
		}
	}
}
