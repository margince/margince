// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An extension's AI task is a task like any other once the boot registers it.
// AI Usage lists it on its own line under the name the unit declared, and a
// call about one record names that record, so an erasure finds the payload.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func registerReportTasks(t *testing.T) {
	t.Helper()
	extTask := func(name, shown string) ai.ExtensionTask {
		return ai.ExtensionTask{
			Task: ai.Task(name), Unit: "reports", DisplayName: shown, Summary: "Written by the reports unit.",
			Ladder: []ai.Tier{ai.TierCheapCloud}, Sites: []string{"one_shot"},
		}
	}
	if err := ai.RegisterExtensionTasks([]ai.ExtensionTask{
		extTask("ext_reports_weekly", "Weekly report"), extTask("ext_reports_digest", "Digest"),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ai.ResetExtensionTasks)
}

func TestAiUsageGroupsByExtensionTask(t *testing.T) {
	registerReportTasks(t)
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO ai_usage (day, task, tier, calls, cached_hits, tokens_in, tokens_out) VALUES
		('2026-07-10', 'ext_reports_weekly', 'cheap_cloud', 3, 0, 900, 90),
		('2026-07-10', 'ext_reports_digest', 'cheap_cloud', 1, 0, 100, 10)`); err != nil {
		t.Fatalf("seeding ai_usage: %v", err)
	}
	var usage struct {
		Days []struct {
			Tasks []struct {
				Task            string `json:"task"`
				TaskDisplayName string `json:"task_display_name"`
				Calls           int    `json:"calls"`
			} `json:"tasks"`
		} `json:"days"`
	}
	if status := e.Call(t, "GET", "/v1/ai/usage?from=2026-07-01&to=2026-07-14", nil, nil, &usage); status != http.StatusOK {
		t.Fatalf("GET /ai/usage → %d, want 200", status)
	}
	if len(usage.Days) != 1 {
		t.Fatalf("%d days, want 1", len(usage.Days))
	}
	shown := map[string]string{}
	calls := map[string]int{}
	for _, line := range usage.Days[0].Tasks {
		shown[line.Task], calls[line.Task] = line.TaskDisplayName, line.Calls
	}
	if len(shown) != 2 || shown["ext_reports_weekly"] != "Weekly report" || shown["ext_reports_digest"] != "Digest" {
		t.Errorf("lines = %v, want one line per extension task under its declared name", shown)
	}
	if calls["ext_reports_weekly"] != 3 || calls["ext_reports_digest"] != 1 {
		t.Errorf("calls = %v, want the two tasks kept apart", calls)
	}
}

func TestErasureFindsAnExtensionCallBySubject(t *testing.T) {
	registerReportTasks(t)
	e := Setup(t)
	contactID := seedSubject(t, e)
	record := func(subject ids.Ref) ids.UUID {
		id := ids.NewV7()
		e.WsExec(t, `INSERT INTO ai_call (id, logical_call_id, task, request_fingerprint, subject_type, subject_id)
			VALUES ($1, $1, 'ext_reports_weekly', 'fp-ext', $2, $3)`, id, subject.Type, subject.ID)
		e.WsExec(t, `INSERT INTO ai_call_payload (ai_call_id, request_payload, response_payload)
			VALUES ($1, '{"messages":[{"role":"user","content":"a summary that names nobody"}]}'::jsonb, '{}'::jsonb)`, id)
		return id
	}
	about := record(ids.Ref{Type: "contact", ID: contactID})
	other := record(ids.Ref{Type: "contact", ID: ids.NewV7()})
	if err := privacy.NewEraser(e.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).EraseContact(e.Admin(), contactID, "test"); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM ai_call_payload WHERE ai_call_id = $1`, about); n != 0 {
		t.Errorf("the payload of an extension call about the subject survived: %d rows", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM ai_call_payload WHERE ai_call_id = $1`, other); n != 1 {
		t.Errorf("a payload about somebody else was purged: %d rows, want 1", n)
	}
}
