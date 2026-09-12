// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Writing a review of how a deal's closing went, through the real seam.
//
// The behaviours that matter, in the order they matter:
//
// The note and the frozen answers are ONE write. A review whose prose saved and
// whose answers did not would show in the timeline as an empty review, and the
// reverse would be a count claiming a review nobody can read.
//
// The questions are frozen. A template is editable, and an edit must not change
// what a review written last quarter appears to have asked.
//
// A review names ONE closing. A deal reopened and re-closed mid-form answers
// 409 rather than filing somebody's account of one outcome against another.

import (
	"errors"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// closedDeal seeds a deal and closes it as won, answering the deal and the
// closing it is on.
func closedDeal(t *testing.T, e *Env, name string) (ids.DealID, ids.UUID) {
	t.Helper()
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	deal := ids.From[ids.DealKind](e.SeedDeal(t, name, pipeline, open, &e.Rep1))
	closed, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
	if closed.ClosingOccurrenceId == nil {
		t.Fatal("the closed deal is on no closing")
	}
	return deal, ids.UUID(*closed.ClosingOccurrenceId)
}

func TestAReviewWritesItsNoteAndItsAnswersTogether(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Reviewed win")
	reviews := compose.NewOutcomeReviews(e.Pool)
	ctx := e.Admin()

	body := "They moved fast once the security review cleared."
	review, err := reviews.Write(ctx, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "Best fit on integrations"},
		Body:                &body,
	})
	if err != nil {
		t.Fatalf("writing the review: %v", err)
	}

	// The outcome comes from the CLOSING, never from the caller.
	if review.Outcome != "won" {
		t.Fatalf("review outcome = %q, want won", review.Outcome)
	}
	// The questions travelled with it, frozen.
	if len(review.Questions) == 0 {
		t.Fatal("the review froze no questions, so nothing later can explain its answers")
	}
	if review.Answers["why_we_won"] != "Best fit on integrations" {
		t.Fatalf("the answer did not survive the write: %v", review.Answers)
	}

	// And the note is a real activity, linked to the deal, carrying the prose.
	if got := e.WsCount(t,
		`SELECT count(*) FROM activity a
		   JOIN activity_link l ON l.activity_id = a.id
		  WHERE a.id = $1 AND l.deal_id = $2 AND a.kind = 'note' AND a.body = $3`,
		review.ActivityId, deal, body); got != 1 {
		t.Fatal("the review's note is missing, unlinked, or lost its prose")
	}
}

// A retry is one review, not two.
func TestRetryingASubmissionReturnsTheReviewAlreadyWritten(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Retried review")
	reviews := compose.NewOutcomeReviews(e.Pool)
	ctx := e.Admin()

	submission := ids.NewV7()
	req := crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(submission),
		Answers:             map[string]string{"why_we_won": "Price"},
	}
	first, err := reviews.Write(ctx, deal, req)
	if err != nil {
		t.Fatalf("first submission: %v", err)
	}
	second, err := reviews.Write(ctx, deal, req)
	if err != nil {
		t.Fatalf("retrying the same submission: %v", err)
	}
	if first.Id != second.Id {
		t.Fatal("a retry wrote a second review of one closing")
	}
	// And exactly one note, not two: the retry must not leave an orphan.
	if got := e.WsCount(t,
		`SELECT count(*) FROM activity_review_response WHERE deal_id = $1`, deal); got != 1 {
		t.Fatalf("the deal carries %d reviews after one submission and one retry, want 1", got)
	}
}

// The case the whole occurrence design exists for.
func TestAReviewOfAClosingTheDealHasLeftIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Re-closed mid-review", pipeline, open, &e.Rep1))
	reviews := compose.NewOutcomeReviews(e.Pool)

	first, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	staleOccurrence := ids.UUID(*first.ClosingOccurrenceId)

	// The deal reopens and closes again while somebody has the form open.
	if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{ToStageID: open}); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	}); err != nil {
		t.Fatalf("re-closing: %v", err)
	}

	_, err = reviews.Write(admin, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(staleOccurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "Written about the first close"},
	})
	var stale *deals.StaleClosingError
	if !errors.As(err, &stale) {
		t.Fatalf("reviewing a closing the deal has left → %v, want deals.StaleClosingError", err)
	}
	// It reads as a conflict, so the caller reloads rather than rewording.
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Error("a stale closing does not read as a conflict, so the caller is told to fix their input")
	}
	// Nothing was written: no half-review, no orphan note.
	if got := e.WsCount(t,
		`SELECT count(*) FROM activity_review_response WHERE deal_id = $1`, deal); got != 0 {
		t.Fatal("the refused review left a row behind")
	}
}

