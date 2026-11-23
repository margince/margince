// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// mailWord is the one word the seeded mail carries in its subject, its
	// body and its quoted earlier mail.
	mailWord = "brightwave"
	// mailMessages is how many mails the corpus holds. Half of them carry the
	// word, which is more than activityRankSample of each copy part.
	mailMessages = 5000
	mailIndex    = "idx_activity_search"
)

// seedMailCorpus files mailMessages mails in two mailbox copies each, linked to
// contacts of which the reader owns every other one. Each body is a few
// kilobytes of distinct words. The mails that carry mailWord quote it again and
// again, so their stored vectors are as large as a quoted thread makes them.
func seedMailCorpus(ctx context.Context, t *testing.T, tx pgx.Tx, reader ids.UUID) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO app_user (id, email, display_name) VALUES ('` + reader.String() + `', 'rep@search-budget.test', 'Rep')`,
		`INSERT INTO contact (full_name, source, captured_by, owner_id)
		   SELECT 'Customer ' || i, 'manual', 'human:seed', CASE WHEN i % 2 = 0 THEN '` + reader.String() + `'::uuid END
		   FROM generate_series(1, 2000) i`,
		`CREATE TEMP TABLE seeded ON COMMIT DROP AS
		   SELECT gen_random_uuid() id, i, k FROM generate_series(1, ` + strconv.Itoa(mailMessages) + `) i, generate_series(0, 1) k`,
		// The triggers run when a link is written, never when mail is searched,
		// so leaving them off hides no search cost.
		`ALTER TABLE activity_link DISABLE TRIGGER USER`,
		`INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at,
		                       source_system, source_id, source, captured_by)
		 SELECT id, 'email', 'Thread ' || i || CASE WHEN i % 2 = 0 THEN ' ` + mailWord + `' ELSE '' END,
		        'Hello, ' || (SELECT string_agg(md5(i::text || j), ' ') FROM generate_series(1, 400) j)
		        || CASE WHEN i % 2 = 0 THEN ' ` + mailWord + ` regards' || repeat(E'\n> ` + mailWord + ` earlier mail ' || (i % 31), 60) ELSE '' END,
		        'inbound', 'buyer' || (i % 400) || '@customer.test', now() - (i || ' minutes')::interval,
		        'email', 'msg-' || i || '@customer.test' || CASE k WHEN 0 THEN '' ELSE ' seat seat-' || k END,
		        'manual', 'human:seed'
		   FROM seeded`,
		`INSERT INTO activity_link (activity_id, entity_type, contact_id)
		   SELECT s.id, 'contact', c.id FROM seeded s
		   JOIN (SELECT id, row_number() OVER (ORDER BY id) rn FROM contact) c ON c.rn = s.i % 2000 + 1`,
		`ANALYZE activity, activity_link, contact`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the mail corpus: %v\n%s", err, statement)
		}
	}
}

// planWalk calls visit for every node of an EXPLAIN (FORMAT JSON) document.
func planWalk(t *testing.T, raw []byte, visit func(node map[string]any)) {
	t.Helper()
	var doc []map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc) != 1 {
		t.Fatalf("unreadable EXPLAIN output %s: %v", raw, err)
	}
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		visit(node)
		children, _ := node["Plans"].([]any)
		for _, child := range children {
			if m, ok := child.(map[string]any); ok {
				walk(m)
			}
		}
	}
	root, ok := doc[0]["Plan"].(map[string]any)
	if !ok {
		t.Fatalf("EXPLAIN output has no Plan: %s", raw)
	}
	walk(root)
}

// jitCompiled reports whether EXPLAIN ANALYZE shows the statement was compiled.
func jitCompiled(t *testing.T, raw []byte) bool {
	t.Helper()
	var doc []map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc) != 1 {
		t.Fatalf("unreadable EXPLAIN output %s: %v", raw, err)
	}
	_, compiled := doc[0]["JIT"]
	return compiled
}

// A word in half of all captured mail answers inside the search budget. That
// holds for an unbounded reader and for a reader who owns half the contacts
// the mail is filed under. The plan reads mail through the text index, and the
// connection does not compile it with JIT.
func TestOneWordMailSearchStaysInsideTheBudgetOnACommonWord(t *testing.T) {
	ctx := context.Background()
	// A connection built from the pool configuration production uses, asked
	// for jit=on by its DSN. The seeded mail stays inside one transaction,
	// which a separate pool could not read.
	poolConfig, err := database.PoolConfig(database.WithDSNParam(os.Getenv("MARGINCE_TEST_DSN"), "jit", "on"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := pgx.ConnectConfig(ctx, poolConfig.ConnConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back the seeded mail: %v", err)
		}
	})
	reader := ids.NewV7()
	seedMailCorpus(ctx, t, tx, reader)
	// The budget is the ceiling a request handle carries. A search over it is
	// cancelled, and rank reports it as too broad.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", database.CallerPredicateBudget.Milliseconds())); err != nil {
		t.Fatal(err)
	}

	rep := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: reader,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{entityActivity: {Read: true}, entityContact: {Read: true}},
			RowScope: principal.RowScopeOwn,
		},
	})
	for name, caller := range map[string]context.Context{
		"an unbounded reader":      readerFor(entityActivity, entityContact),
		"a reader of own contacts": rep,
	} {
		t.Run(name, func(t *testing.T) {
			in := Input{Query: mailWord, Types: []string{entityActivity}, Limit: 5}
			shape, err := rankedShapeFor(in)
			if err != nil {
				t.Fatal(err)
			}
			var args []any
			arg := func(v any) int { args = append(args, v); return len(args) }
			statement, err := rankedStatement(caller, in, mailWord, in.Types, shape, arg)
			if err != nil || statement == "" {
				t.Fatalf("no statement for the search: %q, %v", statement, err)
			}

			hits, err := rank(caller, tx, statement, args)
			if err != nil || len(hits) == 0 {
				t.Fatalf("the search found %d hits within the budget: %v", len(hits), err)
			}

			// Explained the way rank runs it: unnamed, so planned with the words.
			// testdb.ExplainCachedStatement measures a plan cached under a name,
			// which this statement never gets.
			var raw []byte
			if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement,
				append([]any{pgx.QueryExecModeDescribeExec}, args...)...).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var indexed, compiled bool
			planWalk(t, raw, func(node map[string]any) {
				if node["Node Type"] == "Bitmap Index Scan" && node["Index Name"] == mailIndex {
					indexed = true
				}
			})
			compiled = jitCompiled(t, raw)
			if !indexed {
				t.Errorf("the mail branch no longer reads %s:\n%s", mailIndex, raw)
			}
			if compiled {
				t.Errorf("the search statement compiled with JIT on a production-built connection:\n%s", raw)
			}
		})
	}
}
