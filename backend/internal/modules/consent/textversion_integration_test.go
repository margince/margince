// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Published wording against a real database: it is written once, it cannot be
// edited afterwards, and republishing the same words is a no-op rather than a
// conflict.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// The one wording these tests publish. Every test here asks what the database
// does to a published row, not what distinguishes two of them, so they share
// one fixture and name it rather than respelling it per call.
const (
	fixtureKey     = "newsletter"
	fixtureVersion = "v1"
	fixtureSubject = "Stay in touch"
	fixtureBody    = "Yes, email me."
)

func publishFixture(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := PublishTextVersionTx(e.ctx, tx, TextVersion{
			Key: fixtureKey, Version: fixtureVersion,
			Subject: fixtureSubject, Body: fixtureBody,
			PublishedAt: time.Now(),
		})
		return err
	}); err != nil {
		t.Fatalf("publishing %s@%s: %v", fixtureKey, fixtureVersion, err)
	}
}

// TestPublishedWordingCannotBeEdited is the property the whole table exists for.
//
// A proof row points at a version so a later reader can see the canonical text.
// If that text could change, every proof pointing at it would silently start
// describing a screen the subject never saw — and nothing in the tree would
// notice, because the pointer stays valid.
func TestPublishedWordingCannotBeEdited(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)

	_, err := e.owner.Exec(context.Background(),
		`UPDATE consent_text_version SET body = 'Yes, email me about anything.' WHERE key = 'newsletter'`)
	if err == nil {
		t.Fatal("published wording was edited: every proof row pointing at this version now " +
			"describes text the subject never saw, and the pointer still resolves")
	}
}

// TestAPublishedVersionMayStillBeWithdrawn — taking a version out of use is not
// rewriting what it said, so the window columns stay writable. Without this the
// only way to retire wording would be to leave it live forever.
func TestAPublishedVersionMayStillBeWithdrawn(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE consent_text_version SET effective_until = now() WHERE key = 'newsletter'`); err != nil {
		t.Fatalf("withdrawing a published version: %v — retiring wording is not rewriting it", err)
	}
}

// TestADraftIsStillEditable — the trigger keys on published_at, so wording being
// worked on is not frozen before anybody has stood behind it.
func TestADraftIsStillEditable(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO consent_text_version (key, version, body) VALUES ('draft', 'v1', 'Working on it.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE consent_text_version SET body = 'Better wording.' WHERE key = 'draft'`); err != nil {
		t.Errorf("an unpublished draft refused an edit: %v — nothing points at it yet", err)
	}
}

// TestRepublishingTheSameWordingIsANoOp holds what the catalog needs: it
// publishes on every boot, so the second call must find its row rather than
// collide with it.
func TestRepublishingTheSameWordingIsANoOp(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)
	publishFixture(t, e)

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_text_version WHERE key = 'newsletter'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("publishing the same wording twice left %d rows, want 1: the catalog publishes "+
			"on every boot and a second row would make the version ambiguous", rows)
	}
}

// TestRepublishingChangedWordingIsRefused closes the door the trigger cannot
// see. An author who edits a template and leaves its version alone is making
// exactly the change the trigger forbids, one call further out.
func TestRepublishingChangedWordingIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)

	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := PublishTextVersionTx(e.ctx, tx, TextVersion{
			Key: fixtureKey, Version: fixtureVersion, Subject: fixtureSubject,
			Body: "Yes, email me about anything.", PublishedAt: time.Now(),
		})
		return err
	})
	if err == nil {
		t.Fatal("republishing v1 with different words was accepted: the proof rows naming v1 " +
			"would now resolve to wording those subjects never saw")
	}
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		t.Errorf("refused as a validation error on %q; this is a conflict with what is already "+
			"published, not a malformed request", invalid.Field)
	}
}

