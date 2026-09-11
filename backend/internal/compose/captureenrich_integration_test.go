// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The signature-enrich pass over a real Postgres: an evidence-grounded
// title and phone land with their PO-DDL-12 evidence rows; a fabricated
// snippet is dropped by the code-side gate; a human's correction is never
// overwritten; a person once enriched leaves the candidate set; and a pass
// that filled its limit says so, so the worker can queue the next slice
// instead of leaving it until tonight.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// signatureScriptBrain answers every call with a fixed field set.
type signatureScriptBrain struct {
	fields []map[string]any
	calls  int
}

func (s *signatureScriptBrain) Complete(context.Context, model.Request) (model.Response, error) {
	s.calls++
	payload, err := json.Marshal(map[string]any{"fields": s.fields})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

// seedEnrichPerson plants one connector-created person with a linked
// inbound email whose body carries the signature.
func seedEnrichPerson(t *testing.T, e *integration.Env, email, body string) ids.UUID {
	t.Helper()
	person := ids.NewV7()
	activity := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO person (id, full_name, source, captured_by)
			VALUES ($1, 'Bob Person', 'gmail:seed', 'connector:gmail')`, person); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_email (person_id, email, email_type, is_primary, source, captured_by)
			VALUES ($1, $2, 'work', true, 'gmail:seed', 'connector:gmail')`, person, email); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'hello', $2, 'inbound', 'gmail', $3, 'gmail:seed', 'connector:gmail')`,
			activity, body, activity.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, activity, person); err != nil {
			return err
		}
		// The sender, as capture stamps one. Without it the mail reaches this
		// person without anybody having written it, which is the shape the
		// candidate query now refuses.
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, person_id, address, role)
			VALUES ($1, $2, $3, 'from')`, activity, person, email)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return person
}

