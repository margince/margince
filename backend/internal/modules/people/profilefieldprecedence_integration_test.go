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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// fillFromSignature runs the machine pass that scores what it writes, dating
// the statement now — a signature read today, which is the newest thing said
// about the field unless a case says otherwise.
func fillFromSignature(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, f SignatureField) bool {
	t.Helper()
	return fillFromSignatureObserved(ctx, t, e, personID, time.Now(), f)
}

// fillFromSignatureObserved is the same pass with the statement's own date,
// which is what the supersede rule compares.
func fillFromSignatureObserved(ctx context.Context, t *testing.T, e *dedupeEnv, personID ids.PersonID, observedAt time.Time, f SignatureField) bool {
	t.Helper()
	var verdict signatureVerdict
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		verdict, err = e.store.applySignatureField(ctx, tx, personID, "mailto:signature", observedAt, f)
		return err
	}); err != nil {
		t.Fatalf("apply the signature field %s: %v", f.Name, err)
	}
	return verdict.applied
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

// The writer's liveness predicate refuses a row and SAYS SO, which is the half
// its four callers' counters rest on.
//
// Reached the only way it can be reached deterministically: from inside the
// transaction, with the person archived between the entry gate and the write.
// That is exactly the window the predicate exists for — at READ COMMITTED each
// statement takes a fresh snapshot, so an erasure that commits after
// EnsureWritableLive is visible to the INSERT that follows it. Before the
// predicate, the acceptance path could not have been in this position at all;
// with it, a caller that ignored the answer would count a claim that is not
// there, put that number in the audit row, and publish it on the outbox.
func TestARefusedWriteReportsThatItDidNotLand(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Tore Aas", "tore@refused.test", "Aas AS", "refused.test")

	var landed bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE person SET archived_at = now() WHERE id = $1`, personID); err != nil {
			return err
		}
		var err error
		landed, err = writePersonProfileField(ctx, tx, personID, personProfileFieldRow{
			Field: "role", Value: "Owner", EvidenceSnippet: "Tore Aas owns the company.",
			SourceRef: "https://refused.test/about", Source: researchSource, CapturedBy: "human:probe",
		}, replaceOnAcceptance)
		return err
	}); err != nil {
		t.Fatalf("run the write against an archived person: %v", err)
	}
	if landed {
		t.Error("the writer reported a row landed on an archived person")
	}
	if rows := claimRowsFor(ctx, t, e, personID); rows != 0 {
		t.Errorf("rows for the archived person = %d, want 0", rows)
	}
}

// The predicate NARROWS the window; the lock closes it. Proved by making an
// erasure wait for a fill that is holding the subject.
//
// No sleep and no clock: the erasure's own lock_timeout is what reports the
// blocking, and it can only fire when something is holding the row. Without the
// lock the erasure commits immediately and the test fails on the successful
// path, so both directions are observable.
func TestAFillHoldsTheSubjectAgainstAConcurrentErasure(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	personID, _ := e.seedEmployedPerson(ctx, t,
		"Sunniva Dahl", "sunniva@holds.test", "Dahl AS", "holds.test")

	held, release := make(chan struct{}), make(chan struct{})
	filling := make(chan error, 1)
	// once, because WithWorkspaceTx may run the closure again and a second
	// close would panic in a goroutine the test cannot recover.
	var announce sync.Once
	go func() {
		filling <- e.store.tx(ctx, func(tx pgx.Tx) error {
			if _, err := writePersonProfileField(ctx, tx, personID, personProfileFieldRow{
				Field: "role", Value: "Owner", EvidenceSnippet: "Sunniva Dahl owns the company.",
				SourceRef: "https://holds.test/about", Source: researchSource, CapturedBy: "human:probe",
			}, replaceOnAcceptance); err != nil {
				return err
			}
			announce.Do(func() { close(held) })
			<-release
			return nil
		})
	}()
	<-held

	erasure := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE person SET archived_at = now() WHERE id = $1`, personID)
		return err
	})
	close(release)
	if err := <-filling; err != nil {
		t.Fatalf("the fill itself failed, so this test proves nothing about the erasure: %v", err)
	}

	var pgErr *pgconn.PgError
	if !errors.As(erasure, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("the erasure got %v, want a lock timeout (55P03) — a fill in flight must hold the "+
			"subject, or the erasure commits between this transaction's snapshot and its write and "+
			"the row lands on a person it had just cleared", erasure)
	}
}
