// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The three outcomes an imported card can have, against the real dedupe lanes.
//
// A unit test can show that the code branches on a decision; only Postgres can
// show WHICH decision a given card and a given incumbent actually produce, and
// that is the whole question here — the difference between an exact match, a
// resemblance and a stranger is what decides whether a file of forty cards
// doubles somebody's contact list.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func importCards(ctx context.Context, t *testing.T, e *dedupeEnv, file string) []VCardResult {
	t.Helper()
	entries, err := ParseVCards(strings.NewReader(file))
	if err != nil {
		t.Fatalf("parsing the file: %v", err)
	}
	results, err := e.store.ImportVCards(ctx, entries)
	if err != nil {
		t.Fatalf("importing: %v", err)
	}
	return results
}

func TestImportingAnUnknownCardCreatesThePersonAndTheirEmployer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Nadia Newcomer\nORG:Fresh Start GmbH\nTITLE:Head of Ops\n"+
			"EMAIL;TYPE=WORK:nadia@fresh-start.example\nTEL;TYPE=CELL:+49 30 4242\nEND:VCARD\n")

	if len(results) != 1 || results[0].Outcome != VCardCreated {
		t.Fatalf("outcomes = %+v, want one created", results)
	}
	if results[0].PersonID == nil {
		t.Fatal("a created card names no person")
	}
	// The employer and the edge are part of the same card: a person created
	// for the company on their card, with no link to it, is half an import.
	assertVCardEmployment(ctx, t, e, *results[0].PersonID, "Fresh Start GmbH")
}

func TestImportingACardForAKnownAddressFillsOnlyWhatIsEmpty(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// A person a HUMAN already described: their title is not the card's to
	// replace, however recent the card is.
	humanTitle := "Chief Financial Officer"
	incumbent, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Dana Known", Source: "manual", Title: &humanTitle,
		Emails: []PersonEmailInput{{Email: "dana@known.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Dana Known\nTITLE:VP Finance\nURL:https://example.com/team/dana\n"+
			"EMAIL;TYPE=WORK:dana@known.example\nEND:VCARD\n")

	if len(results) != 1 || results[0].Outcome != VCardUpdated {
		t.Fatalf("outcomes = %+v, want one updated", results)
	}
	if results[0].PersonID == nil || results[0].PersonID.UUID != ids.UUID(incumbent.Id) {
		t.Fatalf("matched %v, want the incumbent %v", results[0].PersonID, incumbent.Id)
	}

	// The title a human typed stands.
	after, err := e.store.GetPerson(ctx, ids.From[ids.PersonKind](ids.UUID(incumbent.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the person: %v", err)
	}
	if after.Title == nil || *after.Title != humanTitle {
		t.Errorf("title = %v, want the human's %q — a card may fill an empty field, never replace an answered one", after.Title, humanTitle)
	}
	// The profile URL was empty, so the card filled it.
	if got := profileFieldValue(ctx, t, e, ids.From[ids.PersonKind](ids.UUID(incumbent.Id)), profileURLField); got != "https://example.com/team/dana" {
		t.Errorf("profile url = %q, want the card's — an empty field is the card's to fill", got)
	}
}

func TestImportingACardThatMerelyResemblesSomebodyIsLeftForAHuman(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// Same name, no key in common: close enough to be the same person and not
	// close enough to be sure.
	if _, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Chris Ambiguous", Source: "manual",
		Emails: []PersonEmailInput{{Email: "chris@first.example", EmailType: emailTypeWork, IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Chris Ambiguous\nEMAIL;TYPE=WORK:chris@second.example\nEND:VCARD\n")

	if len(results) != 1 || results[0].Outcome != VCardNeedsReview {
		t.Fatalf("outcomes = %+v, want one needing review", results)
	}
	// Not written EITHER way: not merged onto the incumbent, and not created
	// beside them. Creating here is how a contact list quietly doubles.
	if got := countPeopleNamed(ctx, t, e, "Chris Ambiguous"); got != 1 {
		t.Errorf("people named Chris Ambiguous = %d, want the 1 that already existed", got)
	}
}

func TestImportingAFileReportsEveryCardInOrder(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Row Zero\nEND:VCARD\n"+
			"BEGIN:VCARD\nORG:No Name Ltd\nEND:VCARD\n"+
			"BEGIN:VCARD\nFN:Row Two\nEND:VCARD\n")

	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	// The index is what lets a reader find the row a verdict belongs to, so it
	// is the file's own order and not the order the writes happened in.
	for i, want := range []VCardOutcome{VCardCreated, VCardSkipped, VCardCreated} {
		if results[i].Index != i {
			t.Errorf("result %d carries index %d", i, results[i].Index)
		}
		if results[i].Outcome != want {
			t.Errorf("card %d outcome = %q, want %q", i, results[i].Outcome, want)
		}
	}
	if results[1].Reason == "" {
		t.Error("a skipped card gives no reason — a reader cannot act on that")
	}
}

func assertVCardEmployment(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, orgName string) {
	t.Helper()
	var found int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM relationship r
			  JOIN organization o ON o.id = r.organization_id
			 WHERE r.kind = 'employment' AND r.person_id = $1
			   AND o.display_name = $2 AND r.archived_at IS NULL`,
			personID, orgName).Scan(&found)
	}); err != nil {
		t.Fatalf("reading the employment edge: %v", err)
	}
	if found != 1 {
		t.Errorf("employment edges to %q = %d, want 1", orgName, found)
	}
}

func profileFieldValue(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, field string) string {
	t.Helper()
	var value string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(max(value), '') FROM person_profile_field
			 WHERE person_id = $1 AND field = $2`, personID, field).Scan(&value)
	}); err != nil {
		t.Fatalf("reading the profile field: %v", err)
	}
	return value
}
