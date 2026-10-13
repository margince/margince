// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

const agentTestIndexVersion = "20261013140000"

// moveAs puts a seeded run in status as the given actor, the way a queued move
// stores who the job acts as.
func moveAs(t *testing.T, conn *pgx.Conn, id, status, actorType, actorID string) error {
	t.Helper()
	_, err := conn.Exec(context.Background(), `
		UPDATE import_run SET status = $2, actor = jsonb_build_object('type', $3::text, 'id', $4::text)
		 WHERE id::text = $1`, id, status, actorType, actorID)
	return err
}

func TestAnAgentHoldsOneTestRunAtATime(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	seed := func(ref string) string {
		return seedLegacyRun(t, conn, legacyRun{status: "pending", sourceRef: "ws/import/" + ref})
	}

	if err := moveAs(t, conn, seed("first"), "validating", "agent", "agent-a"); err != nil {
		t.Fatalf("an agent's first test run was refused: %v", err)
	}
	err := moveAs(t, conn, seed("second"), "validating", "agent", "agent-a")
	if err == nil || !strings.Contains(err.Error(), "import_run_one_test_per_agent") {
		t.Fatalf("a second test run by one agent = %v, want the per-agent index to refuse it", err)
	}
	admitted := []struct{ name, status, actorType, actorID string }{
		{"another agent's test run", "validating", "agent", "agent-b"},
		{"one agent's report awaiting approval", "awaiting_approval", "agent", "agent-a"},
		{"a person's first test run", "validating", "human", "person-a"},
		{"a person's second test run", "validating", "human", "person-a"},
	}
	for i, run := range admitted {
		if err := moveAs(t, conn, seed(fmt.Sprint("admitted-", i)), run.status, run.actorType, run.actorID); err != nil {
			t.Errorf("%s was refused: %v; only one agent's second test run is bounded", run.name, err)
		}
	}
}

func TestTheAgentTestDownMigrationDropsItsIndex(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()

	rewound := namespaceThroughVersion(t, customNamespace(t), agentTestIndexVersion)
	if _, err := dbmigrate.Down(ctx, conn, rewound, 1); err != nil {
		t.Fatalf("down: %v", err)
	}
	var left int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE indexname = 'import_run_one_test_per_agent'`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Error("after down the per-agent test-run index is still there")
	}
	replayImportSession(t, conn)
}
