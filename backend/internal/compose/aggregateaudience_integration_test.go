// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A held message is not visible in the shape of a number.
//
// None of these readers shows a word of a message. Each of them disclosed one
// anyway: a strength score that counts a founder's correspondence with their
// lawyer tells every colleague the relationship is strong, and a review queue
// that stages a held sender puts their address in front of contacts who cannot
// read the mail it came from.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestAHeldMessageIsNotCountedInRelationshipStrength drives the REAL
// ContactStrength path (contacts.Store, the one GET /contacts/{id}/strength and
// contact360 both call) rather than a hand-rolled query — a local copy of the
// audience rule proves the rule is right, not that the code applying it is.
// A test that calls its own duplicate helper instead of the real
// strengthInputs query can go green over a leak the query itself still has.
func TestAHeldMessageIsNotCountedInRelationshipStrength(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "anwalt@kanzlei.example")
	contactID := ids.From[ids.ContactKind](contact)
	now := time.Now()

	open := seedLinkedActivity(t, e, contact, "workspace")
	before, err := e.Contacts.ContactStrength(e.Admin(), contactID, now)
	if err != nil {
		t.Fatalf("reading strength: %v", err)
	}
	if before.InteractionCount90d == 0 {
		t.Fatal("the open message was not counted; the held case below would then prove nothing")
	}
	if !containsActivityID(before.ContributingIDs, open) {
		t.Fatal("the open message's id is not among the contributing ids; the held case below would then prove nothing")
	}

	held := seedLinkedActivity(t, e, contact, "participants")
	after, err := e.Contacts.ContactStrength(e.Admin(), contactID, now)
	if err != nil {
		t.Fatalf("reading strength: %v", err)
	}
	if after.InteractionCount90d != before.InteractionCount90d {
		t.Fatalf("the count moved from %d to %d when a HELD message arrived — the number tells a "+
			"colleague the message exists without showing them a word of it",
			before.InteractionCount90d, after.InteractionCount90d)
	}
	if containsActivityID(after.ContributingIDs, held) {
		t.Fatal("the held message's own id was handed back in contributing_activity_ids — its " +
			"reference number is exactly the word this field must not show")
	}
}

// TestAHeldMessageDoesNotMoveLastTouch drives the real contact360.Service —
// the composite read GET /contacts/{id}/360 serves — through lastTouchSection:
// the content gate (auth.ActivityAudienceArm, used correctly by
// readActivities in the same file) is the wrong tool for an AGGREGATE,
// which has to answer the same for every colleague and needs
// auth.AudienceWorkspaceOnly instead.
func TestAHeldMessageDoesNotMoveLastTouch(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "spaet@kanzlei.example")
	contactID := ids.From[ids.ContactKind](contact)

	// Truncated to microseconds: postgres's timestamptz resolution is
	// microseconds, so a nanosecond-precision time.Now() compares unequal to
	// its own round trip through the column — not a clock difference, a
	// storage one, and one this test would otherwise fail on at random
	// depending on which run drew a nonzero nanosecond remainder.
	openAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	seedLinkedActivityAt(t, e, contact, "workspace", openAt)

	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)

	before, err := svc.Assemble(e.Admin(), contactID)
	if err != nil {
		t.Fatalf("assembling contact360: %v", err)
	}
	if before.LastInboundAt == nil || !before.LastInboundAt.Equal(openAt) {
		t.Fatalf("last-inbound = %v, want %v; the open message was not read, "+
			"so the held case below would prove nothing", before.LastInboundAt, openAt)
	}

	// Strictly LATER than the open message: if the held one were (wrongly)
	// counted, last-inbound would visibly jump forward to it rather than
	// merely fail to move — a stronger proof than "the value is unchanged",
	// which a same-instant tie could also produce for the wrong reason.
	heldAt := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	seedLinkedActivityAt(t, e, contact, "participants", heldAt)

	after, err := svc.Assemble(e.Admin(), contactID)
	if err != nil {
		t.Fatalf("assembling contact360: %v", err)
	}
	if after.LastInboundAt == nil || !after.LastInboundAt.Equal(openAt) {
		t.Fatalf("last-inbound moved to %v when a HELD message arrived (want it to stay at %v) — "+
			"a colleague outside its audience is told a private message just came in",
			after.LastInboundAt, openAt)
	}
}

