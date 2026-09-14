// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefevidence_test

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fakeReader stands for the activities store: it records what it was asked and
// how often, which is how the "one read per response" rule is proved. A pgx
// tracer would prove the same thing about statements and nothing about whether
// the collector deduped, which is where the defect would be.
type fakeReader struct {
	calls  int
	asked  [][]ids.UUID
	answer map[ids.UUID]crmcontracts.EmailSummary
	err    error
}

func (f *fakeReader) EmailSummariesByID(
	_ context.Context, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	f.calls++
	f.asked = append(f.asked, activityIDs)
	if f.err != nil {
		return nil, f.err
	}
	return f.answer, nil
}

func uuid(t *testing.T, last byte) openapi_types.UUID {
	t.Helper()
	var raw [16]byte
	raw[15] = last
	return openapi_types.UUID(raw)
}

func summary(t *testing.T, id openapi_types.UUID, subject string) crmcontracts.EmailSummary {
	t.Helper()
	return crmcontracts.EmailSummary{ActivityId: id, Subject: &subject}
}

func cited(kind crmcontracts.CompanyBriefEvidenceEntityType, id openapi_types.UUID) crmcontracts.CompanyBriefEvidence {
	return crmcontracts.CompanyBriefEvidence{EntityType: kind, EntityId: id}
}

const (
	activity     = crmcontracts.CompanyBriefEvidenceEntityTypeActivity
	deal         = crmcontracts.CompanyBriefEvidenceEntityTypeDeal
	contact      = crmcontracts.CompanyBriefEvidenceEntityTypeContact
	factEvidence = crmcontracts.CompanyBriefEvidenceEntityTypeFact
)

