// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A purchase a human ran planted this employer, so the seat repair keeps it
// even when the contact's address is a colleague's login.
func TestTheSeatRepairKeepsAProviderEmployerAHumanBought(t *testing.T) {
	e := Setup(t)
	subject, _ := boughtSubject(t, e)
	login := e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep2)
	execAsOwner(t, e, `INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, 'work', true, 'manual', 'human:x')`, subject, login)
	live := `SELECT count(*) FROM relationship WHERE contact_id = $1 AND kind = 'employment' AND archived_at IS NULL`
	if n := e.WsCount(t, live, subject); n != 1 {
		t.Fatalf("the purchase planted %d employment edges, want 1", n)
	}

	ctx := principal.SystemActing(e.Admin(), "system:seat-repair-test")
	var page contacts.SeatEmploymentPage
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		page, err = contacts.RetireSeatEmploymentEdgesTx(ctx, tx, ids.Nil, 100, ids.NewV7().String())
		return err
	}); err != nil {
		t.Fatalf("running the seat repair: %v", err)
	}
	if page.Retired != 0 || e.WsCount(t, live, subject) != 1 {
		t.Fatalf("the repair retired %d edge(s): a human's purchase was treated as machine inference", page.Retired)
	}
}