// containsActivityID reports whether an activity id appears in a strength
// result's contributing list.
func containsActivityID(list []ids.ActivityID, want ids.UUID) bool {
	for _, id := range list {
		if id.UUID == want {
			return true
		}
	}
	return false
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

func seedLinkedActivityAt(t *testing.T, e *integration.Env, contact ids.UUID, audience string, occurredAt time.Time) {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, audience, occurred_at)
			VALUES ($1, 'email', 'Betreff', 'body', 'inbound', 'gmail', $2,
			        'gmail:'||$2, 'connector:gmail', $3, $4)`,
			id, "agg-"+id.String(), audience, occurredAt); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, contact); err != nil {
			return err
		}
		// They SENT it, as capture stamps an inbound message's counterparty.
		// Without this row the mail reaches them with nobody having written it,
		// and last-inbound is a claim about who wrote.
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'from')`, id, contact)
		return err
	}); err != nil {
		t.Fatalf("seeding a linked activity: %v", err)
	}
}

func seedLinkedContact(t *testing.T, e *integration.Env, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO contact (id, full_name, source, captured_by, visibility)
			VALUES ($1, 'Test Contact', 'gmail', 'connector:gmail', 'workspace')`, id); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact_email (contact_id, email, source, captured_by)
			VALUES ($1, $2, 'gmail', 'connector:gmail')`, id, email)
		return err
	}); err != nil {
		t.Fatalf("seeding a contact: %v", err)
	}
	return id
}

func seedLinkedActivity(t *testing.T, e *integration.Env, contact ids.UUID, audience string) ids.UUID {
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
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, contact)
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

// TestLastInboundNamesTheSenderNotEveryoneOnTheThread drives the same real
// service through the other half of lastTouchSection.
//
// A thread is linked to everybody it concerns, so a message from one
// participant reaches the rest of them. Reading "they wrote last" off that
// membership told a rep the counterparty had answered when the message was
// somebody else's entirely — and the relationship brief repeats the claim in
// words, which is where a reader meets it.
func TestLastInboundNamesTheSenderNotEveryoneOnTheThread(t *testing.T) {
	e := integration.Setup(t)
	recipient := seedLinkedContact(t, e, "recipient@example.test")
	sender := seedLinkedContact(t, e, "sender@example.test")

	at := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, audience, occurred_at)
			VALUES ($1, 'email', 'Betreff', 'body', 'inbound', 'gmail', $2,
			        'gmail:'||$2, 'connector:gmail', 'workspace', $3)`,
			id, "thread-"+id.String(), at); err != nil {
			return err
		}
		// Linked to BOTH, which is what a thread naming two contacts looks like.
		for _, p := range []ids.UUID{recipient, sender} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_link (activity_id, entity_type, contact_id)
				VALUES ($1, 'contact', $2)`, id, p); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, address, role)
			VALUES ($1, $2, 'sender@example.test', 'from')`, id, sender); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, address, role)
			VALUES ($1, $2, 'recipient@example.test', 'to')`, id, recipient)
		return err
	}); err != nil {
		t.Fatalf("seeding the two-party thread: %v", err)
	}

	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)

	// The positive control FIRST: without it, a build that answered nil for
	// everybody would pass the assertion this test exists to make.
	wrote, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](sender))
	if err != nil {
		t.Fatalf("assembling the sender's 360: %v", err)
	}
	if wrote.LastInboundAt == nil || !wrote.LastInboundAt.Equal(at) {
		t.Fatalf("the SENDER's last-inbound = %v, want %v — the message is not being read at all, "+
			"so the assertion below would pass for the wrong reason", wrote.LastInboundAt, at)
	}

	received, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](recipient))
	if err != nil {
		t.Fatalf("assembling the recipient's 360: %v", err)
	}
	if received.LastInboundAt != nil {
		t.Errorf("the RECIPIENT's last-inbound = %v, want none: they were written TO, and a brief "+
			"reading this says they wrote last", received.LastInboundAt)
	}
}
