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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

func TestImportingACardForAKnownAddressAppliesWhatTheCardStates(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// A person a HUMAN already described. The card is the contact's own,
	// handed over now, so what it states is the newer answer and replaces the
	// title — with the typed one kept where a reader can put it back.
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

	personID := ids.From[ids.PersonKind](ids.UUID(incumbent.Id))
	after, err := e.store.GetPerson(ctx, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the person: %v", err)
	}
	if after.Title == nil || *after.Title != "VP Finance" {
		t.Errorf("title = %v, want the card's — a card handed over now is the newer statement", after.Title)
	}
	// And the typed title is recoverable: a replacement nobody can undo is a
	// deletion, whatever it is called.
	if got := supersededFieldValue(ctx, t, e, personID, fieldTitle); got != humanTitle {
		t.Errorf("superseded title = %q, want the typed %q kept for undo", got, humanTitle)
	}
	// A company URL is a WEBSITE. Filed as linkedin — which is what this import
	// did to every card — the page then shows a company site as somebody's
	// LinkedIn profile.
	if got := profileFieldValue(ctx, t, e, personID, fieldWebsite); got != "https://example.com/team/dana" {
		t.Errorf("website = %q, want the card's", got)
	}
	if got := profileFieldValue(ctx, t, e, personID, fieldLinkedin); got != "" {
		t.Errorf("linkedin = %q, want nothing: the card states no LinkedIn profile", got)
	}
}

// supersededFieldValue reads the undo buffer one field carries.
func supersededFieldValue(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, field string) string {
	t.Helper()
	var got *string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT superseded_value FROM person_profile_field WHERE person_id = $1 AND field = $2`,
			personID, field).Scan(&got)
	}); err != nil {
		t.Fatalf("reading the superseded %s: %v", field, err)
	}
	if got == nil {
		return ""
	}
	return *got
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

// Two cards from the same company are two employees of ONE company. Without
// the lookup, a ten-card export from Acme creates ten Acmes for a human to
// merge afterwards.
func TestImportingTwoCardsFromOneCompanyCreatesOneCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// Names deliberately unalike: two colleagues sharing a mail domain score
	// on employer agreement, and a near-match is the fuzzy tier doing its job
	// rather than the question this test asks.
	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Priya Raghunathan\nORG:One Acme GmbH\nEMAIL;TYPE=WORK:priya@one-acme.example\nEND:VCARD\n"+
			"BEGIN:VCARD\nFN:Bartholomew Quist\nORG:One Acme GmbH\nEMAIL;TYPE=WORK:bart@one-acme.example\nEND:VCARD\n")

	for i, r := range results {
		if r.Outcome != VCardCreated {
			t.Fatalf("card %d outcome = %q, want created", i, r.Outcome)
		}
	}
	if got := countOrganizationsNamed(ctx, t, e, "One Acme GmbH"); got != 1 {
		t.Errorf("organizations named One Acme GmbH = %d, want 1", got)
	}
}

// The import UPDATES people, so it asks for person:update — the create grant
// that let somebody start an import says nothing about changing a record that
// already exists.
func TestImportingRefusesACallerWhoMayNotUpdatePeople(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				// Create but not update.
				"person":       {Create: true, Read: true},
				"organization": {Create: true, Read: true, Update: true},
				"relationship": {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})

	entries, err := ParseVCards(strings.NewReader("BEGIN:VCARD\nFN:Refused Import\nEND:VCARD\n"))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if _, err := e.store.ImportVCards(ctx, entries); err == nil {
		t.Fatal("the import ran without person:update")
	}
	if got := countPeopleNamed(ctx, t, e, "Refused Import"); got != 0 {
		t.Errorf("%d person row(s) landed on a refused import, want 0", got)
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

// Undo, and the two ways it must refuse.
//
// A restore that reached past a value somebody typed after the replacement
// would undo THEIR answer in order to undo the machine's, which is the one
// thing an undo must never do — so the refusal is the half worth planting.
func TestRestoringAFieldPutsBackWhatWasReplacedAndRefusesWhenTheRecordMovedOn(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	typed := "Chief Financial Officer"
	incumbent, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Rita Undo", Source: "manual", Title: &typed,
		Emails: []PersonEmailInput{{Email: "rita@undo.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	personID := ids.From[ids.PersonKind](ids.UUID(incumbent.Id))

	importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Rita Undo\nTITLE:VP Finance\n"+
			"EMAIL;TYPE=WORK:rita@undo.example\nEND:VCARD\n")

	if err := e.store.RestoreProfileField(ctx, personID, fieldTitle); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after, err := e.store.GetPerson(ctx, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the person: %v", err)
	}
	if after.Title == nil || *after.Title != typed {
		t.Errorf("title = %v after the undo, want the typed %q back", after.Title, typed)
	}
	// The buffer is one level deep and now empty, so a second undo has nothing
	// to put back rather than putting the card's value back again.
	if got := supersededFieldValue(ctx, t, e, personID, fieldTitle); got != "" {
		t.Errorf("superseded value = %q after the undo, want it cleared", got)
	}
	if err := e.store.RestoreProfileField(ctx, personID, fieldTitle); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("second undo = %v, want ErrNotFound: there is nothing left to restore", err)
	}

	// The refusal. A card replaces the title again, then somebody types their
	// own answer over it — the undo must not reach past that.
	importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Rita Undo\nTITLE:Group CFO\n"+
			"EMAIL;TYPE=WORK:rita@undo.example\nEND:VCARD\n")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE person SET title = 'Typed Since' WHERE id = $1`, personID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.RestoreProfileField(ctx, personID, fieldTitle); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("undo over a since-typed value = %v, want ErrConflict", err)
	}
	moved, err := e.store.GetPerson(ctx, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Title == nil || *moved.Title != "Typed Since" {
		t.Errorf("title = %v, want the typed answer untouched by a refused undo", moved.Title)
	}
}