func TestSignatureEnrichPass(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\n+49 30 1234567\nAcme GmbH"
	person := seedEnrichPerson(t, e, "bob@acme.example", body)

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "CTO", "evidence_snippet": "CTO", "confidence": 0.9},
		{"field": "phone", "value": "+49 30 1234567", "evidence_snippet": "+49 30 1234567", "confidence": 0.85},
		// Fabricated: the snippet is nowhere in the signature — the gate
		// must drop it in code, whatever the model claims.
		{"field": "linkedin", "value": "linkedin.com/in/bob", "evidence_snippet": "linkedin.com/in/bob", "confidence": 0.9},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var title *string
	var phones, evidence, linkedinRows int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `SELECT title FROM person WHERE id = $1`, person).Scan(&title); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM person_phone WHERE person_id = $1`, person).Scan(&phones); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM person_profile_field WHERE person_id = $1`, person).Scan(&evidence); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM person_profile_field WHERE person_id = $1 AND field = 'linkedin'`, person).Scan(&linkedinRows)
	})
	if err != nil {
		t.Fatal(err)
	}
	if title == nil || *title != "CTO" {
		t.Fatalf("title = %v, want the evidence-grounded CTO", title)
	}
	if phones != 1 {
		t.Fatalf("%d phone rows, want the one signature phone", phones)
	}
	if evidence != 2 {
		t.Fatalf("%d evidence rows, want 2 (title + phone; the fabricated linkedin dropped)", evidence)
	}
	if linkedinRows != 0 {
		t.Fatal("a fabricated snippet must never produce an evidence row")
	}

	t.Run("the same mail is never read twice", func(t *testing.T) {
		// The read cursor, not the field set, is what retires a person: this
		// person still has no company_name evidence, so the field predicate would
		// select them again — and asking would show the model the identical
		// window and get the identical answer, nightly, forever.
		before := brain.calls
		if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if brain.calls != before {
			t.Fatal("a person whose latest mail was already read must not be re-asked")
		}
	})

	t.Run("newer mail reopens the person", func(t *testing.T) {
		newer := ids.NewV7()
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			ctx := context.Background()
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source_system, source_id, source, captured_by)
				VALUES ($1, 'email', 'again', $2, 'inbound', now() + interval '1 hour', 'gmail', $3, 'gmail:seed', 'connector:gmail')`,
				newer, "Hi again,\n\nBob Person\nCTO\nAcme Holding GmbH", newer.String()); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_link (activity_id, entity_type, person_id)
				VALUES ($1, 'person', $2)`, newer, person); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO activity_participant (activity_id, person_id, address, role)
				VALUES ($1, $2, 'bob@acme.example', 'from')`, newer, person)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		before := brain.calls
		if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if brain.calls == before {
			t.Fatal("a person who has written again must be read again — the new signature may state what the old one did not")
		}
	})

	t.Run("an occupied title is never touched", func(t *testing.T) {
		occupied := seedEnrichPerson(t, e, "carol@acme.example",
			"Cheers,\nCarol\nVP Sales\n+49 30 7654321")
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(),
				`UPDATE person SET title = 'Handwritten Title' WHERE id = $1`, occupied)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		// She is still a candidate — the company_name her signature may state is
		// unanswered — so the pass reads her mail and the model returns the
		// same title it returns for everyone. The human's answer survives it.
		if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
			t.Fatalf("Run: %v", err)
		}
		var title string
		err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT title FROM person WHERE id = $1`, occupied).Scan(&title)
		})
		if err != nil {
			t.Fatal(err)
		}
		if title != "Handwritten Title" {
			t.Fatalf("title = %q — the human's answer was touched", title)
		}
	})
}

// faultyEnrichBrain answers every call with a fixed error or garbage —
// the model failure modes the pass must absorb without losing the fleet.
type faultyEnrichBrain struct {
	err     error
	garbage bool
	calls   int
}

func (f *faultyEnrichBrain) Complete(context.Context, model.Request) (model.Response, error) {
	f.calls++
	if f.err != nil {
		return model.Response{}, f.err
	}
	if f.garbage {
		return model.Response{Text: "not json at all {{{"}, nil
	}
	return model.Response{Text: "{}"}, nil
}

func TestSignatureEnrichAbsorbsModelFailures(t *testing.T) {
	e := integration.Setup(t)
	seedEnrichPerson(t, e, "flaky@acme.example", "Thanks,\nFlaky Person\nCOO\n+49 30 1111111")

	t.Run("garbage output fails the candidate, not the pass", func(t *testing.T) {
		brain := &faultyEnrichBrain{garbage: true}
		enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
		if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
			t.Fatalf("a per-candidate model failure must not fail the pass: %v", err)
		}
		if brain.calls == 0 {
			t.Fatal("the candidate was never asked")
		}
		// Nothing landed: no evidence row for a verdict nobody could parse.
		if n := enrichEvidenceCount(t, e, "flaky@acme.example"); n != 0 {
			t.Fatalf("%d evidence rows from a garbage verdict, want 0", n)
		}
	})

	t.Run("a budget stop ends the pass cleanly", func(t *testing.T) {
		brain := &faultyEnrichBrain{err: ai.ErrBudgetDeferred}
		enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
		if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
			t.Fatalf("a budget stop must not be an error: %v", err)
		}
		if brain.calls != 1 {
			t.Fatalf("model calls = %d, want 1 — the stop must end the pass, not walk the fleet", brain.calls)
		}
	})
}

// enrichEvidenceCount counts the person's evidence rows by primary email.
func enrichEvidenceCount(t *testing.T, e *integration.Env, email string) int {
	t.Helper()
	var n int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM person_profile_field f
			JOIN person_email pe ON pe.person_id = f.person_id
			WHERE pe.email = $1`, email).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A human who corrected a field is promised no fresh inference replaces their
