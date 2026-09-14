// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Deciding what a subject proposed.
//
// The table has carried resolution, resolved_at and resolved_by since it
// shipped and nothing ever wrote them: a subject could type a correction into
// the link we mailed them, the row was filed, and no colleague could list it,
// see it, or act on it. Every correction anybody has ever sent is still sitting
// there unanswered.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordingApplier stands in for contacts' update path and records what it was
// asked to write.
type recordingApplier struct {
	calls  int
	field  string
	value  string
	refuse error
}

func (a *recordingApplier) ApplyCorrection(
	_ context.Context, _ ids.ContactID, field, value string,
) error {
	a.calls++
	a.field, a.value = field, value
	return a.refuse
}

// proposeCorrection sends one through the real public door, so the row under
// test is the row production writes.
func proposeCorrection(t *testing.T, e *channelConsentEnv, field, value string) ids.UUID {
	t.Helper()
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)
	if _, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections: map[string]string{field: value},
	}); err != nil {
		t.Fatalf("submitting the correction: %v", err)
	}
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM contact_confirm_submission WHERE contact_id = $1 ORDER BY submitted_at DESC LIMIT 1`,
		e.contact).Scan(&id); err != nil {
		t.Fatalf("reading the staged proposal: %v", err)
	}
	return id
}

// reviewerCtx is a seat that may open and edit the contact, which is what
// deciding about a proposal asks for.
func reviewerCtx(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"contact": {Read: true, Update: true},
	}
	return principal.WithActor(e.ctx, actor)
}

// TestAProposalWaitsInTheQueueUntilSomebodyDecides is the gap this closes.
func TestAProposalWaitsInTheQueueUntilSomebodyDecides(t *testing.T) {
	e := setupChannelConsent(t)
	proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	unresolved := false
	queue, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{Resolved: &unresolved})
	if err != nil {
		t.Fatalf("listing the queue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("the queue holds %d proposal(s), want 1 — a correction nobody can list is one "+
			"nobody can answer", len(queue))
	}
	got := queue[0]
	if got.Resolution != nil {
		t.Errorf("a fresh proposal reports resolution %v, want none", *got.Resolution)
	}
	if got.ProposedValue == nil || *got.ProposedValue != "Corrected Name" {
		t.Errorf("the queue shows %v, and a correction is only reviewable as a comparison",
			got.ProposedValue)
	}
}

// TestAnAcceptedCorrectionReachesTheRecordThroughTheOrdinaryPath.
//
// A writer of its own would be a second way to change a contact, answerable to
// none of the gates the first one is.
func TestAnAcceptedCorrectionReachesTheRecordThroughTheOrdinaryPath(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	out, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionAccepted})
	if err != nil {
		t.Fatalf("accepting the correction: %v", err)
	}
	if applier.calls != 1 {
		t.Fatalf("the accepted correction reached the record %d time(s), want 1", applier.calls)
	}
	if applier.field != ConfirmFieldFullName || applier.value != "Corrected Name" {
		t.Errorf("wrote %s = %q, want the field and value the subject proposed",
			applier.field, applier.value)
	}
	if out.Resolution == nil || *out.Resolution != SubmissionAccepted {
		t.Errorf("the decision was not recorded: %v", out.Resolution)
	}
	if out.ResolvedBy == nil || *out.ResolvedBy == "" {
		t.Error("the decision names nobody, and who decided is half the record")
	}
}

// TestARejectedCorrectionChangesNothingAndSaysWhy.
//
// The proposal stays on the record: the subject asked, and an export showing
// the request without its answer would be telling half of it.
func TestARejectedCorrectionChangesNothingAndSaysWhy(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	out, err := e.store.ResolveSubmission(reviewerCtx(e), id, ResolveSubmissionInput{
		Resolution: SubmissionRejected,
		Note:       "The passport we hold spells it the other way.",
	})
	if err != nil {
		t.Fatalf("rejecting the correction: %v", err)
	}
	if applier.calls != 0 {
		t.Errorf("a rejected correction still wrote to the record %d time(s)", applier.calls)
	}
	if out.Note == nil || *out.Note == "" {
		t.Error(`the rejection records no reason — "we did not change it" is the whole of what ` +
			"the record says otherwise, and the subject who asked is entitled to more")
	}
	if out.ProposedValue == nil {
		t.Error("the rejected proposal lost the subject's own words")
	}
}

// TestASecondReviewerDoesNotOverwriteTheFirstsDecision.
//
// A stale screen pressed twice is not two decisions, and the first reviewer's
// name is the one that belongs on the row.
func TestASecondReviewerDoesNotOverwriteTheFirstsDecision(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	first, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionAccepted})
	if err != nil {
		t.Fatalf("the first decision: %v", err)
	}
	second, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionRejected, Note: "Actually no."})
	if err != nil {
		t.Fatalf("the second press: %v", err)
	}
	if second.Resolution == nil || *second.Resolution != SubmissionAccepted {
		t.Errorf("the second press rewrote the decision to %v", second.Resolution)
	}
	if second.ResolvedAt == nil || first.ResolvedAt == nil || !second.ResolvedAt.Equal(*first.ResolvedAt) {
		t.Error("the second press moved the timestamp, so the record says the decision was " +
			"made later than it was")
	}
	if applier.calls != 1 {
		t.Errorf("the record was written %d time(s) for one decision", applier.calls)
	}
}

// TestAnUnwritableFieldIsRefusedRatherThanSilentlyAccepted.
//
// Email and phone live on satellite tables the ordinary update path does not
// reach. An accept that changed nothing is the one outcome nobody could act on:
// the subject would be told their correction was taken and the record would
// still disagree.
func TestAnUnwritableFieldIsRefusedRatherThanSilentlyAccepted(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{refuse: ErrUnsupportedField}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldEmail, "new@example.test")

	_, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionAccepted})
	if !errors.Is(err, ErrUnsupportedField) {
		t.Fatalf("accepting an unwritable correction answered %v, want ErrUnsupportedField", err)
	}

	// AND THE DECISION IS NOT RECORDED. A row reading "accepted" whose value
	// never reached the contact is worse than no row at all.
	var resolution *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT resolution FROM contact_confirm_submission WHERE id = $1`, id).Scan(&resolution); err != nil {
		t.Fatalf("reading the proposal: %v", err)
	}
	if resolution != nil {
		t.Errorf("the proposal reads %q after a refused accept", *resolution)
	}
}

