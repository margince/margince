// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The shared sender rule reads by index. Each reader asks it once per contact
// or per row, so a sequential scan of contact_email or activity_participant
// would grow with the whole installation.

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

//nolint:tagliatelle // PostgreSQL EXPLAIN defines these JSON keys.
type senderPlanNode struct {
	NodeType string           `json:"Node Type"`
	Relation string           `json:"Relation Name"`
	Index    string           `json:"Index Name"`
	Plans    []senderPlanNode `json:"Plans"`
}

// seqScans lists the relations the plan reads whole.
func (p senderPlanNode) seqScans() []string {
	var out []string
	if p.NodeType == "Seq Scan" {
		out = append(out, p.Relation)
	}
	for _, child := range p.Plans {
		out = append(out, child.seqScans()...)
	}
	return out
}

func senderRulePlan(t *testing.T, tx pgx.Tx, e *Env, sql string, args ...any) senderPlanNode {
	t.Helper()
	var raw []byte
	if err := tx.QueryRow(e.Admin(), "EXPLAIN (FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		t.Fatalf("explaining %s: %v", sql, err)
	}
	var plans []struct {
		Plan senderPlanNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%s)", err, raw)
	}
	t.Logf("plan: %s", raw)
	return plans[0].Plan
}

func TestTheSenderRuleReadsByIndexOverUnrelatedRows(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Planned Sender", &e.Rep1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.Admin(), `
			WITH unrelated AS (
			  INSERT INTO contact (full_name, owner_id, visibility, source, captured_by)
			  SELECT 'Unrelated ' || n, $1, 'workspace', 'manual', 'test:plan' FROM generate_series(1, 4000) n RETURNING id),
			addresses AS (
			  INSERT INTO contact_email (contact_id, email, source, captured_by, archived_at)
			  SELECT id, 'unrelated-' || id || '@plan.test', 'manual', 'test:plan',
			         CASE WHEN random() < 0.2 THEN now() END FROM unrelated RETURNING contact_id, email),
			mail AS (
			  INSERT INTO activity (kind, subject, direction, occurred_at, source, captured_by)
			  SELECT 'email', 'hello', 'inbound', now(), 'gmail', 'connector:gmail' FROM generate_series(1, 4000) RETURNING id)
			INSERT INTO activity_participant (activity_id, role, address, contact_id)
			SELECT m.id, 'from', a.email, CASE WHEN random() < 0.5 THEN a.contact_id END
			  FROM (SELECT id, row_number() OVER () AS n FROM mail) m
			  JOIN (SELECT contact_id, email, row_number() OVER () AS n FROM addresses) a ON a.n = m.n`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding unrelated senders: %v", err)
	}
	if _, err := OwnerConn(t).Exec(e.Admin(), "ANALYZE contact_email, activity_participant, activity"); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for name, plan := range map[string]senderPlanNode{
			"subject mail from a contact": senderRulePlan(t, tx, e,
				`SELECT EXISTS (SELECT 1 FROM `+consent.SubjectMailFromSQL("$1::uuid")+`)`, contact),
			"address proves a contact": senderRulePlan(t, tx, e,
				`SELECT `+relstrength.AddressProvesSQL("$1::uuid", "$2::text"), contact, "planned@plan.test"),
		} {
			if scans := plan.seqScans(); len(scans) > 0 {
				t.Errorf("%s reads %v whole", name, scans)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