// TestAProofRowNamesTheWordingItWasRenderedFrom is the link the table was added
// for: the sentence on the event, and the published version it came from.
func TestAProofRowNamesTheWordingItWasRenderedFrom(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)

	var versionID string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_text_version WHERE key = 'newsletter'`).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_event (contact_id, purpose_id, new_state, source,
		                           policy_text, policy_version, consent_text_version_id,
		                           captured_at, captured_by)
		VALUES ($1, $2, 'granted', 'test', 'Yes, email me.', 'v1', $3, now(), 'test')`,
		e.contact, e.newsletter, versionID); err != nil {
		t.Fatalf("recording a proof row naming its wording: %v", err)
	}

	var body string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT v.body FROM consent_event e
		  JOIN consent_text_version v ON v.id = e.consent_text_version_id
		 WHERE e.contact_id = $1 AND e.consent_text_version_id IS NOT NULL`,
		e.contact).Scan(&body); err != nil {
		t.Fatalf("reading the wording a proof row names: %v", err)
	}
	if body != "Yes, email me." {
		t.Errorf("the proof row resolves to %q, want the published wording", body)
	}
}

// TestPublishingTheCatalogTwiceIsANoOp holds what bootstrap depends on: it runs
// on every boot, so a second pass must find each version already published
// rather than collide with the partial unique index.
func TestPublishingTheCatalogTwiceIsANoOp(t *testing.T) {
	e := setupChannelConsent(t)
	now := time.Now()
	for i := range 2 {
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			return PublishControllerTemplatesTx(e.ctx, tx, now)
		}); err != nil {
			t.Fatalf("publishing the catalog (pass %d): %v", i+1, err)
		}
	}

	// Grouped by LOCALE as well: one template at one version legitimately has
	// three rows now, one per language this build speaks. Grouped without it,
	// the second language of every template reads as a duplicate.
	var duplicated int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM (
		  SELECT key, version, locale FROM consent_text_version
		   WHERE published_at IS NOT NULL
		   GROUP BY key, version, locale HAVING count(*) > 1
		) d`).Scan(&duplicated); err != nil {
		t.Fatal(err)
	}
	if duplicated != 0 {
		t.Errorf("%d wording(s) were published twice: a version with two rows makes "+
			"\"which text is this\" a question with two answers", duplicated)
	}

	// Scoped to the CONTROLLER templates. Counting every published wording
	// would make this test fail the day a preference-centre sentence gets its
	// own publisher — a legitimate row this test has no opinion about.
	var published int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_text_version
		  WHERE published_at IS NOT NULL
		    AND key = ANY($1)`, controllerTemplateKeys()).Scan(&published); err != nil {
		t.Fatal(err)
	}
	// Two templates in three languages. Asked as "at least 2" this would pass a
	// build that published one language and dropped the others, which is the
	// failure that leaves a German installation with no German text for a proof
	// row to point at.
	want := len(controllerTemplates) * len(mailcopy.Languages())
	if published != want {
		t.Errorf("the catalog published %d wordings, want %d — %d template(s) in %d language(s)",
			published, want, len(controllerTemplates), len(mailcopy.Languages()))
	}
}

// TestPublishedWordingCannotBeDeleted closes the door the trigger cannot see.
//
// A BEFORE UPDATE trigger says nothing about a DELETE, and the app role
// inherits one from the baseline's default privileges. Without the REVOKE, a
// published version is rewritable by deleting the row and inserting it again
// under the same key and version — and PublishTextVersionTx's hash check does
// not catch that, because after the delete there is no published row left to
// compare against.
func TestPublishedWordingCannotBeDeleted(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)

	// The RUNTIME role, not the owner: the owner may delete anything, so a test
	// running as owner would prove nothing about what the application can do.
	app, err := pgx.Connect(context.Background(), os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatalf("connecting as the runtime role: %v", err)
	}
	defer func() {
		if err := app.Close(context.Background()); err != nil {
			t.Logf("closing the runtime-role connection: %v", err)
		}
	}()

	if _, err := app.Exec(context.Background(),
		`DELETE FROM consent_text_version WHERE key = 'newsletter'`); err == nil {
		t.Fatal("the runtime role deleted published wording: it can then re-insert the same " +
			"key and version with different words, and the trigger never sees it")
	}
}

// TestAPublishedVersionsScopeIsFrozenToo — what a version applied to is
// evidence as much as what it said. Backdating effective_from makes a version
// claim it was live on a day it was not, which is how a proof row from March is
// made to resolve against wording published in June.
func TestAPublishedVersionsScopeIsFrozenToo(t *testing.T) {
	e := setupChannelConsent(t)
	publishFixture(t, e)

	for _, set := range []string{
		"effective_from = '2020-01-01'",
		"jurisdiction = 'DE'",
		"locale = 'de-DE'",
		"channel = 'email'",
	} {
		if _, err := e.owner.Exec(context.Background(),
			`UPDATE consent_text_version SET `+set+` WHERE key = 'newsletter'`); err == nil {
			t.Errorf("a published version accepted %q: what it applied to is evidence too, and "+
				"moving it leaves no trace at all", set)
		}
	}
}

// controllerTemplateKeys names the catalog's own keys, so the count above asks
// about the rows this publisher writes rather than about the table.
func controllerTemplateKeys() []string {
	keys := make([]string, 0, len(controllerTemplates))
	for key := range controllerTemplates {
		keys = append(keys, key)
	}
	return keys
}
