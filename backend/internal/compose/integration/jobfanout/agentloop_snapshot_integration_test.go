// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/platform/database"
)

// auditedCall is the audit rows one tool call caused. They share one
// transaction, so they share one timestamp.
type auditedCall struct {
	// text holds what the call wrote about each seeded deal, keyed by the deal's
	// logical name. The key "" holds a call that changed a record the seed does
	// not name.
	text map[string]string
	// stamp is what the call wrote about no deal, such as an annotation's
	// narrative. It belongs to every deal the call was about.
	stamp string
}

var uuidText = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// snapshotOf reads the end state of one run. A Write is one tool call that
// changed something. The call comes from the run's trace and its changes from
// the passport's audit rows, so no list of tables is kept here. A call that
// changed nothing, such as a repeated note the store absorbs, is no Write.
func (re *runnerEnv) snapshotOf(t *testing.T, world *loopWorld, trigger string) agentloope2e.Snapshot {
	t.Helper()
	status, trace, _ := re.runRow(t, trigger)
	calls := re.auditedCalls(t, world)
	registry := compose.NewRegistry(re.pool, compose.SendPath{})
	snap := agentloope2e.Snapshot{RunStatus: runner.Outcome(status)}
	for _, step := range trace {
		spec, known := registry.Spec(step.Tool)
		if step.Admission != "executed" || !known || spec.ReadOnly() {
			continue
		}
		touched := world.namesIn(string(step.Args))
		i := slices.IndexFunc(calls, func(call auditedCall) bool { return call.touchesAny(touched) })
		if i < 0 {
			continue
		}
		snap.Writes = append(snap.Writes, calls[i].writes(step.Tool)...)
		calls = slices.Delete(calls, i, i+1)
	}
	for _, call := range calls {
		snap.Writes = append(snap.Writes, call.writes("unrecorded call")...)
	}
	return snap
}

// namesIn is the seeded names of every id a text carries, or "" when it carries
// none the seed knows.
func (w *loopWorld) namesIn(text string) []string {
	var names []string
	for _, id := range uuidText.FindAllString(text, -1) {
		if name := w.nameOf(id); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		names = []string{""}
	}
	return names
}

func (c auditedCall) touchesAny(names []string) bool {
	for _, name := range names {
		if _, ok := c.text[name]; ok {
			return true
		}
	}
	return false
}

// writes is the call as one Write for each deal it was about.
func (c auditedCall) writes(tool string) []agentloope2e.Write {
	var out []agentloope2e.Write
	for name, text := range c.text {
		out = append(out, agentloope2e.Write{Tool: tool, Subject: name, Text: strings.TrimSpace(text + " " + c.stamp)})
	}
	return out
}

// auditedCalls reads the run passport's audit rows in the order they landed and
// groups the rows of one transaction into one call.
func (re *runnerEnv) auditedCalls(t *testing.T, world *loopWorld) []auditedCall {
	t.Helper()
	var calls []auditedCall
	err := database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		found, err := tx.Query(context.Background(), `
			SELECT occurred_at, entity_type, entity_id::text, coalesce(after::text, '')
			FROM audit_log WHERE passport_id = $1 ORDER BY occurred_at, id`, re.passportID)
		if err != nil {
			return err
		}
		var rows []auditRow
		for found.Next() {
			var r auditRow
			if err := found.Scan(&r.at, &r.entityType, &r.entityID, &r.after); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		found.Close()
		if err := found.Err(); err != nil {
			return err
		}
		for i, r := range rows {
			if i == 0 || !r.at.Equal(rows[i-1].at) {
				calls = append(calls, auditedCall{text: map[string]string{}})
			}
			if err := calls[len(calls)-1].add(tx, world, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the run's audit trail: %v", err)
	}
	for i := range calls {
		if len(calls[i].text) == 0 {
			calls[i].text[""] = ""
		}
	}
	return calls
}

type auditRow struct {
	at                          time.Time
	entityType, entityID, after string
}

// add files one audit row under the seeded deals it is about. A note is about
// the deals it links and an item finding about its item's deal. A row about no
// deal, such as the run's own stamp, is the call's stamp. A note linked to no
// deal is about a record the seed does not name.
func (c *auditedCall) add(tx pgx.Tx, world *loopWorld, r auditRow) error {
	text := r.after
	var linked string
	switch r.entityType {
	case "activity":
		linked = `SELECT deal_id::text FROM activity_link WHERE activity_id = $1 AND deal_id IS NOT NULL`
		var subject, body string
		if err := tx.QueryRow(context.Background(),
			`SELECT coalesce(subject, ''), coalesce(body, '') FROM activity WHERE id = $1`, r.entityID).Scan(&subject, &body); err != nil {
			return err
		}
		text += " " + subject + " " + body
	case "brief_item":
		linked = `SELECT deal_id::text FROM brief_item WHERE id = $1`
	default:
		c.stamp += " " + text
		return nil
	}
	found, err := tx.Query(context.Background(), linked, r.entityID)
	if err != nil {
		return err
	}
	defer found.Close()
	filed := false
	for found.Next() {
		var dealID string
		if err := found.Scan(&dealID); err != nil {
			return err
		}
		c.text[world.nameOf(dealID)] += " " + text
		filed = true
	}
	if !filed {
		c.text[""] += " " + text
	}
	return found.Err()
}
