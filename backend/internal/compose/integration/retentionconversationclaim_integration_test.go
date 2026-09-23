// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A claim quoting a message does not outlive the message.
//
// conversation_claim carries a VERBATIM snippet of the activity it was read
// from — its writer refuses a claim that carries none — so it is a second copy
// of the subject's words held outside the message. Neither eraser reached it:
// the sweep cleared activity.body and left the quotation, and Art. 17 left it
// too, because erasure ANONYMIZES the contact row in place so the foreign key
// never cascades.
//
// Not a live disclosure — every reader joins the activity on archived_at IS
// NULL and both erasers stamp it — but a storage-limitation failure under
// Art. 5(1)(e): the copy survives a window the original has left.
//
// Seeded through RecordConversationClaim rather than an INSERT, because the
// claim's grounding is the writer's rule: a test that wrote the row itself
// could seed one with no quotation at all and prove nothing about the copy
// this erasure is for.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheRetentionSweepDestroysTheClaimsQuotingTheMessage(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Ute Sommer", nil)
	activity := ids.NewV7()

	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		inner := context.Background()
		if _, err := tx.Exec(inner, `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`); err != nil {
			return err
		}
		if _, err := tx.Exec(inner, `
			INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'Quarterly figures', 'I will send the signed order by Friday.',
			        now() - interval '400 days', 'capture', 'connector:t')`, activity); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the aged-out message: %v", err)
	}
	// Through the harness's own linker, so the column this link lands in is the
	// one the product writes rather than one this test guessed.
	LinkActivity(t, OwnerConn(t), activity, "contact", contact)

	if _, err := e.Contacts.RecordConversationClaim(ctx, contacts.ClaimInput{
		ContactID:  ids.From[ids.ContactKind](contact),
		Kind:       "commitment_theirs",
		Body:       "Send the signed order by Friday",
		ActivityID: activity,
		Quote:      "I will send the signed order by Friday.",
		Source:     "manual",
	}); err != nil {
		t.Fatalf("recording the claim through its own writer: %v", err)
	}
	if got := claimsQuoting(t, e, activity); got != 1 {
		t.Fatalf("the fixture left %d claims on the message, want 1 — without one there is nothing "+
			"for the sweep to fail to destroy", got)
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	var body *string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body FROM activity WHERE id = $1`, activity).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the message after the sweep: %v", err)
	}
	if body != nil {
		t.Fatalf("the sweep left the body %q, so this run says nothing about the quotation of it", *body)
	}
	if got := claimsQuoting(t, e, activity); got != 0 {
		t.Errorf("%d claim(s) still quote a message whose retention window closed — the words were "+
			"destroyed in one place and kept verbatim in another", got)
	}
}

// claimsQuoting counts the claims read out of one activity, by the foreign key
// rather than through a join to the activity: the join reads zero once the
// message is archived, which is the answer this test must not accept.
func claimsQuoting(t *testing.T, e *Env, activity ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM conversation_claim WHERE source_activity_id = $1`, activity).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the claims on the message: %v", err)
	}
	return n
}