// answer without a confirm. This is the case that promise is made of: they
// correct the title, the contact then writes again stating something else, and
// the pass must leave the correction alone.
//
// Recorded through ai.FeedbackStore, the writer the page itself uses, because
// the ledger stores a HASH of the claim path — a fixture that inserted the row
// by hand could spell the key any way at all and would prove nothing about
// whether the pass can find a real one.
func TestASignatureDoesNotOverwriteACorrectedField(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\n+49 30 1234567\nAcme GmbH"
	person := seedEnrichPerson(t, e, "corrected@acme.example", body)

	ctx := e.Admin()
	if err := ai.NewFeedbackStore(InstallationDB(e.Pool)).Record(ctx, ai.RecordInput{
		SubjectType: "person",
		SubjectID:   person,
		ClaimKind:   ai.ClaimProfileField,
		ClaimPath:   ai.ProfileFieldClaimPath("title"),
		Verdict:     ai.VerdictCorrected,
		CorrectedValue: func() *string {
			v := "Head of Engineering"
			return &v
		}(),
	}); err != nil {
		t.Fatalf("record the human's correction: %v", err)
	}

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "CTO", "evidence_snippet": "CTO", "confidence": 0.9},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var titleRows int
	var title *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		c := context.Background()
		if err := tx.QueryRow(c, `SELECT title FROM person WHERE id = $1`, person).Scan(&title); err != nil {
			return err
		}
		return tx.QueryRow(c,
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1 AND field = 'title'`,
			person).Scan(&titleRows)
	}); err != nil {
		t.Fatal(err)
	}
	if title != nil {
		t.Errorf("person.title = %q — a corrected field was overwritten by a fresh inference", *title)
	}
	if titleRows != 0 {
		t.Errorf("%d title evidence rows, want 0: the pass wrote over a human's ruling", titleRows)
	}
}

// TestAFullPassAsksForAContinuation drives the boundary the continuation turns
// on: a pass whose candidate set filled the limit reports that more people are
// due, and one that came back short does not.
//
// The limit is set to 2 rather than seeding a hundred people. What is under
// test is the comparison against the pass's own limit, and that comparison is
// the same one at 2 as at 100.
func TestAFullPassAsksForAContinuation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		people     int
		wantFilled bool
	}{
		// One short of the limit: nobody is waiting, and a continuation would
		// be a job that lists no candidates.
		{"a short pass is finished", 1, false},
		// Exactly the limit. There may be a hundred more behind these two, and
		// before the continuation they waited for the nightly pass.
		{"a full pass has more to do", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\nAcme GmbH"
			for i := range tc.people {
				seedEnrichPerson(t, e, fmt.Sprintf("full%d@acme.example", i), body)
			}

			brain := &signatureScriptBrain{fields: []map[string]any{
				{"field": "title", "value": "CTO", "evidence_snippet": "CTO", "confidence": 0.9},
			}}
			enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
			enricher.limit = 2

			filled, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if filled != tc.wantFilled {
				t.Errorf("filled = %v, want %v for %d candidates against a limit of 2",
					filled, tc.wantFilled, tc.people)
			}
		})
	}
}

// A budget stop reports no continuation. The pass stopped because the model
// budget ran out, so a follow-on would hit the same wall at once — and the
// candidate set was full, which is the signal that would otherwise fire.
func TestABudgetStopAsksForNoContinuation(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\nAcme GmbH"
	for i := range 2 {
		seedEnrichPerson(t, e, fmt.Sprintf("budget%d@acme.example", i), body)
	}

	enricher := NewCaptureEnricher(e.Pool, &budgetStoppedBrain{}, slog.New(slog.DiscardHandler))
	enricher.limit = 2

	filled, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if filled {
		t.Error("a budget-stopped pass asked for a continuation, which would spend a job to hit the same wall")
	}
}

// budgetStoppedBrain is the model lane refusing on budget, which is the one
// error RunWorkspace treats as ending the pass rather than skipping a person.
type budgetStoppedBrain struct{}

func (b *budgetStoppedBrain) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, ai.ErrBudgetDeferred
}

// TestAPassThatMovedNobodyAsksForNoContinuation is the runaway bound.
//
// A candidate whose model call fails writes no watermark, deliberately, so it
// stays the newest mail and fills the next pass too. If a full set alone
// queued a continuation, a hundred permanently failing candidates would
// re-select themselves and enqueue another job forever — every job succeeding,
// every one spending the model budget, and River's attempt cap unable to see
// it because nothing ever fails.
func TestAPassThatMovedNobodyAsksForNoContinuation(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\nAcme GmbH"
	for i := range 2 {
		seedEnrichPerson(t, e, fmt.Sprintf("stuck%d@acme.example", i), body)
	}

	// Output the pass cannot parse: the candidate fails, is logged, and keeps
	// its place at the head of the queue.
	enricher := NewCaptureEnricher(e.Pool, &unparseableBrain{}, slog.New(slog.DiscardHandler))
	enricher.limit = 2

	filled, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if filled {
		t.Error("a pass that enriched nobody asked for a continuation — " +
			"the same candidates would fill the next pass and chain forever")
	}
}

// unparseableBrain answers with text the verdict parser rejects, which fails
// the candidate without failing the pass.
type unparseableBrain struct{}

func (b *unparseableBrain) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: "not json"}, nil
}

// TWO PASSES DO NOT PAY FOR THE SAME PAGE.
//
// The candidate list is a plain read, and three doors can put a pass in flight
// — the nightly tick, the arrival trigger and a pass's own continuation. The
// first two dedupe on the queue's uniqueness window; the continuation is
// enqueued from inside a running job with the same args and deliberately
// cannot. Without mutual exclusion here, the second pass selects the same
// people, makes the same model calls, and each one spends for them.
//
// The holder is staged as a REAL lock on its own connection rather than as a
// second goroutine racing this one: what is under test is that a pass finding
// the lock held stands down, and a race would prove it on the runs where it
// happened to lose.
func TestAPassStandsDownWhileAnotherHoldsTheInstallation(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\nAcme GmbH"
	seedEnrichPerson(t, e, "contended@acme.example", body)

	holder, err := e.Pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("taking a connection for the holding pass: %v", err)
	}
	defer holder.Release()
	var held bool
	if err := holder.QueryRow(context.Background(),
		`SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, enrichPassLock).Scan(&held); err != nil {
		t.Fatalf("holding the pass lock: %v", err)
	}
	if !held {
		t.Fatal("the fixture could not take the pass lock, so nothing below proves a second pass stands down")
	}
	defer func() {
		if _, err := holder.Exec(context.Background(),
			`SELECT pg_advisory_unlock(hashtextextended($1, 0))`, enrichPassLock); err != nil {
			t.Errorf("releasing the pass lock: %v", err)
		}
	}()

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "CTO", "evidence_snippet": "CTO", "confidence": 0.9},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	filled, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS))
	if err != nil {
		t.Fatalf("the second pass returned an error, and standing down is not one: %v", err)
	}
	if filled {
		t.Error("a pass that did no work asked for a continuation")
	}
	// The money assertion. A candidate is sitting there — the admission case
	// below proves the same fixture is enrichable — and this pass spent nothing
	// on it.
	if brain.calls != 0 {
		t.Errorf("the second pass made %d model call(s) against a page another pass is reading", brain.calls)
	}
}

