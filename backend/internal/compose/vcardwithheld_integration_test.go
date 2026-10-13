// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A card kept from private mail by name only has no bytes to import, so the
// capture event queues nothing for it. TestAMailedCardQueuesItsImport is the
// other arm: the same trigger queues a card whose bytes were kept.
func TestAWithheldCardQueuesNoImport(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	ctx := e.Admin()

	activity := ids.NewV7()
	if _, err := e.Pool.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, body, direction, occurred_at,
		                      source_system, source_id, source, captured_by, audience)
		VALUES ($1, 'email', 'my card', 'attached', 'inbound', now(),
		        'gmail', $2, 'gmail:test', $3, 'workspace')`,
		activity, activity.String(), "connector:gmail:"+e.Rep1.String()); err != nil {
		t.Fatalf("seeding the captured message: %v", err)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return e.Activities.RecordWithheldFiles(ctx, tx, ids.From[ids.ActivityKind](activity),
			activities.CapturedFileSource{
				System: "gmail", MessageID: activity.String(), CapturedBy: "connector:gmail", Category: "email_attachment",
			}, []activities.WithheldFile{{PartID: "part:1", Filename: "card.vcf", ContentType: "text/plain", ByteSize: 120}})
	}); err != nil {
		t.Fatalf("recording the withheld card: %v", err)
	}

	inserter, err := jobs.NewInserter(e.Pool, quietIngestLog())
	if err != nil {
		t.Fatalf("building the insert-only runner: %v", err)
	}
	if err := NewVCardIngestTrigger(e.Pool, inserter, quietIngestLog()).
		HandleEvent(ctx, capturedEnvelope(t, activity)); err != nil {
		t.Fatalf("handling the capture event: %v", err)
	}
	var queued int
	if err := e.Pool.QueryRow(ctx, `
		SELECT count(*) FROM river_job WHERE kind = 'vcard_ingest' AND args->>'activity_id' = $1`,
		activity.String()).Scan(&queued); err != nil {
		t.Fatalf("counting the queued imports: %v", err)
	}
	if queued != 0 {
		t.Fatalf("the trigger queued %d imports for a card with no bytes, want 0", queued)
	}
}
