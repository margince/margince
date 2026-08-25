// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The precedence rule of person_profile_field, over a real Postgres, in BOTH
// directions.
//
// One direction was already held: a second acceptance replaces the first
// (researchclaim_integration_test.go). Held alone it reads green over the
// inverse defect — a machine fill that also replaced would satisfy every
// assertion there, and the pass that overwrote a human's correction would run
// again the next night and overwrite it again. So the case that matters is the
// one this file plants: a fill that arrives AFTER a human has answered.
//
// The signature pass is the fill used for the confidence half, because it is
// the only writer that scores what it stores. A human's acceptance has no
// model score, and a replacement that kept the old number would leave the row
// saying a person's decision had been measured by a model that never saw it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// storedConfidence reads the column readStoredClaim leaves out, as the pointer
// the table allows: NULL means unscored, which is what a human's answer is.
func storedConfidence(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, field string) *float64 {
	t.Helper()
	var got *float64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT confidence FROM person_profile_field WHERE person_id = $1 AND field = $2`,
			personID, field).Scan(&got)
	}); err != nil {
		t.Fatalf("read back the %s confidence: %v", field, err)
	}
	return got
}

// fillFromSignature runs the machine pass that scores what it writes.
func fillFromSignature(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, f SignatureField) bool {
	t.Helper()
	var applied bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		applied, err = e.store.applySignatureField(ctx, tx, personID, "mailto:signature", f)
		return err
	}); err != nil {
		t.Fatalf("apply the signature field %s: %v", f.Name, err)
	}
	return applied
}

// A machine read a page or a footer; the human read the evidence and chose.
// The fill claims an unanswered field and defers to an answered one, whichever
// pass happens to run last.
func TestAMachineFillNeverReplacesWhatAHumanAccepted(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Ola Brekke", "ola@precedence.test", "Brekke AS", "precedence.test")

	accepted := ResearchClaimInput{
		Field:     "linkedin",
		Value:     "https://www.linkedin.com/in/ola-brekke",
		Quote:     "Ola Brekke — Brekke AS, Oslo.",
		SourceURL: "https://precedence.test/team",
	}
	if _, err := e.store.SaveResearchClaims(ctx, personID, []ResearchClaimInput{accepted}); err != nil {
		t.Fatalf("accept the claim: %v", err)
	}

	applied, err := e.store.ApplyDiscoveredFields(ctx, personID, []DiscoveredField{{
		Field:           "linkedin",
		Value:           "https://www.linkedin.com/in/someone-else",
		EvidenceSnippet: "Someone Else — Brekke AS",
		SourceRef:       "search:precedence",
	}})
	if err != nil {
		t.Fatalf("ApplyDiscoveredFields: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("the search fill reported %v applied, want nothing: the field was already answered", applied)
	}

	got := readStoredClaim(ctx, t, e, personID, "linkedin")
	if got.value != accepted.Value || got.source != researchSource {
		t.Errorf("stored row = %+v, want the human's value under %s — a machine fill overwrote a decision", got, researchSource)
	}
	// The trigger owns the bump, so a version past 1 means the row was updated
	// even where the value happened to survive.
	if got.version != 1 {
		t.Errorf("version = %d, want 1: the fill wrote the row rather than deferring to it", got.version)
	}
	if rows := claimRowsFor(ctx, t, e, personID); rows != 1 {
		t.Errorf("rows under this person = %d, want 1 — the fill landed a second row under a key the reader cannot see", rows)
	}
}

// The other direction, and the column the replacement used to leave behind: a
// human's acceptance takes the whole row, score included.
func TestAnAcceptanceReplacesAMachineFillAndItsScore(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Mira Halvorsen", "mira@precedence.test", "Halvorsen AS", "precedence.test")

	if !fillFromSignature(ctx, t, e, personID, SignatureField{
		Name:       "role",
		Value:      "Sales Engineer",
		Evidence:   "Mira Halvorsen | Sales Engineer | Halvorsen AS",
		Confidence: 0.9,
	}) {
		t.Fatal("the signature fill wrote nothing, so this test proves nothing about replacing it")
	}
	if score := storedConfidence(ctx, t, e, personID, "role"); score == nil || *score != 0.9 {
		t.Fatalf("the signature fill stored confidence %v, want 0.9 — the setup is not the state this test replaces", score)
	}

	accepted := ResearchClaimInput{
		Field:     "role",
		Value:     "Head of Sales",
		Quote:     "Mira Halvorsen now heads sales at Halvorsen AS.",
		SourceURL: "https://precedence.test/news",
	}
	if _, err := e.store.SaveResearchClaims(ctx, personID, []ResearchClaimInput{accepted}); err != nil {
		t.Fatalf("accept the claim: %v", err)
	}

	got := readStoredClaim(ctx, t, e, personID, "role")
	if got.value != accepted.Value || got.source != researchSource || got.capturedBy != "human:"+e.rep.String() {
		t.Errorf("stored row = %+v, want the accepted claim under %s, captured by the human who chose it", got, researchSource)
	}
	if score := storedConfidence(ctx, t, e, personID, "role"); score != nil {
		t.Errorf("confidence = %v after a human's acceptance, want NULL: the row would otherwise say a "+
			"person's decision had been scored by a model that never saw it", *score)
	}
}
