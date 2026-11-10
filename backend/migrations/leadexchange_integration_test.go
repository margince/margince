// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheLeadExchangeDowngradeKeepsRecordedEvidence(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	lead, event := ids.NewV7(), ids.NewV7()
	if _, err := conn.Exec(t.Context(), `INSERT INTO lead(id,full_name,source,captured_by)
 VALUES(@id,'Exchange fixture','manual','human:fixture')`, pgx.StrictNamedArgs{"id": lead}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO consent_qualifying_event(id,lead_id,kind,note,occurred_at,source,captured_by)
 VALUES(@event,@lead,'requested_by_subject','Asked for the document.','2020-03-02T09:00:00Z','human','human:fixture')`, pgx.StrictNamedArgs{"event": event, "lead": lead}); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("core/1794266732_a_recorded_exchange_can_name_a_lead.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(t.Context(), string(down))
	var fault *pgconn.PgError
	if !errors.As(err, &fault) || fault.Code != "23502" {
		t.Fatalf("downgrade = %v; want refusal to discard recorded evidence", err)
	}
	var exists bool
	if err := conn.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM consent_qualifying_event WHERE id=@event AND lead_id=@lead)`, pgx.StrictNamedArgs{"event": event, "lead": lead}).Scan(&exists); err != nil || !exists {
		t.Fatalf("recorded exchange retained=%v, error=%v", exists, err)
	}
}
