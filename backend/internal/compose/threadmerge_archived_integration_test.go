// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Capture archives mail itself (the counterparty verdict does). A backfill or
// sync that meets the same message again resolves onto that archived row, and
// the thread join used to fail on it with "reading the message's thread: no
// rows in result set" — which failed the backfill page and closed the whole
// run. The second capture must succeed, file nothing new, and leave the
// archive standing.
func TestRecapturingAnArchivedMessageSkipsTheThreadJoin(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	addr := seatAddress(t, e, owner)
	opener := threadMail(addr, "arch-opener@counterparty.example", "Mon, 01 Jun 2026 08:00:00 +0000", "")
	reply := threadMail(addr, "arch-reply@counterparty.example", "Tue, 02 Jun 2026 08:00:00 +0000",
		"arch-opener@counterparty.example", "arch-opener@counterparty.example")

	captureThreadMail(t, e, owner, opener)
	captureThreadMail(t, e, owner, reply)

	const replyRow = `
		SELECT a.id FROM activity a
		  JOIN activity_identity i ON i.activity_id = a.id
		 WHERE i.identity_kind = 'mail' AND i.identity_key = 'arch-reply@counterparty.example'`
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET archived_at = now() WHERE id = (`+replyRow+`)`)
		return err
	}); err != nil {
		t.Fatalf("archiving the reply: %v", err)
	}
	before := countEmailActivities(t, e)

	captureThreadMail(t, e, owner, reply)

	if after := countEmailActivities(t, e); after != before {
		t.Errorf("email activities went from %d to %d; the recapture must not file a second row", before, after)
	}
	var archived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT archived_at IS NOT NULL FROM activity WHERE id = (`+replyRow+`)`).Scan(&archived)
	}); err != nil {
		t.Fatalf("reading the reply: %v", err)
	}
	if !archived {
		t.Error("the recapture revived the archived reply; the archive must stand")
	}
	if got := threadKeyOfMessage(t, e, "arch-opener@counterparty.example"); got != "arch-opener@counterparty.example" {
		t.Errorf("the opener moved to thread %q", got)
	}
}

func countEmailActivities(t *testing.T, e *integration.Env) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity WHERE kind = 'email'`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting email activities: %v", err)
	}
	return n
}
