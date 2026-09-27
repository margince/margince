// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Per-number undo when one statement replaced two numbers.

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A card that changes the German AND the Singapore number leaves two undos,
// one per number. Restoring without naming one is refused rather than guessed,
// and naming one brings back that number alone.
func TestTwoReplacedNumbersEachKeepTheirOwnUndo(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, _ := e.seedEmployedContact(ctx, t,
		"Sara Two", "sara@two.example", "Two AS", "two.example")
	seedOldWorkNumbers(ctx, t, e, contactID, "+49301111111", "+6561111111")

	importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Sara Two\nTEL;TYPE=WORK:+49 30 2222222\nTEL;TYPE=WORK:+65 6222 2222\n"+
			"EMAIL;TYPE=WORK:sara@two.example\nEND:VCARD\n")

	if got := phoneEvidence(ctx, t, e, contactID); !maps.Equal(got, map[string]string{
		"+49302222222": "+49301111111", "+6562222222": "+6561111111",
	}) {
		t.Fatalf("phone evidence (number: replaced) = %v, want each new number to name the one of its country", got)
	}

	var ambiguous *values.ParseError
	if err := e.store.RestoreProfileField(ctx, contactID, fieldPhone, ""); !errors.As(err, &ambiguous) {
		t.Fatalf("undo naming no number = %v, want a validation refusal: two numbers can be restored", err)
	}
	if err := e.store.RestoreProfileField(ctx, contactID, fieldPhone, "+65 6222 2222"); err != nil {
		t.Fatalf("undo of the Singapore number: %v", err)
	}
	if got := livePhones(ctx, t, e, contactID); !slices.Equal(slices.Sorted(slices.Values(got)),
		[]string{"+49302222222", "+6561111111"}) {
		t.Errorf("live numbers = %v, want the old Singapore number back and the new German one untouched", got)
	}
}