// And it is the LOCK that stood the pass down, not an empty fixture: the same
// seed, with nobody holding, enriches.
func TestThePassRunsWhenNobodyHoldsTheInstallation(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\nAcme GmbH"
	seedEnrichPerson(t, e, "uncontended@acme.example", body)

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "CTO", "evidence_snippet": "CTO", "confidence": 0.9},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if brain.calls != 1 {
		t.Errorf("the pass made %d model call(s) with nobody holding the lock, want 1", brain.calls)
	}
}

// The lock does not outlive the pass. A pass that took it and finished must
// leave the next one able to run — the failure would be an installation that
// enriches once and then goes quiet until the worker restarts.
func TestThePassReleasesWhatItHeld(t *testing.T) {
	e := integration.Setup(t)
	body := "Hi,\n\nsounds good.\n\nBest,\nBob Person\nCTO\nAcme GmbH"
	seedEnrichPerson(t, e, "first@acme.example", body)

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "CTO", "evidence_snippet": "CTO", "confidence": 0.9},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, err := enricher.RunWorkspace(ctx); err != nil {
		t.Fatalf("the first pass: %v", err)
	}

	// A second person, and a second pass. It can only reach them if the first
	// pass gave the lock back.
	seedEnrichPerson(t, e, "second@acme.example", body)
	before := brain.calls
	if _, err := enricher.RunWorkspace(ctx); err != nil {
		t.Fatalf("the second pass: %v", err)
	}
	if brain.calls == before {
		t.Error("the second pass read nobody: the first pass never released the lock, so this " +
			"installation would enrich once and go quiet until the worker restarted")
	}
}

