// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Retention is on the receipt, as a count, for a reader who may read the
// retention policies — and a record it anonymized is still never named.
//
// The done lane leaves out every audit image of a scrubbed record, which also
// hid the one machine action that destroys data: on a rehearsal install the
// retention pass anonymized 1,200 imported leads and this page never said so.

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRetentionIsOnTheReceiptAsACountWithoutNamingTheRecords(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for range 3 {
			lead := ids.NewV7()
			if _, err := tx.Exec(ctx, `INSERT INTO lead (id, full_name, status, source, captured_by)
				VALUES ($1, 'Anonymized Lead', 'new', 'manual', 'human:x')`, lead); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, evidence, occurred_at)
				VALUES ('system', 'system', 'anonymize', 'lead', $1,
				        '{"policy": "p", "retain_days": 365, "retention_action": "anonymize"}', now() - interval '5 minutes')`,
				lead); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	svc := magic.NewService(e.Pool, nil, time.Now)

	perms := RepPerms
	perms.Objects = maps.Clone(RepPerms.Objects)
	perms.Objects["retention_policy"] = principal.ObjectGrant{Read: true}
	receipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	var found bool
	for _, line := range receipt.Done {
		if line.Summary.Key != "magic.action.retention_lead_anonymize" {
			continue
		}
		found = true
		if line.Count == nil || *line.Count != 3 {
			t.Errorf("count %v, want the three anonymized leads", line.Count)
		}
		if line.Entity != nil {
			t.Error("the retention line names a record; an anonymized record must not be named")
		}
		if line.Reason == nil || (*line.Reason.Values)["days"] != "365" {
			t.Errorf("reason %v, want the policy's window", line.Reason)
		}
		if line.Actor.Label == nil || line.Actor.Label.Key != "magic.by.retention" {
			t.Errorf("actor %v, want retention named as the actor", line.Actor.Label)
		}
	}
	if !found {
		t.Fatal("three anonymizations in the window and no retention line on the receipt")
	}

	repReceipt, err := svc.Read(e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the rep's receipt: %v", err)
	}
	for _, line := range repReceipt.Done {
		if line.Summary.Key == "magic.action.retention_lead_anonymize" {
			t.Fatal("a reader without retention read access was shown the installation's retention count")
		}
	}
}