func seedOldWorkNumbers(ctx context.Context, t *testing.T, e *dedupeEnv, contactID ids.ContactID, numbers ...string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for i, number := range numbers {
			if _, err := tx.Exec(ctx, `
				INSERT INTO contact_phone (contact_id, phone, phone_type, is_primary, position, source, captured_by, observed_at)
				VALUES ($1, $2, 'work', $3, $4, 'manual', 'human:test', now() - interval '30 days')`,
				contactID, number, i == 0, i); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Two German numbers, and a newer statement that changes the mobile and lists
// it FIRST. The unchanged desk number is confirmed before anything looks for a
// number to replace, so the new mobile replaces the old mobile and not the desk
// number it happens to precede.
func TestAChangedNumberListedFirstDoesNotRetireAnUnchangedOneOfItsCountry(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, _ := e.seedEmployedContact(ctx, t,
		"Jonas Desk", "jonas@desk.example", "Desk AS", "desk.example")
	seedOldWorkNumbers(ctx, t, e, contactID, "+49301111111", "+491701111111")

	importCards(ctx, t, e,
		"BEGIN:VCARD\nFN:Jonas Desk\nTEL;TYPE=WORK:+49 170 2222222\nTEL;TYPE=WORK:+49 30 1111111\n"+
			"EMAIL;TYPE=WORK:jonas@desk.example\nEND:VCARD\n")

	if got := slices.Sorted(slices.Values(livePhones(ctx, t, e, contactID))); !slices.Equal(got,
		[]string{"+491702222222", "+49301111111"}) {
		t.Errorf("live numbers = %v, want the desk number kept and the mobile replaced", got)
	}
	if got := phoneEvidence(ctx, t, e, contactID)["+491702222222"]; got != "+491701111111" {
		t.Errorf("the new mobile records it replaced %q, want the old mobile", got)
	}
}

// A search result is somebody else's description of the contact, so it claims
// only an unanswered field. A second number is a different row, and without
// the guard it would land beside the number the contact stated themselves.
func TestADiscoveredNumberDefersToAStatedOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, _ := e.seedEmployedContact(ctx, t,
		"Ida Stated", "ida@stated.example", "Stated AS", "stated.example")
	if !fillFromSignature(ctx, t, e, contactID, SignatureField{
		Name: fieldPhone, Value: "+49 30 5550101", Evidence: "+49 30 5550101", Confidence: 0.9,
	}) {
		t.Fatal("the signature wrote no number, so this test proves nothing about the search fill")
	}
	applied, err := e.store.ApplyDiscoveredFields(ctx, contactID, []DiscoveredField{{
		Field: fieldPhone, Value: "+65 9555 0102", EvidenceSnippet: "Ida Stated, +65 9555 0102", SourceRef: "search:test",
	}})
	if err != nil {
		t.Fatalf("applying the discovered number: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("applied = %v, want nothing: the phone field is already answered", applied)
	}
	if got := phoneEvidence(ctx, t, e, contactID); !maps.Equal(got, map[string]string{"+49305550101": ""}) {
		t.Errorf("phone evidence = %v, want only the stated number", got)
	}
}

// Research already put evidence for number B on the record — without adding B
// to the number list — and then a newer signature replaces the live number A
// with B. B's row already exists, so the replacement lands as an update of it;
// the undo must still name A, whether research spelled B the canonical way or
// its own.
func TestAReplacementOntoExistingEvidenceKeepsItsUndo(t *testing.T) {
	for name, researched := range map[string]string{
		"canonical": "+49302222222",
		"formatted": "+49 (30) 222-2222",
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			contactID, _ := e.seedEmployedContact(ctx, t,
				"Rhea Research", "rhea@research.example", "Research AS", "research.example")
			seedOldWorkNumbers(ctx, t, e, contactID, "+49301111111")
			if _, err := e.store.SaveResearchClaims(ctx, contactID, []ResearchClaimInput{{
				Field: fieldPhone, Value: researched, Quote: "Rhea Research, " + researched,
				SourceURL: "https://research.example/team",
			}}); err != nil {
				t.Fatalf("accepting the researched number: %v", err)
			}

			if !fillFromSignature(ctx, t, e, contactID, SignatureField{
				Name: fieldPhone, Value: "+49 30 2222222", Evidence: "+49 30 2222222", Confidence: 0.9,
			}) {
				t.Fatal("the signature wrote no number, so there is nothing to undo")
			}
			if got := phoneEvidence(ctx, t, e, contactID); !maps.Equal(got, map[string]string{"+49302222222": "+49301111111"}) {
				t.Fatalf("phone evidence (number: replaced) = %v, want B's row to name A as what it replaced", got)
			}
			if err := e.store.RestoreProfileField(ctx, contactID, fieldPhone, ""); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if got := livePhones(ctx, t, e, contactID); !slices.Equal(got, []string{"+49301111111"}) {
				t.Errorf("live numbers = %v after the undo, want A back and B retired", got)
			}
		})
	}
}

// The same number in another spelling is not a replacement: a signature
// stating the number research already recorded leaves no undo behind.
func TestARespelledNumberIsNotAReplacement(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, _ := e.seedEmployedContact(ctx, t,
		"Otto Spelling", "otto@spelling.example", "Spelling AS", "spelling.example")
	if _, err := e.store.SaveResearchClaims(ctx, contactID, []ResearchClaimInput{{
		Field: fieldPhone, Value: "+49 (30) 222-2222", Quote: "Otto Spelling, +49 (30) 222-2222",
		SourceURL: "https://spelling.example/team",
	}}); err != nil {
		t.Fatalf("accepting the researched number: %v", err)
	}
	if !fillFromSignature(ctx, t, e, contactID, SignatureField{
		Name: fieldPhone, Value: "+49 30 2222222", Evidence: "+49 30 2222222", Confidence: 0.9,
	}) {
		t.Fatal("the signature wrote no number")
	}
	if got := phoneEvidence(ctx, t, e, contactID); !maps.Equal(got, map[string]string{"+49302222222": ""}) {
		t.Errorf("phone evidence (number: replaced) = %v, want one row that replaced nothing", got)
	}
}
