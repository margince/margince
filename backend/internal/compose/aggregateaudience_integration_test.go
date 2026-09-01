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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/person360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestAHeldMessageIsNotCountedInRelationshipStrength drives the REAL
// PersonStrength path (people.Store, the one GET /people/{id}/strength and
// person360 both call) rather than a hand-rolled query — a local copy of the
// audience rule proves the rule is right, not that the code applying it is,
// which is exactly why margince#3406 shipped past the previous version of
// this test (it called its own `interactionCount` helper, never the real
// strengthInputs query strength.go's fix landed in).
func TestAHeldMessageIsNotCountedInRelationshipStrength(t *testing.T) {
	e := integration.Setup(t)
	person := seedLinkedPerson(t, e, "anwalt@kanzlei.example")
	personID := ids.From[ids.PersonKind](person)
	now := time.Now()

	open := seedLinkedActivity(t, e, person, "workspace")
	before, err := e.People.PersonStrength(e.Admin(), personID, now)
	if err != nil {
		t.Fatalf("reading strength: %v", err)
	}
	if before.InteractionCount90d == 0 {
		t.Fatal("the open message was not counted; the held case below would then prove nothing")
	}
	if !containsActivityID(before.ContributingIDs, open) {
		t.Fatal("the open message's id is not among the contributing ids; the held case below would then prove nothing")
	}

	held := seedLinkedActivity(t, e, person, "participants")
	after, err := e.People.PersonStrength(e.Admin(), personID, now)
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

// TestAHeldMessageDoesNotMoveLastTouch drives the real person360.Service —
// the composite read GET /people/{id}/360 serves — through lastTouchSection,
// the second, previously unreported gap margince#3406 found: the content
// gate (auth.ActivityAudienceArm, used correctly by readActivities in the
// same file) is the wrong tool for an AGGREGATE, which has to answer the
// same for every colleague and needs auth.AudienceWorkspaceOnly instead.
func TestAHeldMessageDoesNotMoveLastTouch(t *testing.T) {
	e := integration.Setup(t)
	person := seedLinkedPerson(t, e, "spaet@kanzlei.example")
	personID := ids.From[ids.PersonKind](person)

	openAt := time.Now().Add(-time.Hour)
	seedLinkedActivityAt(t, e, person, "workspace", openAt)

	svc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)

	before, err := svc.Assemble(e.Admin(), personID)
	if err != nil {
		t.Fatalf("assembling person360: %v", err)
	}
	if before.LastInboundAt == nil || !before.LastInboundAt.Equal(openAt) {
		t.Fatalf("last-inbound = %v, want %v; the open message was not read, "+
			"so the held case below would prove nothing", before.LastInboundAt, openAt)
	}

	// Strictly LATER than the open message: if the held one were (wrongly)
	// counted, last-inbound would visibly jump forward to it rather than
	// merely fail to move — a stronger proof than "the value is unchanged",
	// which a same-instant tie could also produce for the wrong reason.
	heldAt := time.Now().Add(time.Hour)
	seedLinkedActivityAt(t, e, person, "participants", heldAt)

	after, err := svc.Assemble(e.Admin(), personID)
	if err != nil {
		t.Fatalf("assembling person360: %v", err)
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

func seedLinkedActivityAt(t *testing.T, e *integration.Env, person ids.UUID, audience string, occurredAt time.Time) {
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
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, id, person)
		return err
	}); err != nil {
		t.Fatalf("seeding a linked activity: %v", err)
	}
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