// seedForeignSignature seeds a mail this person RECEIVED: linked to them, with
// somebody else as its sender, and that sender's signature at the foot. It is
// the exact shape that put a partner manager's title, employer and street onto
// a consultant's profile — the mail was legitimately linked to her, and every
// line of the block was verbatim in it.
func seedForeignSignature(t *testing.T, e *integration.Env) ids.UUID {
	// One fixture, no dials. Every case here is the same shape — this recipient
	// wrongly inheriting this sender's signature block — and what varies
	// between them is what the pass is asked to do with it, not the mail.
	const (
		recipient = "judith@example.test"
		sender    = "marcus@other.test"
		body      = foreignSignatureBody
	)
	t.Helper()
	person := ids.NewV7()
	activity := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO person (id, full_name, source, captured_by)
			VALUES ($1, 'Judith Recipient', 'gmail:seed', 'connector:gmail')`, person); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO person_email (person_id, email, email_type, is_primary, source, captured_by)
			VALUES ($1, $2, 'work', true, 'gmail:seed', 'connector:gmail')`, person, recipient); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'speaker slot', $2, 'inbound', 'gmail', $3, 'gmail:seed', 'connector:gmail')`,
			activity, body, activity.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, activity, person); err != nil {
			return err
		}
		// Somebody else wrote it. She is on it as a recipient, which is why the
		// link exists at all.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, address, role)
			VALUES ($1, $2, 'from')`, activity, sender); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, person_id, address, role)
			VALUES ($1, $2, $3, 'to')`, activity, person, recipient)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return person
}

// The signature block of a mail somebody else sent, with every field a
// signature normally yields.
const foreignSignatureBody = "Hallo Judith,\n\ngerne.\n\nViele Grüße\nMarcus Sender\n" +
	"PARTNER MANAGER DACH\nOther Company GmbH\nSomestreet 7"

func TestASignatureIsNotReadOffAMessageThePersonDidNotSend(t *testing.T) {
	e := integration.Setup(t)
	person := seedForeignSignature(t, e)

	// The model would happily extract all three, and every snippet is verbatim
	// in the window — so the evidence gate alone cannot refuse them.
	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "PARTNER MANAGER DACH", "evidence_snippet": "PARTNER MANAGER DACH", "confidence": 1},
		{"field": "company_name", "value": "Other Company GmbH", "evidence_snippet": "Other Company GmbH", "confidence": 1},
		{"field": "address", "value": "Somestreet 7", "evidence_snippet": "Somestreet 7", "confidence": 1},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var fields int
	var title *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, person).Scan(&fields); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT title FROM person WHERE id = $1`, person).Scan(&title)
	}); err != nil {
		t.Fatal(err)
	}
	if fields != 0 {
		t.Errorf("%d profile fields written off another sender's signature, want 0", fields)
	}
	if title != nil {
		t.Errorf("title = %q, want none: that title belongs to the person who WROTE the mail", *title)
	}
	if brain.calls != 0 {
		t.Errorf("the model was asked %d times about a message this person did not send, want 0", brain.calls)
	}
}

