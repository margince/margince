// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The refresh pass driven through its worker, with the wiring River gets.
// A store built without the own-domain reader counts colleagues' fast replies
// as customers being answered. No unit over the store alone would show it.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestTheRefreshPassLeavesColleaguesMailOutOfTheSpread(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('colleagues.test', 'admin', true)`)
	// Customers wait forty days for an answer, so the horizon reaches its
	// 30-day ceiling. The colleagues' mail is answered within a minute and
	// outnumbers it: counted, the horizon falls to the fourteen-day floor.
	e.WsExec(t, `
		CREATE TEMP TABLE horizon_seed AS
		  SELECT gen_random_uuid() id, g, g <= 40 AS customer,
		         now() - interval '200 days' + g * interval '1 hour' AS at_time,
		         CASE WHEN g <= 40 THEN 'buyer'||g||'@customer.test' ELSE 'rep'||g||'@colleagues.test' END AS addr
		    FROM generate_series(1, 400) g;
		INSERT INTO contact (id, full_name, source, captured_by)
		  SELECT id, 'Seed '||g, 'seed', 'system' FROM horizon_seed;
		INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by, counterparty_email)
		  SELECT id, 'email', 'inbound', 'Topic '||g, at_time, 'in'||g, 'seed', 'system', addr FROM horizon_seed;
		INSERT INTO activity_participant (id, activity_id, role, address)
		  SELECT gen_random_uuid(), id, 'from', addr FROM horizon_seed;
		INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		  SELECT gen_random_uuid(), id, 'contact', id FROM horizon_seed;
		INSERT INTO activity (kind, direction, subject, occurred_at, thread_key, source, captured_by,
		                      counterparty_email, counterparty_outbound_attested)
		  SELECT 'email', 'outbound', 'Re: Topic '||g,
		         at_time + CASE WHEN customer THEN interval '40 days' ELSE interval '1 minute' END,
		         'reply'||g, 'seed', 'system', addr, true
		    FROM horizon_seed`)

	worker := newWaitingHorizonRefreshWorker(e.Pool, slog.New(slog.DiscardHandler))
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatalf("the refresh pass failed: %v", err)
	}

	var days int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT horizon_days FROM activity_waiting_horizon`).Scan(&days)
	})
	if err != nil || days != 30 {
		t.Fatalf("the stored horizon is (%d, %v), want the 30-day ceiling from the customers' answers alone", days, err)
	}
}