// An answer to a question nobody asked is refused, because the frozen questions
// beside it could not explain it.
func TestAnAnswerToAQuestionTheTemplateDoesNotAskIsRefused(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Bad answer key")
	reviews := compose.NewOutcomeReviews(e.Pool)

	_, err := reviews.Write(e.Admin(), deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"invented_question": "who knows"},
	})
	if err == nil {
		t.Fatal("an answer to no question was accepted, and nothing later could explain it")
	}
}

// A required question left blank is refused, named so the author knows which.
func TestARequiredQuestionMustBeAnswered(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Blank required answer")
	reviews := compose.NewOutcomeReviews(e.Pool)

	_, err := reviews.Write(e.Admin(), deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "   "},
	})
	if err == nil {
		t.Fatal("a blank required answer was accepted")
	}
}

// The freezing, proved by editing the template afterwards.
func TestEditingATemplateDoesNotChangeWhatAnOldReviewAsked(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Frozen questions")
	reviews := compose.NewOutcomeReviews(e.Pool)

	review, err := reviews.Write(e.Admin(), deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "Integrations"},
	})
	if err != nil {
		t.Fatal(err)
	}
	asked := review.Questions[0].Label

	// The template is reworded, as an administrator may do at any time.
	//
	// activity_review_template is a PRESERVED reference table: a reset does not
	// truncate it, because the migrations seed it and every later test expects
	// the seeded rows to be there. So this edit outlives the test unless it is
	// put back, and leaving it would starve every subsequent review test of a
	// live template — which is exactly what it did before this restore existed.
	var seeded string
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT questions::text FROM activity_review_template WHERE key = 'win_review'`).
		Scan(&seeded); err != nil {
		t.Fatalf("reading the seeded template: %v", err)
	}
	t.Cleanup(func() {
		e.WsExec(t, `UPDATE activity_review_template SET questions = $1::jsonb WHERE key = 'win_review'`, seeded)
	})
	e.WsExec(t,
		`UPDATE activity_review_template
		    SET questions = '[{"key":"why_we_won","label":"REWORDED","type":"text","required":true}]'::jsonb
		  WHERE key = 'win_review'`)

	after, err := reviews.List(e.Admin(), deal)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("the deal carries %d reviews, want 1", len(after))
	}
	if after[0].Questions[0].Label != asked {
		t.Fatalf("the old review now says it asked %q, but it asked %q",
			after[0].Questions[0].Label, asked)
	}
}

// The composite foreign key, exercised directly: a review cannot name a
// closing that happened on somebody else's deal.
func TestAReviewCannotNameAnotherDealsClosing(t *testing.T) {
	e := Setup(t)
	// Two deals on ONE fixture pipeline: DealFixture mints a pipeline per
	// call, and a second one collides with the first.
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	mine := ids.From[ids.DealKind](e.SeedDeal(t, "My deal", pipeline, open, &e.Rep1))
	theirs := ids.From[ids.DealKind](e.SeedDeal(t, "Their deal", pipeline, open, &e.Rep1))
	for _, deal := range []ids.DealID{mine, theirs} {
		if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{
			ToStageID: won, WonWithoutContractReason: WonByImport(),
		}); err != nil {
			t.Fatalf("closing: %v", err)
		}
	}
	closedTheirs, err := e.Deals.GetDeal(admin, theirs, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	theirOccurrence := ids.UUID(*closedTheirs.ClosingOccurrenceId)
	noteID := ids.NewV7()

	// A note of its own, so the activity_id uniqueness cannot be what refuses
	// this: what is under test is the COMPOSITE key, and a fixture that trips
	// a different constraint would pass while proving nothing.
	e.WsExec(t,
		`INSERT INTO activity (id, kind, source, captured_by, occurred_at)
		 VALUES ($1, 'note', 'test', 'test', now())`, noteID)

	insertErr := e.WsExecErr(t,
		`INSERT INTO activity_review_response
		     (activity_id, deal_id, closing_occurrence_id, outcome, template_key,
		      template_version, questions, answers, submission_id, source, captured_by)
		 VALUES ($1, $2, $3, 'won', 'win_review', 1,
		         '[{"key":"k","label":"l","type":"text","required":false}]'::jsonb,
		         '{}'::jsonb, gen_random_uuid(), 'test', 'test')`,
		noteID, mine, theirOccurrence)
	if insertErr == nil {
		t.Fatal("a review was filed against a closing on a different deal")
	}
	if !strings.Contains(insertErr.Error(), "occurrence_fkey") {
		t.Fatalf("refused by something other than the composite key: %v", insertErr)
	}
}

// Editing a stage's meaning does not rewrite what an old closing meant.
//
// A stage semantic is configuration and an administrator may change it. Reading
// the outcome by joining the LIVE stage would then report a deal won in March
// as lost in June, or lose its closing entirely once the deal's status and the
// stage's current meaning stop agreeing — taking the review with it.
func TestEditingAStagesMeaningDoesNotRewriteAnOldClosing(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Stage reconfigured")
	reviews := compose.NewOutcomeReviews(e.Pool)

	if _, err := reviews.Write(e.Admin(), deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "Before the reconfiguration"},
	}); err != nil {
		t.Fatalf("writing the review: %v", err)
	}

	// The stage the deal closed through is re-pointed at a different meaning.
	e.WsExec(t,
		`UPDATE stage SET semantic = 'lost', win_probability = 0
		  WHERE id = (SELECT to_stage_id FROM deal_stage_history
		               WHERE id = $1)`, occurrence)
	// Asserted, because a silent no-op here would make everything below pass
	// while proving nothing: the whole test turns on the stage having actually
	// changed under a closing that already happened.
	if got := e.WsCount(t,
		`SELECT count(*) FROM stage s
		   JOIN deal_stage_history h ON h.to_stage_id = s.id
		  WHERE h.id = $1 AND s.semantic = 'lost'`, occurrence); got != 1 {
		t.Fatal("the stage edit did not land, so this test proves nothing")
	}
	// And the deal still says it was won, which is the disagreement a reader
	// joining the live stage would fall into.
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal WHERE id = $1 AND status = 'won'`, deal); got != 1 {
		t.Fatal("the deal's own status changed with the stage, so there is no disagreement to test")
	}

	after, err := reviews.List(e.Admin(), deal)
	if err != nil {
		t.Fatalf("listing after the stage edit: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("the deal carries %d reviews after a stage edit, want 1 — the review was lost", len(after))
	}
	if after[0].Outcome != "won" {
		t.Fatalf("the old review now reports outcome %q, want won — the stage edit rewrote history",
			after[0].Outcome)
	}

	// And the deal still names the closing it is on, which is what the WRITE
	// path reads. Derived from the live stage instead, this answers nothing —
	// the stage now means lost while the deal still says won, so no row
	// matches — and the deal becomes unreviewable with nothing saying why.
	stillClosed, err := e.Deals.GetDeal(e.Admin(), deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if stillClosed.ClosingOccurrenceId == nil {
		t.Fatal("a stage edit left the deal on no closing, so it can never be reviewed again")
	}
	if *stillClosed.ClosingOccurrenceId != openapi_types.UUID(occurrence) {
		t.Fatalf("the deal now names closing %s, want the one it actually closed on",
			stillClosed.ClosingOccurrenceId)
	}
}

// Reopening a deal does not hide the review of the closing it already had.
//
// The review is still the record of what somebody thought when the deal closed.
// Hiding it because the deal happens to be open again would make a reopen look
// like an erasure.
func TestReopeningADealKeepsTheReviewOfItsEarlierClosing(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Reopened after review", pipeline, open, &e.Rep1))
	reviews := compose.NewOutcomeReviews(e.Pool)

	closed, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviews.Write(admin, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: *closed.ClosingOccurrenceId,
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "Then it reopened"},
	}); err != nil {
		t.Fatalf("writing the review: %v", err)
	}

	if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{ToStageID: open}); err != nil {
		t.Fatalf("reopening: %v", err)
	}

	after, err := reviews.List(admin, deal)
	if err != nil {
		t.Fatalf("listing after the reopen: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("the reopened deal reports %d reviews, want 1 — a reopen is not an erasure", len(after))
	}
}

