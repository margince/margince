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
// run. The second capture must succeed, file nothing new, leave the archive
// standing, and join nothing: the archived message replies into two live
// conversations, and it must not bridge them.
func TestRecapturingAnArchivedMessageSkipsTheThreadJoin(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	addr := seatAddress(t, e, owner)
	// The bridge names both openers; its References root is the first one.
	bridge := threadMail(addr, "arch-bridge@counterparty.example", "Wed, 03 Jun 2026 08:00:00 +0000",
		"arch-b@counterparty.example", "arch-a@counterparty.example", "arch-b@counterparty.example")

	// Newest first, the way a backfill walks a mailbox: the bridge lands before
	// the openers exist, and is archived before they arrive.
	captureThreadMail(t, e, owner, bridge)
	const bridgeRow = `
		SELECT a.id FROM activity a
		  JOIN activity_identity i ON i.activity_id = a.id
		 WHERE i.identity_kind = 'mail' AND i.identity_key = 'arch-bridge@counterparty.example'`
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET archived_at = now() WHERE id = (`+bridgeRow+`)`)
		return err
	}); err != nil {
		t.Fatalf("archiving the bridge: %v", err)
	}
	captureThreadMail(t, e, owner, threadMail(addr, "arch-a@counterparty.example", "Mon, 01 Jun 2026 08:00:00 +0000", ""))
	captureThreadMail(t, e, owner, threadMail(addr, "arch-b@counterparty.example", "Tue, 02 Jun 2026 08:00:00 +0000", ""))
	before := countEmailActivities(t, e)

	captureThreadMail(t, e, owner, bridge)

	if after := countEmailActivities(t, e); after != before {
		t.Errorf("email activities went from %d to %d; the recapture must not file a second row", before, after)
	}
	var archived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT archived_at IS NOT NULL FROM activity WHERE id = (`+bridgeRow+`)`).Scan(&archived)
	}); err != nil {
		t.Fatalf("reading the bridge: %v", err)
	}
	if !archived {
		t.Error("the recapture revived the archived message; the archive must stand")
	}
	for _, id := range []string{"arch-a@counterparty.example", "arch-b@counterparty.example"} {
		if got := threadKeyOfMessage(t, e, id); got != id {
			t.Errorf("%s moved to thread %q; an archived message must not join two conversations", id, got)
		}
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