func TestABackfilledSenderRowIsReadByItsAddressNotItsPersonID(t *testing.T) {
	e := integration.Setup(t)
	person := seedForeignSignature(t, e)

	// The participant backfill takes person_id from the FIRST activity_link and
	// address from activity.counterparty_email, so a historical row can name the
	// recipient while carrying the sender's address. A predicate that trusted
	// person_id would admit exactly the mail this test refuses.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity_participant SET person_id = $1
			 WHERE role = 'from' AND address = 'marcus@other.test'`, person)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "PARTNER MANAGER DACH", "evidence_snippet": "PARTNER MANAGER DACH", "confidence": 1},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var fields int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, person).Scan(&fields)
	}); err != nil {
		t.Fatal(err)
	}
	if fields != 0 {
		t.Errorf("%d fields written, want 0: the row carries the sender's ADDRESS, and that is what decides", fields)
	}
}

func TestAForeignSignatureUnderTheirOwnIsNotReadAsTheirs(t *testing.T) {
	e := integration.Setup(t)
	// Their own message — they really did send it — but the block at the foot
	// is a forwarded colleague's. The sender predicate admits the mail; only
	// the name check can refuse the block.
	body := "Hi,\n\nforwarding this on.\n\nBest,\nSomebody Else\nHEAD OF NOTHING\nElsewhere GmbH"
	person := seedEnrichPerson(t, e, "bob@acme.example", body)

	brain := &signatureScriptBrain{fields: []map[string]any{
		{"field": "title", "value": "HEAD OF NOTHING", "evidence_snippet": "HEAD OF NOTHING", "confidence": 1},
	}}
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var fields int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, person).Scan(&fields)
	}); err != nil {
		t.Fatal(err)
	}
	if fields != 0 {
		t.Errorf("%d fields written off a block naming somebody else, want 0", fields)
	}
	if brain.calls != 0 {
		t.Errorf("the model was asked %d times about a block naming somebody else, want 0", brain.calls)
	}
}

func TestFieldsWrittenOffAnotherSendersSignatureAreTakenBack(t *testing.T) {
	e := integration.Setup(t)
	person := seedForeignSignature(t, e)

	// What the pass wrote before it knew to ask who sent the mail. Written
	// through the real applier, so the row carries the source and source_ref
	// production writes and the retraction has to match.
	var activity ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT activity_id FROM activity_link WHERE person_id = $1`, person).Scan(&activity)
	}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, err := e.People.ApplySignatureFields(e.Admin(), ids.From[ids.PersonKind](person), activity,
		[]people.SignatureField{{
			Name: "title", Value: "PARTNER MANAGER DACH",
			Evidence: "PARTNER MANAGER DACH", Confidence: 1,
			ClaimKey: ai.ClaimKey(ai.ProfileFieldClaimPath("title")),
		}}); err != nil {
		t.Fatalf("seeding the wrong field through the real applier: %v", err)
	}

	enricher := NewCaptureEnricher(e.Pool, &signatureScriptBrain{}, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var fields int
	var title *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		c := context.Background()
		if err := tx.QueryRow(c,
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, person).Scan(&fields); err != nil {
			return err
		}
		return tx.QueryRow(c, `SELECT title FROM person WHERE id = $1`, person).Scan(&title)
	}); err != nil {
		t.Fatal(err)
	}
	if fields != 0 {
		t.Errorf("%d misattributed profile fields survived the pass, want 0", fields)
	}
	if title != nil {
		t.Errorf("title = %q, want it taken back off the person it never belonged to", *title)
	}

	// The write shape. Taking a field back is the same size of change to the
	// same record as writing it, and a repair that left no trace would make a
	// title move on its own between two readings of the history.
	var audits, events int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		c := context.Background()
		if err := tx.QueryRow(c, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'person' AND entity_id = $1
			   AND evidence->>'source' = 'capture_enrich'`, person).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(c, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->'entity'->>'id' = $1::text`, person).Scan(&events)
	}); err != nil {
		t.Fatal(err)
	}
	if audits == 0 {
		t.Error("the retraction wrote no audit row — the field changed with nothing recording it")
	}
	if events == 0 {
		t.Error("the retraction emitted no event — a reader's cached 360 keeps the wrong title")
	}
}

func TestARetractionLeavesATitleSomebodyChangedAfterwards(t *testing.T) {
	e := integration.Setup(t)
	person := seedForeignSignature(t, e)

	var activity ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT activity_id FROM activity_link WHERE person_id = $1`, person).Scan(&activity)
	}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, err := e.People.ApplySignatureFields(e.Admin(), ids.From[ids.PersonKind](person), activity,
		[]people.SignatureField{{
			Name: "title", Value: "PARTNER MANAGER DACH",
			Evidence: "PARTNER MANAGER DACH", Confidence: 1,
			ClaimKey: ai.ClaimKey(ai.ProfileFieldClaimPath("title")),
		}}); err != nil {
		t.Fatalf("seeding the wrong field: %v", err)
	}

	// A human sees the wrong title and types the right one. UpdatePerson writes
	// the COLUMN and leaves the sidecar row alone, so the row still says the
	// machine's value while the page shows theirs.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE person SET title = 'Geschäftsführerin' WHERE id = $1`, person)
		return err
	}); err != nil {
		t.Fatalf("the human's correction: %v", err)
	}

	enricher := NewCaptureEnricher(e.Pool, &signatureScriptBrain{}, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var title *string
	var fields int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		c := context.Background()
		if err := tx.QueryRow(c, `SELECT title FROM person WHERE id = $1`, person).Scan(&title); err != nil {
			return err
		}
		return tx.QueryRow(c,
			`SELECT count(*) FROM person_profile_field WHERE person_id = $1`, person).Scan(&fields)
	}); err != nil {
		t.Fatal(err)
	}
	if title == nil || *title != "Geschäftsführerin" {
		t.Errorf("title = %v, want the one a human typed — the sweep restored over their answer", title)
	}
	if fields != 0 {
		t.Errorf("%d misattributed field rows survived, want 0: the stale row is still withdrawn", fields)
	}
}
