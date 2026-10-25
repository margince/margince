// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A card a model read off a photo is never written, whichever entry point it
// arrives through. It is reported for approval or skipped. The contact list
// stays as it was until a human approves.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func photoCard(name string, emails ...string) VCardEntry {
	entry := VCardEntry{FullName: name, Company: "Photo Works GmbH", Title: "Head of Photos", ModelRead: true}
	for _, email := range emails {
		entry.Emails = append(entry.Emails, VCardChannel{Value: email, Kind: "work"})
	}
	return entry
}

func TestAModelReadCardNobodyMatchesIsProposedAndCreatesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	entries := []VCardEntry{photoCard("Petra Photo", "petra@photo-works.example")}

	entryPoints := map[string]func() ([]VCardResult, error){
		"ImportVCards":          func() ([]VCardResult, error) { return e.store.ImportVCards(ctx, entries) },
		"ProposeCardsFromPhoto": func() ([]VCardResult, error) { return e.store.ProposeCardsFromPhoto(ctx, entries) },
	}
	for name, run := range entryPoints {
		results, err := run()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(results) != 1 || results[0].Outcome != VCardProposed || results[0].ContactID != nil {
			t.Fatalf("%s: results = %+v, want one proposed card naming no contact", name, results)
		}
		if got := countContactsNamed(ctx, t, e, "Petra Photo"); got != 0 {
			t.Fatalf("%s: %d contacts named Petra Photo exist, want none before an approval", name, got)
		}
	}
}

func TestAModelReadCardWithAKnownAddressIsSkippedAndLeavesTheContactAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	title := "Chief Financial Officer"
	incumbent, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Dana Known", Source: "manual", Title: &title,
		Emails: []ContactEmailInput{{Email: "dana@known.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the incumbent: %v", err)
	}

	results, err := e.store.ProposeCardsFromPhoto(ctx, []VCardEntry{photoCard("Dana Known", "dana@known.example")})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != VCardSkipped || results[0].Reason != vcardExistsReason {
		t.Fatalf("results = %+v, want one skipped card saying the contact exists", results)
	}
	if results[0].ContactID == nil || results[0].ContactID.UUID != ids.UUID(incumbent.Id) {
		t.Errorf("the skipped card names %v, want the contact the caller can read", results[0].ContactID)
	}
	if got := contactTitleOf(ctx, t, e, ids.UUID(incumbent.Id)); got != title {
		t.Errorf("title = %q, want %q: a photo must not overwrite a record", got, title)
	}
}

func TestAModelReadCardThatResemblesSomebodyIsLeftForAHumanWithTheCandidate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Chris Ambiguous", Source: "manual",
		Emails: []ContactEmailInput{{Email: "chris@first.example", EmailType: emailTypeWork, IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := e.store.ProposeCardsFromPhoto(ctx, []VCardEntry{photoCard("Chris Ambiguous", "chris@second.example")})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != VCardNeedsReview || !results[0].Outcome.AwaitsApproval() {
		t.Fatalf("results = %+v, want one needing review", results)
	}
	if results[0].ContactID == nil || results[0].ContactID.UUID != ids.UUID(incumbent.Id) {
		t.Errorf("candidate = %v, want the resembled contact", results[0].ContactID)
	}
	if got := countContactsNamed(ctx, t, e, "Chris Ambiguous"); got != 1 {
		t.Errorf("contacts named Chris Ambiguous = %d, want the 1 that existed", got)
	}
}

func TestProposeCardsFromPhotoRefusesACardNoModelRead(t *testing.T) {
	e := setupDedupe(t)
	typed := photoCard("Tina Typed")
	typed.ModelRead = false
	if _, err := e.store.ProposeCardsFromPhoto(e.as(), []VCardEntry{typed}); err == nil {
		t.Fatal("an unmarked card was accepted by the photo entry point")
	}
}

func TestAPhotoYieldingMoreThanTheCapIsRefusedWhole(t *testing.T) {
	e := setupDedupe(t)
	entries := make([]VCardEntry, CardPhotoMaxCards+1)
	for i := range entries {
		entries[i] = photoCard("Card Holder")
	}
	if _, err := e.store.ProposeCardsFromPhoto(e.as(), entries); err == nil {
		t.Fatal("a photo above the cap was accepted")
	}
}

func contactTitleOf(ctx context.Context, t *testing.T, e *dedupeEnv, id ids.UUID) string {
	t.Helper()
	var title *string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT title FROM contact WHERE id = $1`, id).Scan(&title)
	}); err != nil {
		t.Fatalf("reading the title: %v", err)
	}
	if title == nil {
		return ""
	}
	return *title
}
