// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a signature may do to a name the record already carries.
//
// A display name split into the two columns is a guess. "Greven Ebusiness"
// parses to first `Greven` and last `Ebusiness`, and every greeting then uses
// it. The contact is Marcus Greven, and his own signature says so.
//
// The precedence is human, then what the conversation shows, then the split.

import (
	"testing"
)

func TestASignedNameReplacesWhatTheDisplaySplitGuessed(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const addr = "m.greven@greven.test"

	// Capture mints the contact from a display name that is a brand, and the
	// split lands it in the name columns.
	res, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, addr, "Greven Ebusiness", "greven.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, first, last := e.storedName(ctx, t, res.ContactID); nameOrNull(first) != "Greven" || nameOrNull(last) != "Ebusiness" {
		t.Fatalf("the split did not land: %q / %q", nameOrNull(first), nameOrNull(last))
	}

	// His own signature names him.
	if _, err := e.store.ApplySignatureFields(ctx, res.ContactID, e.openSignatureSource(ctx, t, res.ContactID), []SignatureField{
		{Name: FieldFirstName, Value: "Marcus", Evidence: "Marcus Greven", Confidence: 0.9},
		{Name: FieldLastName, Value: "Greven", Evidence: "Marcus Greven", Confidence: 0.9},
	}, true); err != nil {
		t.Fatalf("applying the signed name: %v", err)
	}

	_, first, last := e.storedName(ctx, t, res.ContactID)
	if nameOrNull(first) != "Marcus" {
		t.Errorf("first_name = %q, want Marcus: he signs his own mail, and the split only ever guessed",
			nameOrNull(first))
	}
	if nameOrNull(last) != "Greven" {
		t.Errorf("last_name = %q, want Greven", nameOrNull(last))
	}
}

func TestASignedNameLeavesANameAColleagueTypedAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const addr = "typed@greven.test"

	res, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, addr, "Greven Ebusiness", "greven.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// A colleague reads the record and says who this is.
	typedFirst, typedLast := "Marcus", "Greven-Hoffmann"
	if _, err := e.store.UpdateContact(ctx, res.ContactID, UpdateContactInput{
		FirstName: &typedFirst, LastName: &typedLast,
	}); err != nil {
		t.Fatalf("the colleague's edit: %v", err)
	}

	// A signature stating something else does not get to rule on it.
	if _, err := e.store.ApplySignatureFields(ctx, res.ContactID, e.openSignatureSource(ctx, t, res.ContactID), []SignatureField{
		{Name: FieldFirstName, Value: "Marc", Evidence: "Marc Greven", Confidence: 0.95},
		{Name: FieldLastName, Value: "Greven", Evidence: "Marc Greven", Confidence: 0.95},
	}, true); err != nil {
		t.Fatalf("applying the signed name: %v", err)
	}

	_, first, last := e.storedName(ctx, t, res.ContactID)
	if nameOrNull(first) != typedFirst || nameOrNull(last) != typedLast {
		t.Errorf("name = %q / %q, want the colleague's %q / %q — they read the record and said who this is",
			nameOrNull(first), nameOrNull(last), typedFirst, typedLast)
	}
}
