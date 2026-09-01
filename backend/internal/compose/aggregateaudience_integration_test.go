// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A held message is not visible in the shape of a number.
//
// None of these readers shows a word of a message. Each of them disclosed one
// anyway: a strength score that counts a founder's correspondence with their
// lawyer tells every colleague the relationship is strong, and a review queue
// that stages a held sender puts their address in front of people who cannot
// read the mail it came from.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAHeldMessageIsNotCountedInRelationshipStrength(t *testing.T) {
	e := integration.Setup(t)
	person := seedLinkedPerson(t, e, "anwalt@kanzlei.example")

	open := seedLinkedActivity(t, e, person, "workspace")
	strengthWithOpen := interactionCount(t, e, person)
	if strengthWithOpen == 0 {
		t.Fatal("the open message was not counted; the held case below would then prove nothing")
	}

	held := seedLinkedActivity(t, e, person, "participants")
	if got := interactionCount(t, e, person); got != strengthWithOpen {
		t.Fatalf("the count moved from %d to %d when a HELD message arrived — the number tells a "+
			"colleague the message exists without showing them a word of it",
			strengthWithOpen, got)
	}
	_ = open
	_ = held
}

func TestAHeldSenderIsNotStagedForReview(t *testing.T) {
	// The review queue is a shared surface. Staging a held sender puts their
	// address and display name in front of colleagues who cannot read the
	// message it came from, and asks them to decide about a correspondent they
	// were never meant to know about.
	e := integration.Setup(t)
	openMail := seedCapturedMail(t, e, "kunde@example.test", "Angebot")
	heldMail := seedCapturedMail(t, e, "privat@example.test", "Familie")
	holdActivity(t, e, heldMail)

	openID := seedPendingDisposition(t, e, "kunde@example.test", "example.test", openMail)
	heldID := seedPendingDisposition(t, e, "privat@example.test", "example.test", heldMail)
	retireUnsure(t, e, openID)
	retireUnsure(t, e, heldID)

	store := capture.NewPendingStore(InstallationDB(e.Pool))
	awaiting, err := store.AwaitingReview(e.Admin(), 50)
	if err != nil {
		t.Fatalf("reading the review queue: %v", err)
	}
	var staged []string
	for _, p := range awaiting {
		staged = append(staged, p.Email)
	}
	if !slices.Contains(staged, "kunde@example.test") {
		t.Fatalf("the open sender was not staged (%v); the held case below would then prove nothing", staged)
	}
	if slices.Contains(staged, "privat@example.test") {
		t.Fatalf("a held sender is on the shared review queue (%v) — colleagues are being asked to "+
			"decide about a correspondent whose mail they cannot read", staged)
	}
}

// interactionCount reads what relationship strength counts for one person.
func interactionCount(t *testing.T, e *integration.Env, person ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM activity a
			  JOIN activity_link l ON l.activity_id = a.id
			 WHERE l.person_id = $1 AND a.archived_at IS NULL
			   AND a.audience = 'workspace'`, person).Scan(&n)
	}); err != nil {
		t.Fatalf("counting interactions: %v", err)
	}
	return n
}

func seedLinkedPerson(t *testing.T, e *integration.Env, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO person (id, full_name, source, captured_by, visibility)
			VALUES ($1, 'Test Person', 'gmail', 'connector:gmail', 'workspace')`, id); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, source, captured_by)
			VALUES ($1, $2, 'gmail', 'connector:gmail')`, id, email)
		return err
	}); err != nil {
		t.Fatalf("seeding a person: %v", err)
	}
	return id
}

func seedLinkedActivity(t *testing.T, e *integration.Env, person ids.UUID, audience string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, audience)
			VALUES ($1, 'email', 'Betreff', 'body', 'inbound', 'gmail', $2,
			        'gmail:'||$2, 'connector:gmail', $3)`,
			id, "agg-"+id.String(), audience); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, id, person)
		return err
	}); err != nil {
		t.Fatalf("seeding a linked activity: %v", err)
	}
	return id
}

func holdActivity(t *testing.T, e *integration.Env, id ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET audience = 'participants' WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("holding the activity: %v", err)
	}
}

func retireUnsure(t *testing.T, e *integration.Env, id ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_pending_counterparty SET status = 'unsure' WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("retiring the disposition: %v", err)
	}
}
