// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A task is current by its source only when that source is named: no source,
// an importer's namespace, or a declared automatic family. An unregistered
// source_system is not current work until a human writes or adopts the task.
func TestOnlyANamedTaskSourceIsCurrentWork(t *testing.T) {
	e := setupLoad(t)
	asOf := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	seed := func(source *string) ids.UUID {
		id := ids.NewV7()
		e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, is_done, assignee_id, source, captured_by, source_system)
			VALUES ($1, 'task', 'Follow up', $2, $2, false, $3, 'system', 'system', $4)`, id, asOf, e.rep, source)
		return id
	}
	text := func(s string) *string { return &s }
	cases := map[string]struct {
		id   ids.UUID
		want bool
	}{
		"no source":           {seed(nil), true},
		"empty source":        {seed(text("")), true},
		"imported":            {seed(text(provenance.ReservedSourceSystemPrefix + "csv")), true},
		"declared automatic":  {seed(text(provenance.LeadSLATaskSource)), true},
		"unregistered source": {seed(text("new-bot")), false},
	}
	for name, tc := range cases {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		idPos := arg(tc.id)
		clause := storekit.CurrentTaskClause("a", asOf, "'{}'::jsonb", arg)
		var current bool
		if err := e.owner.QueryRow(context.Background(),
			fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%d AND %s)", idPos, clause), args...).Scan(&current); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if current != tc.want {
			t.Errorf("%s: current = %v, want %v", name, current, tc.want)
		}
	}
}
