// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two runs of one release correction, read back through the receipt's own
// SQL. Each run is one maintenance line named as a data correction, counted
// by its records. An ordinary mail filing in the same window stays a plain
// line. The run id travels in audit_log.batch_id, as datatask.Execute writes
// it, so this proves the read carries the column the grouping needs.
func TestEachCorrectionRunIsOneMaintenanceLineOnTheReceipt(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	since := now.Add(-time.Hour)
	const task = "system:datatask:example/mail_refiled"
	seed := func(actor string, batch *ids.UUID, n int, name string) {
		t.Helper()
		for i := range n {
			contact := e.SeedContact(t, fmt.Sprintf("%s %d", name, i), &e.Rep1)
			if _, err := e.Pool.Exec(context.Background(), `
				INSERT INTO audit_log (id, occurred_at, action, entity_type, entity_id, actor_type, actor_id, after, batch_id)
				VALUES (uuidv7(), $4, 'update', 'contact', $1, 'system', $2,
				        '{"cohort_linked": 0, "cohort_promoted": 1}'::jsonb, $3)`,
				contact, actor, batch, now.Add(-time.Minute)); err != nil {
				t.Fatalf("seeding %s: %v", name, err)
			}
		}
	}
	first, second := ids.NewV7(), ids.NewV7()
	seed(task, &first, 3, "First run")
	seed(task, &second, 2, "Second run")
	seed("link-reconcile", nil, 1, "Ordinary filing")

	receipt, err := newMagicService(e.Pool, approvalsServiceWithEffects(e.Pool), func() time.Time { return now }).
		Read(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	var maintenance []int
	plain := 0
	for _, line := range receipt.Done {
		if line.Maintenance != nil && *line.Maintenance {
			if line.Actor.Label == nil || line.Actor.Label.Key != "magic.by.data_correction" {
				t.Errorf("a correction line is labelled %v", line.Actor.Label)
			}
			n := 1
			if line.Count != nil {
				n = *line.Count
			}
			maintenance = append(maintenance, n)
			continue
		}
		plain++
	}
	if len(maintenance) != 2 || maintenance[0]+maintenance[1] != 5 || (maintenance[0] != 3 && maintenance[1] != 3) {
		t.Fatalf("correction lines %v, want one of 3 and one of 2", maintenance)
	}
	if plain != 1 {
		t.Fatalf("ordinary lines = %d, want the one mail filing", plain)
	}
}