// TestAReviewerWhoCannotEditTheContactDecidesNothing. Deciding what a proposal
// does to a record is a write to that record, even a decline.
func TestAReviewerWhoCannotEditTheContactDecidesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}}
	readOnly := principal.WithActor(e.ctx, actor)

	if _, err := e.store.ResolveSubmission(readOnly, id,
		ResolveSubmissionInput{Resolution: SubmissionRejected}); err == nil {
		t.Fatal("a reader with no update grant recorded a decision about somebody's record")
	}
}

// TestTheQueueCarriesBothHalvesOfTheComparison is Codex's finding, and the
// screen it feeds could not make a decision without it.
//
// "She says Schmidt, we hold Schmitt" is the decision; the proposal alone is
// not. And a queue spanning every contact needs the name: two contacts
// proposing the same value on the same day are indistinguishable, while
// accepting either changes a different record.
func TestTheQueueCarriesBothHalvesOfTheComparison(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET full_name = 'Anna Schmitt' WHERE id = $1`, e.contact); err != nil {
		t.Fatalf("naming the contact: %v", err)
	}
	proposeCorrection(t, e, ConfirmFieldFullName, "Anna Schmidt")

	queue, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{})
	if err != nil {
		t.Fatalf("listing the queue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("the queue holds %d proposal(s), want 1", len(queue))
	}
	got := queue[0]
	if got.ContactName != "Anna Schmitt" {
		t.Errorf("the row names %q, and a reviewer cannot tell whose record they are "+
			"changing without it", got.ContactName)
	}
	if got.CurrentValue != "Anna Schmitt" {
		t.Errorf("the row says the record currently holds %q — without it the reviewer sees "+
			"one half of a comparison and has to go and look up the other", got.CurrentValue)
	}
}
