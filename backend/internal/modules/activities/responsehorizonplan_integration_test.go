// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The shape of the plan behind the waiting horizon, on an installation-sized
// mailbox.
//
// The statement measures every inbound message of a year against the mail that
// answered it, so its cost is per row and a row's cost is set by the plan. A
// result test cannot see that: a slow plan and a fast one return the same
// number. So this reads the plan.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var planCopySuffix = regexp.MustCompile(`_\d+$`)

// The keys are Postgres's EXPLAIN JSON, not ours.
//
//nolint:tagliatelle // fixed by the server's plan format
type planNode struct {
	NodeType  string     `json:"Node Type"`
	Alias     string     `json:"Alias"`
	IndexName string     `json:"Index Name"`
	IndexCond string     `json:"Index Cond"`
	Filter    string     `json:"Filter"`
	Plans     []planNode `json:"Plans"`
}

func (n planNode) walk(visit func(planNode)) {
	visit(n)
	for _, child := range n.Plans {
		child.walk(visit)
	}
}

// seedMailbox writes a year of mail shaped like a working installation's: each
// customer has a score of messages to us and several times that many from us,
// and most of ours are replies to something.
//
// Inside the caller's transaction, which the test rolls back: the suite shares
// one database, and a thousand answered messages left behind would move every
// figure the neighbouring tests measure.
func seedMailbox(ctx context.Context, t *testing.T, tx pgx.Tx, shape mailboxShape) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
	INSERT INTO contact (id, full_name, source, captured_by)
	  SELECT gen_random_uuid(), 'Customer '||g, 'manual', 'human:seed' FROM generate_series(1, 400) g;
	CREATE TEMP TABLE mailbox_customer AS SELECT id, row_number() OVER () n FROM contact WHERE full_name LIKE 'Customer %';
	INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
	  SELECT id, 'c'||n||'@customers.example', true, 'manual', 'human:seed' FROM mailbox_customer;
	CREATE TEMP TABLE mailbox_inbound AS
	  SELECT gen_random_uuid() id, g, (1 + floor(random() * 400))::int n,
	         now() - random() * 364 * interval '1 day' - interval '1 hour' at_time
	    FROM generate_series(1, 1500) g;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key, counterparty_email)
	  SELECT id, 'email', 'Topic '||g, at_time, 'inbound', 'manual', 'human:seed', 'in'||g, 'c'||n||'@customers.example'
	    FROM mailbox_inbound;
	INSERT INTO activity_participant (activity_id, address, contact_id, role)
	  SELECT i.id, 'c'||i.n||'@customers.example', c.id, 'from' FROM mailbox_inbound i JOIN mailbox_customer c ON c.n = i.n;
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT i.id, 'contact', c.id FROM mailbox_inbound i JOIN mailbox_customer c ON c.n = i.n;
	INSERT INTO activity (kind, subject, occurred_at, direction, source, captured_by, thread_key, counterparty_email, counterparty_outbound_attested)
	  SELECT 'email', 'Re: Topic '||g, at_time + interval '2 days', 'outbound', 'manual', 'human:seed', 'reply'||g, 'c'||n||'@customers.example', true
	    FROM mailbox_inbound WHERE random() < `+fmt.Sprint(shape.replyShare)+`;
	INSERT INTO activity (kind, subject, occurred_at, direction, source, captured_by, thread_key, counterparty_email, counterparty_outbound_attested)
	  SELECT 'email', 'Update '||g, now() - random() * 364 * interval '1 day', 'outbound', 'manual', 'human:seed', 'out'||g,
	         'c'||(1 + floor(random() * 400))::int||'@customers.example', true
	    FROM generate_series(1, `+fmt.Sprint(shape.strayOutbound)+`) g;
	ANALYZE activity`); err != nil {
		t.Fatalf("seeding the mailbox: %v", err)
	}
}

// mailboxShape is how much attested outbound mail the seeded installation has.
// A sparse one makes a range over the whole index look cheap to the planner, so
// it is the shape on which an arm left free to start from the activity side
// does.
type mailboxShape struct {
	replyShare    float64
	strayOutbound int
}

var (
	busyMailbox   = mailboxShape{replyShare: 0.75, strayOutbound: 5000}
	sparseMailbox = mailboxShape{replyShare: 0.05, strayOutbound: 300}
)

func TestTheHorizonMeasurementRunsEachAnswerCheckOnceAndFromTheAddressIndex(t *testing.T) {
	for name, shape := range map[string]mailboxShape{
		"busy outbound":   busyMailbox,
		"sparse outbound": sparseMailbox,
	} {
		t.Run(name, func(t *testing.T) { checkHorizonPlan(t, shape) })
	}
}

func checkHorizonPlan(t *testing.T, shape mailboxShape) {
	t.Helper()
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded mailbox: %v", err)
		}
	}()
	seedMailbox(ctx, t, tx, shape)

	statement := fmt.Sprintf(firstResponseSQL,
		scopeUnbounded,
		liveRecord(openDealPredicate, "d"),
		liveRecord(workingLeadPredicate, "ld"),
		ownDomainSenderSQL("inbound", 3),
		slowPercentile)
	now := time.Now()
	var raw string
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+statement,
		now.AddDate(0, 0, -waitingHorizonWindowDays), now, []string{"mine.example"}).Scan(&raw); err != nil {
		t.Fatalf("planning the horizon measurement: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}

	seen := map[string]int{}
	var kindIndexUnderCounterpartyArm, addressIndexBoundsKind, touchRangedOnTime int
	plans[0].Plan.walk(func(n planNode) {
		// A relation planned twice is aliased twice: answer_thread, answer_thread_1.
		alias := planCopySuffix.ReplaceAllString(n.Alias, "")
		seen[alias]++
		if alias == "answer_mail" && n.IndexName == "idx_activity_kind" {
			kindIndexUnderCounterpartyArm++
		}
		if alias == "answer_mail" && n.IndexName == "idx_activity_answer_mail" &&
			strings.Contains(n.IndexCond, "counterparty_email") && strings.Contains(n.IndexCond, "kind") {
			addressIndexBoundsKind++
		}
		// A heap scan of activity under the touch alias, or an index read keyed
		// on anything but the walked id, is the per-message walk of every
		// later call and meeting. The lateral's own Subquery Scan carries the
		// alias too and reads no relation, so it is not counted.
		heapScan := n.NodeType == "Seq Scan" || n.NodeType == "Bitmap Heap Scan"
		if alias == "answer_touch" && (heapScan || (n.IndexName != "" && !strings.Contains(n.IndexCond, "(id = "))) {
			touchRangedOnTime++
		}
	})
	// Flattened, the planner pastes the LEAST into the row filter and again
	// into the percentile, so the thread arm is planned twice.
	if seen["answer_thread"] != 1 {
		t.Errorf("the thread answer arm is planned %d times, want once — the measurement runs every answer check twice per message", seen["answer_thread"])
	}
	if addressIndexBoundsKind == 0 {
		t.Errorf("no answer arm reads idx_activity_answer_mail keyed on the sender's address and the kind: the address falls to a join filter and every message reads every later attested mail of the installation")
	}
	if touchRangedOnTime != 0 {
		t.Errorf("a touch arm reads activity %d times by something other than the linked id: it walks every later call and meeting of the installation for each message, where the sender's contact bounds it",
			touchRangedOnTime)
	}
	if kindIndexUnderCounterpartyArm != 0 {
		t.Errorf("the same-subject arm reads idx_activity_kind %d times: it walks every later email of the installation for each message, where idx_activity_answer_mail bounds it",
			kindIndexUnderCounterpartyArm)
	}
}