// Two sentences citing one message ask for it once, and both are filled. The
// dedupe is the whole reason this is a batch: an account page whose four
// sections rest on the same thread would otherwise read it four times.
func TestOneMessageCitedTwiceIsReadOnceAndFillsBoth(t *testing.T) {
	t.Parallel()
	id := uuid(t, 1)
	sentences := []crmcontracts.CompanyBriefSentence{
		{Text: "They asked about fallbacks.", Evidence: []crmcontracts.CompanyBriefEvidence{cited(activity, id)}},
		{Text: "Nobody has answered.", Evidence: []crmcontracts.CompanyBriefEvidence{cited(activity, id)}},
	}
	reader := &fakeReader{answer: map[ids.UUID]crmcontracts.EmailSummary{
		ids.UUID(id): summary(t, id, "Translation fallback"),
	}}

	if err := briefevidence.Attach(context.Background(), reader, briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	if reader.calls != 1 {
		t.Fatalf("read %d times, want exactly one read for the whole response", reader.calls)
	}
	if len(reader.asked[0]) != 1 {
		t.Fatalf("asked for %d ids, want the one message deduped to a single id", len(reader.asked[0]))
	}
	for i, sentence := range sentences {
		got := sentence.Evidence[0].EmailSummary
		if got == nil {
			t.Fatalf("sentence %d kept no summary; both citations name the same readable message", i)
		}
		if got.Subject == nil || *got.Subject != "Translation fallback" {
			t.Errorf("sentence %d carries %v, want the message's own subject", i, got.Subject)
		}
	}
}

// The refusal case for the same rule: a response citing no activity spends no
// statement. Without this the dedupe above would pass over an implementation
// that always read.
func TestEvidenceWithNoActivityReadsNothing(t *testing.T) {
	t.Parallel()
	sentences := []crmcontracts.CompanyBriefSentence{{
		Text: "The deal is open and the champion is named.",
		Evidence: []crmcontracts.CompanyBriefEvidence{
			cited(deal, uuid(t, 2)),
			cited(contact, uuid(t, 3)),
			cited(factEvidence, uuid(t, 4)),
		},
	}}
	reader := &fakeReader{}

	if err := briefevidence.Attach(context.Background(), reader, briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	if reader.calls != 0 {
		t.Fatalf("read %d times over evidence citing no activity, want none", reader.calls)
	}
	for _, each := range sentences[0].Evidence {
		if each.EmailSummary != nil {
			t.Errorf("%s citation gained an email summary", each.EntityType)
		}
	}
}

// An id the reader does not answer for stays absent. That covers the three
// cases the store folds into one: not an email, not this reader's to read, and
// no such row. The citation still renders — as prose, with nothing to open.
func TestAnUnansweredIdLeavesTheCitationBare(t *testing.T) {
	t.Parallel()
	readable, withheld := uuid(t, 5), uuid(t, 6)
	suggestions := []crmcontracts.Company360Suggestion{{
		Evidence: []crmcontracts.CompanyBriefEvidence{cited(activity, readable), cited(activity, withheld)},
	}}
	reader := &fakeReader{answer: map[ids.UUID]crmcontracts.EmailSummary{
		ids.UUID(readable): summary(t, readable, "Yours to read"),
	}}

	if err := briefevidence.Attach(context.Background(), reader, briefevidence.FromSuggestions(suggestions)); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	if suggestions[0].Evidence[0].EmailSummary == nil {
		t.Error("the readable message kept no summary; the admit case is what says the refusal below means something")
	}
	if suggestions[0].Evidence[1].EmailSummary != nil {
		t.Error("an id the reader did not answer for gained a summary")
	}
}

// A reader error fails the response. The alternative — dropping the summaries
// and rendering on — is a page that silently degrades to unopenable citations,
// which is the defect this package removes wearing a green test.
func TestAReaderErrorFailsTheResponseAndTouchesNothing(t *testing.T) {
	t.Parallel()
	id := uuid(t, 7)
	reasons := []crmcontracts.AccountDraftReason{{
		Kind: crmcontracts.AccountDraftReasonKindConversation, Label: "They asked",
		EvidenceRef: &crmcontracts.CompanyBriefEvidence{EntityType: activity, EntityId: id},
	}}
	sentinel := errors.New("the connection went away")
	reader := &fakeReader{err: sentinel}

	err := briefevidence.Attach(context.Background(), reader, briefevidence.FromReasons(reasons))

	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the reader's own error wrapped", err)
	}
	if reasons[0].EvidenceRef.EmailSummary != nil {
		t.Error("a failed read still wrote a summary")
	}
}

// A service constructed without a reader draws citations exactly as it did
// before. That is the unit-test path only: compose's assembly test is what
// holds that every production construction wires one.
func TestANilReaderChangesNothing(t *testing.T) {
	t.Parallel()
	sentences := []crmcontracts.CompanyBriefSentence{{
		Evidence: []crmcontracts.CompanyBriefEvidence{cited(activity, uuid(t, 8))},
	}}

	if err := briefevidence.Attach(context.Background(), nil, briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("a nil reader is a no-op, got %v", err)
	}
	if sentences[0].Evidence[0].EmailSummary != nil {
		t.Error("a nil reader wrote a summary")
	}
}

// The deal card's basis is its own wire shape, so it needs its own collector.
// A basis line with no activity behind it is prose and asks for nothing.
func TestTheDealMoveBasisIsEnrichedAndAProseLineIsNot(t *testing.T) {
	t.Parallel()
	id := uuid(t, 9)
	basis := []crmcontracts.DealNextBestActionEvidence{
		{Text: "They asked on 3 September and nobody replied.", ActivityId: &id},
		{Text: "The close date is inside the week."},
	}
	reader := &fakeReader{answer: map[ids.UUID]crmcontracts.EmailSummary{
		ids.UUID(id): summary(t, id, "Fallback decision"),
	}}

	if err := briefevidence.Attach(context.Background(), reader, briefevidence.FromDealMove(basis)); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	if len(reader.asked) != 1 || len(reader.asked[0]) != 1 {
		t.Fatalf("asked %v, want the one line that names an activity", reader.asked)
	}
	if basis[0].EmailSummary == nil {
		t.Error("the line naming a readable message kept no summary")
	}
	if basis[1].EmailSummary != nil {
		t.Error("a basis line naming no activity gained a summary")
	}
}

// FromActivities answers out of rows already in hand, which is what lets the
// deal card enrich with no second statement. A row that is not an email carries
// no summary and yields none.
func TestFromActivitiesAnswersHeldRowsAndSkipsNonEmails(t *testing.T) {
	t.Parallel()
	mail, note := uuid(t, 10), uuid(t, 11)
	held := summary(t, mail, "Held already")
	rows := []crmcontracts.Activity{
		{Id: mail, EmailSummary: &held},
		{Id: note},
	}
	evidence := []crmcontracts.CompanyBriefEvidence{cited(activity, mail), cited(activity, note)}

	if err := briefevidence.Attach(context.Background(), briefevidence.FromActivities(rows), briefevidence.FromEvidence(evidence)); err != nil {
		t.Fatalf("attaching: %v", err)
	}

	if evidence[0].EmailSummary == nil || evidence[0].EmailSummary.Subject == nil ||
		*evidence[0].EmailSummary.Subject != "Held already" {
		t.Error("the held email row did not reach its citation")
	}
	if evidence[1].EmailSummary != nil {
		t.Error("an activity holding no email summary produced one")
	}
}

// Strip is what stands between an enriched value and a cache. It clears the
// same fields the collectors fill, so a writer can never strip a different set
// than the reader wrote.
func TestStripClearsWhatAttachFilled(t *testing.T) {
	t.Parallel()
	id := uuid(t, 12)
	sentences := []crmcontracts.CompanyBriefSentence{{
		Evidence: []crmcontracts.CompanyBriefEvidence{cited(activity, id)},
	}}
	reader := &fakeReader{answer: map[ids.UUID]crmcontracts.EmailSummary{
		ids.UUID(id): summary(t, id, "Not for the cache"),
	}}
	if err := briefevidence.Attach(context.Background(), reader, briefevidence.FromSentences(sentences)); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if sentences[0].Evidence[0].EmailSummary == nil {
		t.Fatal("nothing was attached, so stripping proves nothing")
	}

	briefevidence.Strip(briefevidence.FromSentences(sentences))

	if sentences[0].Evidence[0].EmailSummary != nil {
		t.Error("a summary survived Strip and would reach the cache")
	}
}