// The restore's correction check spells the claim path in SQL, because the
// store may not import the module that owns the ledger. Two spellings of a
// HASHED key is the defect that already shipped once here: nothing mismatches
// visibly, the guard simply never fires and every correction is overwritten.
//
// So the SQL hash is checked against a key built the way the ledger builds
// one. Change either side alone and this fails.
func TestTheRestoreMatchesTheLedgersOwnClaimKey(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, field := range []string{fieldTitle, fieldRole, fieldWebsite} {
		want := sha256.Sum256([]byte("profile_field:" + field))
		var got string
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT encode(sha256(('profile_field:' || $1)::bytea), 'hex')`, field).Scan(&got)
		}); err != nil {
			t.Fatalf("hashing %s in SQL: %v", field, err)
		}
		if got != hex.EncodeToString(want[:]) {
			t.Errorf("%s: SQL claim key %s, ledger builds %s — the restore's correction "+
				"check would match nothing and silently overwrite every correction",
				field, got, hex.EncodeToString(want[:]))
		}
	}
}

// A card carries its own date, so importing the same file twice states the
// same thing twice.
//
// Without it, a re-upload is dated NOW and outranks everything since — so a
// reader re-uploading a file they were unsure landed puts back a detail a
// signature had already corrected. That is the whole defect; the second import
// asserting nothing new is the fix.
func TestReimportingACardDoesNotOutrankWhatCameAfterIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	card := "BEGIN:VCARD\nFN:Rev Card\nTITLE:VP Finance\nREV:20250101T090000Z\n" +
		"EMAIL;TYPE=WORK:rev@card.example\nEND:VCARD\n"

	results := importCards(ctx, t, e, card)
	if len(results) != 1 || results[0].Outcome != VCardCreated {
		t.Fatalf("outcomes = %+v, want one created", results)
	}
	personID := *results[0].PersonID

	// Something states a newer title after the card was written.
	if !fillFromSignature(ctx, t, e, personID, SignatureField{
		Name: fieldTitle, Value: "Group CFO", Evidence: "Group CFO", Confidence: 0.9,
	}) {
		t.Fatal("the signature wrote nothing, so this test proves nothing about the re-import")
	}

	// The same file again. Its REV has not moved, so it is not a newer
	// statement and the title it once carried must not come back.
	importCards(ctx, t, e, card)

	after, err := e.store.GetPerson(ctx, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the person: %v", err)
	}
	if after.Title == nil || *after.Title != "Group CFO" {
		t.Errorf("title = %v after re-importing the same card, want the newer %q — "+
			"a re-upload must not resurrect what a later statement replaced", after.Title, "Group CFO")
	}
}

// Undoing a replaced number moves the ROW, not just the evidence line.
//
// The sidecar holds one answer per field while the record holds a list, so a
// restore that rewrote only the sidecar would tell a reader their old number
// was back while the record still held the replacement — and they would find
// out by dialling it.
func TestUndoingAReplacedNumberBringsBackTheRowAReaderDials(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Nora Numbers", "nora@numbers.example", "Numbers AS", "numbers.example")

	// The number on the record, stated a month ago.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO person_phone (person_id, phone, phone_type, is_primary, position, source, captured_by, observed_at)
			VALUES ($1, '+49301111111', 'work', true, 0, 'manual', 'human:test', now() - interval '30 days')`,
			personID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !fillFromSignatureObserved(ctx, t, e, personID, time.Now().Add(-30*24*time.Hour), SignatureField{
		Name: fieldPhone, Value: "+49301111111", Evidence: "+49 30 1111111", Confidence: 0.9,
	}) {
		t.Fatal("seeding the evidence line for the old number wrote nothing")
	}

	// A newer signature states a different one.
	if !fillFromSignature(ctx, t, e, personID, SignatureField{
		Name: fieldPhone, Value: "+49302222222", Evidence: "+49 30 2222222", Confidence: 0.9,
	}) {
		t.Fatal("the replacing number wrote nothing, so there is nothing to undo")
	}
	if live := livePhones(ctx, t, e, personID); len(live) != 1 || live[0] != "+49302222222" {
		t.Fatalf("live numbers = %v, want only the replacement before the undo", live)
	}

	if err := e.store.RestoreProfileField(ctx, personID, fieldPhone); err != nil {
		t.Fatalf("restore: %v", err)
	}
	live := livePhones(ctx, t, e, personID)
	if len(live) != 1 || live[0] != "+49301111111" {
		t.Errorf("live numbers = %v after the undo, want the old number back and the "+
			"replacement retired — an undo that leaves the record dialling the new one undid nothing", live)
	}
}

// livePhones reads the numbers a reader would be offered, in position order.
func livePhones(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID) []string {
	t.Helper()
	var out []string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT phone FROM person_phone
			 WHERE person_id = $1 AND archived_at IS NULL
			 ORDER BY position, created_at`, personID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var phone string
			if err := rows.Scan(&phone); err != nil {
				return err
			}
			out = append(out, phone)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the live numbers: %v", err)
	}
	return out
}

// A card cannot date itself into the future to win forever.
//
// REV is written by whoever exported the card, so it is attacker-supplied like
// every other field on it — and unlike the others it decides what the card
// OUTRANKS. A card claiming 2099 would beat every statement the contact makes
// afterwards, permanently, from one file somebody mailed in.
func TestACardCannotDateItselfIntoTheFuture(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// The person exists first, so the card takes the UPDATE path — the one that
	// applies a card by its own date. A card that CREATES somebody states
	// nothing this rule has to arbitrate, because there is no earlier answer.
	incumbent, err := e.store.CreatePerson(ctx, CreatePersonInput{
		FullName: "Future Card", Source: "manual",
		Emails: []PersonEmailInput{{Email: "future@card.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}
	personID := ids.From[ids.PersonKind](ids.UUID(incumbent.Id))

	results := importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Future Card\nTITLE:Time Traveller\nREV:20991231T235959Z\n"+
			"EMAIL;TYPE=WORK:future@card.example\nEND:VCARD\n")
	if len(results) != 1 || results[0].Outcome != VCardUpdated {
		t.Fatalf("outcomes = %+v, want one updated", results)
	}

	// An ordinary statement made now must still win, which it cannot if the
	// card's own date was taken at face value.
	if !fillFromSignature(ctx, t, e, personID, SignatureField{
		Name: fieldTitle, Value: "Head of Present", Evidence: "Head of Present", Confidence: 0.9,
	}) {
		t.Fatal("the signature wrote nothing — a future-dated card is outranking a statement made now")
	}
	after, err := e.store.GetPerson(ctx, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title == nil || *after.Title != "Head of Present" {
		t.Errorf("title = %v, want the statement made now to win over a card claiming 2099", after.Title)
	}
}
