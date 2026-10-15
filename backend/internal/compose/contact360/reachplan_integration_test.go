// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contact360

// The plan behind contactReachesActivity. The Worklist asks it for every
// contact a queue page names. A plan that tests every activity against the
// contact walks the whole table per contact. Both plans return the same rows,
// so only the plan shows it.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The keys are Postgres's EXPLAIN JSON, not ours.
//
//nolint:tagliatelle // fixed by the server's plan format
type planNode struct {
	NodeType  string     `json:"Node Type"`
	Alias     string     `json:"Alias"`
	IndexName string     `json:"Index Name"`
	IndexCond string     `json:"Index Cond"`
	Plans     []planNode `json:"Plans"`
}

func (n planNode) walk(visit func(planNode)) {
	visit(n)
	for _, child := range n.Plans {
		child.walk(visit)
	}
}

// readsByPrimaryKey holds for an index scan on activity_pkey with a condition,
// and for a bitmap heap scan whose index child is that same scan.
func readsByPrimaryKey(n planNode) bool {
	if n.IndexName == "activity_pkey" && n.IndexCond != "" {
		return true
	}
	if n.NodeType != "Bitmap Heap Scan" {
		return false
	}
	byKey := false
	for _, child := range n.Plans {
		child.walk(func(c planNode) {
			if c.NodeType == "Bitmap Index Scan" && c.IndexName == "activity_pkey" && c.IndexCond != "" {
				byKey = true
			}
		})
	}
	return byKey
}

func TestAContactsReachIsReadFromItsOwnLinksAndParticipants(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, dsn)
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
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded activities: %v", err)
		}
	}()
	// Many activities and one contact on a few of them: the shape of any
	// installation, where one contact is on a sliver of the mail.
	if _, err := tx.Exec(ctx, `
	CREATE TEMP TABLE reach_contact AS SELECT gen_random_uuid() id;
	INSERT INTO contact (id, full_name, source, captured_by)
	  SELECT id, 'Reached Contact', 'manual', 'human:seed' FROM reach_contact;
	INSERT INTO activity (kind, subject, occurred_at, direction, source, captured_by)
	  SELECT 'email', 'Topic '||g, now() - g * interval '1 hour', 'inbound', 'manual', 'human:seed'
	    FROM generate_series(1, 20000) g;
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT a.id, 'contact', c.id FROM (SELECT id FROM activity ORDER BY occurred_at LIMIT 3) a, reach_contact c;
	INSERT INTO activity_participant (activity_id, contact_id, role)
	  SELECT a.id, c.id, 'to' FROM (SELECT id FROM activity ORDER BY occurred_at DESC LIMIT 3) a, reach_contact c;
	ANALYZE activity; ANALYZE activity_link; ANALYZE activity_participant;
	SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("seeding the activities: %v", err)
	}
	var contactID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM reach_contact`).Scan(&contactID); err != nil {
		t.Fatal(err)
	}
	statement := `SELECT max(a.occurred_at) FROM activity a
		WHERE a.archived_at IS NULL AND ` + fmt.Sprintf(contactReachesActivity, "$1::uuid")
	var raw string
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+statement, contactID).Scan(&raw); err != nil {
		t.Fatalf("planning the reach: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	var activityReads, walks int
	plans[0].Plan.walk(func(n planNode) {
		if n.Alias != "a" {
			return
		}
		activityReads++
		if !readsByPrimaryKey(n) {
			walks++
		}
	})
	if activityReads == 0 {
		t.Fatalf("the plan reads no activity under the alias a — the statement changed shape:\n%s", raw)
	}
	if walks != 0 {
		t.Errorf("activity is read %d times by something other than the ids the contact's links and participants name: each contact walks the whole table:\n%s", walks, raw)
	}
}
