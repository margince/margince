// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logEntry plants one system_log row of an action, days old.
func logEntry(t *testing.T, e *Env, action, detail string, days int) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO system_log (id, actor_type, actor_id, action, detail, occurred_at)
		VALUES ($1, 'system', 'system:test', $2, $3::jsonb, now() - make_interval(days => $4))`,
		id, action, detail, days)
	return id
}

// The nightly pass removes a listed entry past its window. It keeps a young
// one, an action with no window, and a sign-in row that still waits for its key.
func TestTheNightlyPassRemovesOldLogEntriesOfTheListedKinds(t *testing.T) {
	e := Setup(t)
	const keyed = `{"outcome":"failed","email_hash":"aa","email_key_id":"bb"}`
	gone := map[string]ids.UUID{
		"an old sync fault":     logEntry(t, e, "capture_sync_error", `{}`, 91),
		"an old task state":     logEntry(t, e, "ai_task.state_changed", `{}`, 91),
		"an old keyed sign-in":  logEntry(t, e, "login", keyed, 401),
		"an old password reset": logEntry(t, e, "password_reset", `{}`, 401),
	}
	kept := map[string]ids.UUID{
		"a young sync fault":             logEntry(t, e, "capture_sync_error", `{}`, 10),
		"a sign-in inside its year":      logEntry(t, e, "login", keyed, 100),
		"an old sign-in without its key": logEntry(t, e, "login", `{"outcome":"failed","email_hash":"aa"}`, 401),
		"an old export":                  logEntry(t, e, "export", `{}`, 900),
		"an old factor enrolment":        logEntry(t, e, "mfa_enrolled", `{}`, 900),
	}

	if err := compose.NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}

	for name, id := range gone {
		if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE id = $1`, id); n != 0 {
			t.Errorf("%s stayed past its window", name)
		}
	}
	for name, id := range kept {
		if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE id = $1`, id); n != 1 {
			t.Errorf("%s was removed", name)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'retention.pruned'
		AND detail->>'rows' LIKE '%of the system log' AND (detail->>'count')::int = 2`); n != 2 {
		t.Errorf("the pass left %d record(s) of its two prunes, want one for each window with two rows", n)
	}
}

// The database admits a delete for the actions the prune lists and no other,
// and only past their window. The two lists then cannot part.
func TestTheSystemLogAdmitsADeleteOnlyForAListedActionPastItsWindow(t *testing.T) {
	e := Setup(t)
	remove := func(id ids.UUID) error {
		return e.WsExecErr(t, `DELETE FROM system_log WHERE id = $1`, id)
	}
	for _, window := range storekit.SystemLogWindows() {
		for _, action := range window.Actions {
			if err := remove(logEntry(t, e, action, `{}`, window.Days+1)); err != nil {
				t.Errorf("%s past its %d days was refused: %v", action, window.Days, err)
			}
			if err := remove(logEntry(t, e, action, `{}`, window.Days-1)); err == nil {
				t.Errorf("%s inside its %d days was deleted", action, window.Days)
			}
		}
	}
	if err := remove(logEntry(t, e, "login", `{"outcome":"failed","email_hash":"aa"}`, 5000)); err == nil {
		t.Errorf("a sign-in row that waits for its key was deleted")
	}
	for _, action := range []string{"export", "retention.pruned", "installation_bootstrap", "an action nobody listed"} {
		if err := remove(logEntry(t, e, action, `{}`, 5000)); err == nil {
			t.Errorf("%s has no window and was deleted", action)
		}
	}
}

// The nightly pass removes a completion or decision trace past 400 days.
// It keeps a younger one, and one whose stored content has its own rule.
func TestTheNightlyPassRemovesAnswerTracesPastTheirYear(t *testing.T) {
	e := Setup(t)
	trace := func(kind string, days int) ids.UUID {
		id := ids.NewV7()
		e.WsExec(t, `INSERT INTO ai_call (id, logical_call_id, task, tier, kind, occurred_at)
			VALUES ($1, $1, 'chat', 'fast', $2, now() - make_interval(days => $3))`, id, kind, days)
		return id
	}
	gone := []ids.UUID{trace("completion", 401), trace("decision", 401)}
	withContent := trace("completion", 401)
	e.WsExec(t, `INSERT INTO ai_call_payload (ai_call_id, request_payload, response_payload, occurred_at)
		VALUES ($1, '{}'::jsonb, '{}'::jsonb, now() - make_interval(days => 401))`, withContent)
	kept := []ids.UUID{trace("completion", 399), trace("decision", 100), withContent}

	if err := compose.NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}

	for _, id := range gone {
		if n := e.WsCount(t, `SELECT count(*) FROM ai_call WHERE id = $1`, id); n != 0 {
			t.Errorf("a trace past 400 days stayed")
		}
	}
	for _, id := range kept {
		if n := e.WsCount(t, `SELECT count(*) FROM ai_call WHERE id = $1`, id); n != 1 {
			t.Errorf("a trace inside 400 days, or one with stored content, was removed")
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'retention.pruned'
		AND detail->>'rows' = 'completion and decision call traces' AND (detail->>'count')::int = 2`); n != 1 {
		t.Errorf("the pass left %d record(s) of the prune, want one that counts both traces", n)
	}
}
