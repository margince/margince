// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// An older build queued version 2 rows without the sending entity, the chosen
// mailbox or the stated countries. Such a row still fires and sends once.
func TestAScheduledRowFromAnOlderBuildStillFires(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	id := p.scheduleFor(t, clocktest.Now(t).Add(2*time.Hour))

	var version int
	var hadEntity bool
	if err := p.Owner.QueryRow(t.Context(),
		`SELECT payload_version, payload ? 'sending_entity' FROM scheduled_send WHERE id = $1`, id).Scan(&version, &hadEntity); err != nil {
		t.Fatal(err)
	}
	if version != 2 || !hadEntity {
		t.Fatalf("the row is version %d, carries a sending entity: %t; the fixture proves nothing", version, hadEntity)
	}
	if _, err := p.Owner.Exec(t.Context(), `
		UPDATE scheduled_send
		   SET payload = jsonb_set(payload - 'sending_entity' - 'from_connection', '{evidence}',
		                           coalesce(payload->'evidence', '{}') - 'recipient_countries')
		 WHERE id = $1`, id); err != nil {
		t.Fatalf("rewriting the row to the older shape: %v", err)
	}

	before := p.countDeliveries(t)
	p.makeDue(t, id)
	p.fire(t, id)
	if !p.sent(t, id) {
		status, reason := p.scheduledStatus(t, id)
		t.Fatalf("the older row reads %q/%q after its timer, want released", status, reason)
	}
	if got := p.countDeliveries(t) - before; got != 1 {
		t.Fatalf("the older row queued %d deliveries, want 1", got)
	}
}