// dealReaderNoNotesPerms may read deals and may not write activities — the
// seat that separates "can open the record" from "may file an account of it".
func dealReaderNoNotesPerms() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
	}
}

// Reading a deal is not authority to file a review of it.
//
// A review is a note somebody wrote, so what it costs is the authority to
// write a note — not the authority to read the deal it is about. deal is an
// identity table: every seat that may read deals reads every one, so a gate
// resting on the deal alone would let any seat in the workspace file an
// account of any closing, signed with their name and counted as that deal's
// review.
func TestReadingADealIsNotAuthorityToReviewIt(t *testing.T) {
	e := Setup(t)
	deal, occurrence := closedDeal(t, e, "Read but not reviewable")
	reviews := compose.NewOutcomeReviews(e.Pool)

	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, dealReaderNoNotesPerms())
	_, err := reviews.Write(reader, deal, crmcontracts.CreateOutcomeReviewRequest{
		ClosingOccurrenceId: openapi_types.UUID(occurrence),
		SubmissionId:        openapi_types.UUID(ids.NewV7()),
		Answers:             map[string]string{"why_we_won": "I only read it"},
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a seat that may only READ deals filed a review: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_review_response WHERE deal_id = $1`, deal); got != 0 {
		t.Fatalf("%d review(s) landed from a seat with no authority to write one", got)
	}
}
