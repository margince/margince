// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unownedPromiseOnDay is a dated promise whose owner "Rep" names every seat.
func unownedPromiseOnDay(t *testing.T, day string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": "Send the revised pricing", "owner": "Rep", "promiser_ref": "",
		"due_date": day, "source_lines": []int{3}, "confidence": 0.9,
		"commitment": &commitmentMeaning{State: "open", DeadlinePrecision: "day", DeadlineQuote: "Friday"},
	}}})
	if err != nil {
		t.Fatalf("building the model reply: %v", err)
	}
	return string(raw)
}

func TestADayInsideTheWindowIsStoredOnTheTask(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	day := clocktest.Now(t).AddDate(0, 0, 7).Format(time.DateOnly)

	read := e.read(t, cannedBrain{reply: datedReply(t, 3, day, e.Rep2)})
	if len(read.ProposalIDs) != 0 || e.taskCount(t) != 1 {
		t.Fatalf("an ordinary day gave %d proposal(s) and %d task(s), want one task", len(read.ProposalIDs), e.taskCount(t))
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND due_at IS NOT NULL`); n != 1 {
		t.Errorf("the task kept no due date for %s", day)
	}
}

func TestAFarOffDayIsPutToThePromiserOnceAndNotStored(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	day := clocktest.Now(t).AddDate(15, 0, 0).Format(time.DateOnly)
	reply := datedReply(t, 3, day, e.Rep2)

	first := e.read(t, cannedBrain{reply: reply})
	again := e.read(t, cannedBrain{reply: reply})
	if len(first.ProposalIDs) != 1 || len(again.ProposalIDs) != 0 {
		t.Fatalf("a far day raised %d question(s), then %d on the second reading; want 1 then 0", len(first.ProposalIDs), len(again.ProposalIDs))
	}
	if e.taskCount(t) != 0 {
		t.Fatal("a task with a far day was written before the colleague answered")
	}
	asked := e.wsString(t, `SELECT requested_decider_id::text || ' ' || (proposed_change->>'odd_due_date') FROM approval WHERE kind = 'commitment_task'`)
	if want := e.Rep2.String() + " " + day; asked != want {
		t.Errorf("the question reads %q, want %q", asked, want)
	}
}

func TestKeepingTheFarOffDayWritesItAsTheColleague(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	day := clocktest.Now(t).AddDate(15, 0, 0).Format(time.DateOnly)
	read := e.read(t, cannedBrain{reply: datedReply(t, 3, day, e.Rep2)})

	priya := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	if _, err := e.svc.Decide(priya, ids.From[ids.ApprovalKind](read.ProposalIDs[0]), true, nil); err != nil {
		t.Fatalf("keeping the day: %v", err)
	}
	got := e.wsString(t, `SELECT to_char(due_at AT TIME ZONE 'UTC', 'YYYY') || ' ' || captured_by FROM activity WHERE kind = 'task'`)
	want := e.wsString(t, `SELECT to_char(now() + interval '15 years', 'YYYY')`) + " human:" + e.Rep2.String()
	if got != want {
		t.Errorf("the kept task reads %q, want %q", got, want)
	}
}

func TestAFarOffDayWithNoNamedOwnerAsksNobody(t *testing.T) {
	e := setupTranscript(t)
	day := clocktest.Now(t).AddDate(15, 0, 0).Format(time.DateOnly)

	read := e.read(t, cannedBrain{reply: unownedPromiseOnDay(t, day)})
	e.assertNobodyAsked(t, read)
}
