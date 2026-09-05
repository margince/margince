// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A connector files what it captures under its own word.
//
// The claim the feature makes is narrow and worth stating exactly: a contact
// this connector created carries the connector's word, a contact another
// connector created carries THAT connector's word, and a connector whose
// operator chose none files nothing. Anything looser — "some tag is applied" —
// would pass over a filer that tagged everything with the first word it found.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedTag puts one word in the vocabulary, archived or live.
func seedTag(t *testing.T, e *integration.Env, name string, archived bool) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO tag (id, name, archived_at)
			VALUES ($1, $2, CASE WHEN $3 THEN now() ELSE NULL END)`, id, name, archived)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the tag: %v", err)
	}
	return id
}

// seedFilingConnection puts one seat's connector in place, filing under tagID
// or under nothing when it is nil. Named apart from participantbackfill's
// seedConnection because that one fixes the provider: the ambiguity these tests
// probe is one seat per provider with DIFFERENT words, which needs both.
func seedFilingConnection(t *testing.T, e *integration.Env, owner ids.UUID, provider string, tagID *ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, context_tag_id)
			VALUES ($1, $2, 'connected', $3)`, provider, owner, tagID)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the connection: %v", err)
	}
}

// seedMailFrom is seedMail for a chosen connector, so a test can put two
// providers in one workspace and tell their records apart.
func seedMailFrom(t *testing.T, e *integration.Env, provider, from string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, raw, direction, source_system, source_id,
			                      source, captured_by, counterparty_email)
			VALUES ($1, 'email', 'hello', 'the message body', '{"headers":"…"}'::jsonb, 'inbound',
			        $2, $3, $2||':'||$3, 'connector:'||$2, $4)`,
			id, provider, "ctag-"+id.String(), from)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the captured mail: %v", err)
	}
	return id
}

// seedOwnedDisposition is seedPendingDisposition for a chosen mailbox owner.
func seedOwnedDisposition(t *testing.T, e *integration.Env, owner ids.UUID, email, domain string, activityID ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_pending_counterparty
			  (id, email, domain, display_name, activity_id, owner_id, status, next_attempt_at)
			VALUES ($1, $2, $3, 'Sender Name', $4, $5, 'pending', now())`,
			id, email, domain, activityID, owner)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the disposition: %v", err)
	}
	return id
}

// tagsOn names every word filed against the person behind one address.
func tagsOn(t *testing.T, e *integration.Env, email string) []string {
	t.Helper()
	var names []string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT t.name
			  FROM taggable g
			  JOIN tag t ON t.id = g.tag_id
			  JOIN person_email pe ON pe.person_id = g.entity_id
			 WHERE g.entity_type = 'person' AND pe.email = $1
			 ORDER BY t.name`, email)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			names = append(names, name)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the words filed against %s: %v", email, err)
	}
	return names
}

// Two connectors, two words, and each files only its own. Asserted as the exact
// set rather than as containment: a filer that applied every word it could find
// would satisfy "carries the Nord word" and be wrong about what a batch is.
func TestTwoConnectorsFileUnderTheirOwnWord(t *testing.T) {
	e := integration.Setup(t)
	nord := seedTag(t, e, "Nord inbox", false)
	sued := seedTag(t, e, "Sued inbox", false)
	seedFilingConnection(t, e, e.Rep1, "gmail", &nord)
	seedFilingConnection(t, e, e.Rep2, "graph", &sued)

	fromNord := seedMailFrom(t, e, "gmail", "a@nord.example")
	fromSued := seedMailFrom(t, e, "graph", "b@sued.example")
	nordID := seedOwnedDisposition(t, e, e.Rep1, "a@nord.example", "nord.example", fromNord)
	suedID := seedOwnedDisposition(t, e, e.Rep2, "b@sued.example", "sued.example", fromSued)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{
		nordID.String(): "person", suedID.String(): "person",
	}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := tagsOn(t, e, "a@nord.example"); len(got) != 1 || got[0] != "Nord inbox" {
		t.Errorf("the Nord mailbox's contact is filed under %v, want exactly [Nord inbox]", got)
	}
	if got := tagsOn(t, e, "b@sued.example"); len(got) != 1 || got[0] != "Sued inbox" {
		t.Errorf("the Sued mailbox's contact is filed under %v, want exactly [Sued inbox]", got)
	}
}

// A connector whose operator chose no word files none. The honest default: an
// absent choice, not a guessed one.
func TestAConnectorWithNoWordFilesNothing(t *testing.T) {
	e := integration.Setup(t)
	// A live word exists in the vocabulary — so a filer that reached for "any
	// tag" would have one to find, and this test would catch it.
	seedTag(t, e, "Nord inbox", false)
	seedFilingConnection(t, e, e.Rep1, "gmail", nil)

	activityID := seedMailFrom(t, e, "gmail", "c@plain.example")
	dispositionID := seedOwnedDisposition(t, e, e.Rep1, "c@plain.example", "plain.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): "person"}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := tagsOn(t, e, "c@plain.example"); len(got) != 0 {
		t.Errorf("a connector that chose no word filed its contact under %v", got)
	}
}

// A word archived after it was chosen files nothing, and the capture still
// happens. The alternative — failing the capture — would trade a mailbox going
// unread for a vocabulary edit, which is the worse of the two by a distance.
func TestAnArchivedWordFilesNothingAndTheContactIsStillMade(t *testing.T) {
	e := integration.Setup(t)
	retired := seedTag(t, e, "Retired inbox", true)
	seedFilingConnection(t, e, e.Rep1, "gmail", &retired)

	activityID := seedMailFrom(t, e, "gmail", "d@retired.example")
	dispositionID := seedOwnedDisposition(t, e, e.Rep1, "d@retired.example", "retired.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): "person"}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := tagsOn(t, e, "d@retired.example"); len(got) != 0 {
		t.Errorf("an archived word was applied anyway: %v", got)
	}
	// The contact IS made. A word is a finding aid, and refusing to create a
	// contact because a tag could not be applied would trade the product's
	// actual job for its index.
	if n := countIn(t, e, `SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		 WHERE pe.email = 'd@retired.example'`); n != 1 {
		t.Errorf("%d people created behind the address, want 1 — the capture failed over a retired word", n)
	}
}

// A contact that ALREADY EXISTS is not something this connector captured, so
// the connector's word is not applied to it.
//
// This is the assertion that keeps the created-only guard honest. Without it,
// filing every counterparty a verdict touches would pass every test above —
// and the word would then claim the batch brought in contacts that were
// already here, which is exactly the question "which records came in from this
// source" is asked to answer.
func TestAContactThatWasAlreadyHereIsNotFiledUnderTheConnectorsWord(t *testing.T) {
	e := integration.Setup(t)
	nord := seedTag(t, e, "Nord inbox", false)
	seedFilingConnection(t, e, e.Rep1, "gmail", &nord)

	const address = "incumbent@nord.example"
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		personID := ids.NewV7()
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO person (id, full_name, source, captured_by, visibility)
			VALUES ($1, 'Already Here', 'gmail', 'connector:gmail', 'workspace')`, personID); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, source, captured_by)
			VALUES ($1, $2, 'gmail', 'connector:gmail')`, personID, address)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}

	activityID := seedMailFrom(t, e, "gmail", address)
	dispositionID := seedOwnedDisposition(t, e, e.Rep1, address, "nord.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): "person"}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := tagsOn(t, e, address); len(got) != 0 {
		t.Errorf("a contact that already existed was filed under %v, claiming this connector brought them in", got)
	}
}
