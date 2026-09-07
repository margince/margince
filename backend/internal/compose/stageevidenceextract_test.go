// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the stage_evidence_extract site may accept from a model, and what it
// must refuse outright.
//
// The refusals are the subject. This site reads text a counterparty wrote,
// against criteria a stage move will later rest on, so a reply it accepts is a
// citation a human is invited to trust.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/deals"
)

var evidenceCriteria = []stageEvidenceCriterion{
	{Key: "security_review", Label: "Security review cleared", Hint: "their security team signed off"},
	{Key: "budget_confirmed", Label: "Budget confirmed"},
}

var evidenceSpans = []stageEvidenceSpan{
	{SourceID: "01a07000-0000-7000-8000-000000000001", Lines: []string{
		"Ines: security have finished their review and we're clear to proceed.",
		"Rep: that's great news.",
	}},
	{SourceID: "01a07000-0000-7000-8000-000000000002", Lines: []string{
		"Ines: budget is not signed off yet, we're waiting on the CFO.",
	}},
}

// oneClaim renders a reply carrying a single claim, so each test names only
// the field it is about.
func oneClaim(t *testing.T, claim stageEvidenceClaim) string {
	t.Helper()
	raw, err := json.Marshal(stageEvidencePayload{Claims: &[]stageEvidenceClaim{claim}})
	if err != nil {
		t.Fatalf("rendering a reply: %v", err)
	}
	return string(raw)
}

// groundedClaim is a reply this site accepts, and the control every refusal
// below is measured against: without it a validator refusing everything would
// pass each of them.
func groundedClaim() stageEvidenceClaim {
	return stageEvidenceClaim{
		CriterionKey: "security_review",
		SourceID:     evidenceSpans[0].SourceID,
		SourceLines:  []int{1},
		Quote:        "security have finished their review",
		Met:          stageEvidenceMet,
		Commitment:   deals.CommitmentAgreed,
		Confidence:   0.9,
	}
}

func TestAGroundedClaimIsAccepted(t *testing.T) {
	valid := stageEvidenceValid(evidenceCriteria, evidenceSpans)
	if err := valid(oneClaim(t, groundedClaim())); err != nil {
		t.Fatalf("a claim quoting the line it cites was refused: %v", err)
	}
}

// The schema has no stage field, so a reply naming one is not a shape this
// site can even express. What follows from a settled criterion is the policy
// function's call, made from the whole ledger — not one conversation's.
func TestTheReplyMayNotNameAStage(t *testing.T) {
	for _, forbidden := range []string{"stage", "stage_id", "to_stage", "advance"} {
		if strings.Contains(string(stageEvidenceSchema()), forbidden) {
			t.Errorf("the reply schema names %q; a reader that can name a stage "+
				"is deciding the move on one conversation's evidence", forbidden)
		}
	}
	// author_side belongs to the same family: it is computed from the
	// activity's participants, so a reply asserting it would be claiming
	// authorship of a span whose author this site already knows.
	if strings.Contains(string(stageEvidenceSchema()), "author_side") {
		t.Error("the reply schema names author_side, which is computed and never claimed")
	}
}

// A claim citing a record this call did not read invalidates the WHOLE reply,
// not just itself: a reader inventing one source was not reading the others.
func TestAClaimCitingAnUnmintedSourceInvalidatesTheWholeReply(t *testing.T) {
	claim := groundedClaim()
	claim.SourceID = "01a07000-0000-7000-8000-00000000dead"
	err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim))
	if err == nil {
		t.Fatal("a claim citing a source this call never read was accepted")
	}
	if !strings.Contains(err.Error(), "not read by this call") {
		t.Errorf("the refusal does not name the unminted source: %v", err)
	}
}

// The whole reply goes, including claims that would validate on their own.
func TestOneUngroundedClaimTakesTheGroundedOnesWithIt(t *testing.T) {
	ungrounded := groundedClaim()
	ungrounded.CriterionKey = "budget_confirmed"
	ungrounded.Quote = "the CFO approved it this morning"
	raw, err := json.Marshal(stageEvidencePayload{
		Claims: &[]stageEvidenceClaim{groundedClaim(), ungrounded},
	})
	if err != nil {
		t.Fatalf("rendering a reply: %v", err)
	}
	if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(string(raw)); err == nil {
		t.Fatal("a reply pairing a grounded claim with an invented one was accepted; " +
			"the grounded one is the same reading's output")
	}
}

// A criterion key this deal's stage does not carry has nothing to be written
// against, and a reader offered two keys that answers about a third read
// neither.
func TestAnUnknownCriterionKeyInvalidatesTheReply(t *testing.T) {
	claim := groundedClaim()
	claim.CriterionKey = "champion_identified"
	err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim))
	if err == nil {
		t.Fatal("a claim on a criterion this stage does not carry was accepted")
	}
	if !strings.Contains(err.Error(), "not offered to this reading") {
		t.Errorf("the refusal does not name the unknown key: %v", err)
	}
}

// The quote must be in the lines the claim NAMES, not merely somewhere in the
// conversation. A citation a reader clicks has to lead to the passage.
func TestAQuoteFromAnotherLineIsNotACitation(t *testing.T) {
	claim := groundedClaim()
	claim.SourceLines = []int{2}
	err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim))
	if err == nil {
		t.Fatal("a claim quoting line 1 while citing line 2 was accepted")
	}
	if !strings.Contains(err.Error(), "not in the lines it cites") {
		t.Errorf("the refusal does not name the mislocated quote: %v", err)
	}
}

// A quote reflowed across a wrap is the same passage. Refusing it would make
// the guard fire on formatting rather than on invention.
func TestAReflowedQuoteStillCounts(t *testing.T) {
	claim := groundedClaim()
	claim.Quote = "security   have finished\n  their review"
	if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim)); err != nil {
		t.Fatalf("a reflowed quote was refused: %v", err)
	}
}

// A line number outside the span points at text this call never supplied.
func TestALineOutsideTheSpanIsRefused(t *testing.T) {
	claim := groundedClaim()
	claim.SourceLines = []int{99}
	if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim)); err == nil {
		t.Fatal("a claim citing line 99 of a two-line span was accepted")
	}
}

// met is a two-value enum, and the third state is expressed by omitting the
// criterion. A reply inventing "unknown" is answering a question this site did
// not ask.
func TestMetTakesOnlyTrueOrFalse(t *testing.T) {
	claim := groundedClaim()
	claim.Met = "unknown"
	if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim)); err == nil {
		t.Fatal(`met:"unknown" was accepted; silence about a criterion is an omission`)
	}
}

// The commitment vocabulary is the ledger's, so a value this site accepted but
// the store refuses would fail at the write with the reading already paid for.
func TestTheCommitmentVocabularyIsTheLedgersOwn(t *testing.T) {
	claim := groundedClaim()
	claim.Commitment = "definitely"
	if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim)); err == nil {
		t.Fatal("a commitment outside the ledger's vocabulary was accepted")
	}
	for _, allowed := range []string{
		deals.CommitmentAgreed, deals.CommitmentProposed, deals.CommitmentNone,
	} {
		ok := groundedClaim()
		ok.Commitment = allowed
		if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, ok)); err != nil {
			t.Errorf("the ledger's own commitment %q was refused: %v", allowed, err)
		}
	}
}

// A reply with no claims key did not answer; one with an empty list said the
// conversation settles nothing, which is the common and correct answer.
func TestAnEmptyListIsAnAnswerAndAMissingKeyIsNot(t *testing.T) {
	valid := stageEvidenceValid(evidenceCriteria, evidenceSpans)
	if err := valid(`{"claims": []}`); err != nil {
		t.Errorf("a conversation settling nothing was refused: %v", err)
	}
	if err := valid(`{}`); err == nil {
		t.Error("a reply carrying no claims key was read as settling nothing")
	}
}

// Two readings of one criterion in a single reply either disagree or repeat,
// and a reader would have to arbitrate between them.
func TestOneCriterionIsClaimedOnceInAReading(t *testing.T) {
	second := groundedClaim()
	second.Met = stageEvidenceNotMet
	raw, err := json.Marshal(stageEvidencePayload{
		Claims: &[]stageEvidenceClaim{groundedClaim(), second},
	})
	if err != nil {
		t.Fatalf("rendering a reply: %v", err)
	}
	if err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(string(raw)); err == nil {
		t.Fatal("one criterion claimed both met and unmet in one reading was accepted")
	}
}

// Every span reaches the model inside its own fence, addressed by the id a
// claim cites it by — so a buyer writing "ignore your instructions" is inside
// the fence with the rest of their message.
func TestEverySpanReachesTheModelInsideItsOwnFence(t *testing.T) {
	req := stageEvidenceRequest(evidenceCriteria, evidenceSpans, "en")
	prompt := req.Messages[0].Content
	for _, span := range evidenceSpans {
		for i, line := range span.Lines {
			addressed := span.SourceID + ":" + strconv.Itoa(i+1)
			if !strings.Contains(prompt, addressed) {
				t.Errorf("span %s line %d is not addressed in the prompt, so a "+
					"claim could not cite it", span.SourceID, i+1)
			}
			if !strings.Contains(prompt, line) {
				t.Errorf("span %s line %d did not reach the model", span.SourceID, i+1)
			}
		}
	}
	// The fence rule must be in the system prompt, or the markers around the
	// data mean nothing to the model reading them.
	if !strings.Contains(req.System, "never instructions") {
		t.Error("the system prompt does not carry the fence rule")
	}
	// EVERY line WRAPPED, not merely present. Checking that the text appears
	// somewhere passes just as well against plain concatenation, which is the
	// composition this whole test exists to refuse — untrusted text reaching
	// the model outside a boundary. So each line is required to sit between
	// the markers, and the count of wrappers is required to match the count of
	// lines, which no unfenced build can satisfy.
	// The nonce is minted per request and unknowable here, so it is read back
	// out of the prompt once and then required literally. RE2 has no
	// backreferences, which is why this is not one regex per line.
	nonce := regexp.MustCompile(`<(\S+) span="`).FindStringSubmatch(prompt)
	if nonce == nil {
		t.Fatal("no fenced span in the prompt at all: every line reached the " +
			"model as bare text, which is what the fence exists to prevent")
	}
	fenced := 0
	for _, span := range evidenceSpans {
		for i, line := range span.Lines {
			addressed := span.SourceID + ":" + strconv.Itoa(i+1)
			// The SHAPE, not mere presence: an opening tag carrying this
			// line's address, then this line, then the matching close.
			wrapped := "<" + nonce[1] + ` span="` + addressed + `">` + line +
				"</" + nonce[1] + ">"
			if !strings.Contains(prompt, wrapped) {
				t.Errorf("span %s line %d is in the prompt but not inside a fence "+
					"opened for it — untrusted text outside the boundary is what "+
					"the fence exists to prevent", span.SourceID, i+1)
				continue
			}
			fenced++
		}
	}
	if want := len(evidenceSpans[0].Lines) + len(evidenceSpans[1].Lines); fenced != want {
		t.Errorf("%d of %d lines reached the model fenced", fenced, want)
	}
}

// The criteria reach the model by KEY, because the key is what a claim must
// name back and an unlisted key is what the validator refuses.
func TestTheCriteriaReachTheModelByKey(t *testing.T) {
	prompt := stageEvidenceRequest(evidenceCriteria, evidenceSpans, "en").Messages[0].Content
	for _, c := range evidenceCriteria {
		if !strings.Contains(prompt, c.Key) {
			t.Errorf("criterion key %q was not offered to the model", c.Key)
		}
		if !strings.Contains(prompt, c.Label) {
			t.Errorf("criterion %q reached the model without its label", c.Key)
		}
	}
	if !strings.Contains(prompt, evidenceCriteria[0].Hint) {
		t.Error("a criterion's hint did not reach the model")
	}
}

// The splice: lines that read "We have", "not", "approved the budget" joined
// as 1 and 3 make a sentence saying the opposite of the record, grounded
// against text that refutes it, with a citation a reader would click and find
// convincing.
func TestACitationCannotSpliceAroundTheWordsThatReverseIt(t *testing.T) {
	spans := []stageEvidenceSpan{{
		SourceID: evidenceSpans[0].SourceID,
		Lines:    []string{"Ines: we have", "not", "approved the budget yet."},
	}}
	claim := stageEvidenceClaim{
		CriterionKey: "budget_confirmed",
		SourceID:     spans[0].SourceID,
		SourceLines:  []int{1, 3},
		Quote:        "we have approved the budget yet.",
		Met:          stageEvidenceMet,
		Commitment:   deals.CommitmentAgreed,
		Confidence:   0.95,
	}
	err := stageEvidenceValid(evidenceCriteria, spans)(oneClaim(t, claim))
	if err == nil {
		t.Fatal("a quote spliced across the line that negates it was accepted")
	}
	if !strings.Contains(err.Error(), "skips line 2") {
		t.Errorf("the refusal does not name the skipped line: %v", err)
	}
}

// Reordering is the same fabrication by another route: cite 3 then 1 and the
// joined text reads backwards from the record.
func TestACitationMayNotReorderTheRecord(t *testing.T) {
	claim := groundedClaim()
	claim.SourceLines = []int{2, 1}
	err := stageEvidenceValid(evidenceCriteria, evidenceSpans)(oneClaim(t, claim))
	if err == nil {
		t.Fatal("a claim citing lines in reverse order was accepted")
	}
	if !strings.Contains(err.Error(), "out of order") {
		t.Errorf("the refusal does not name the reordering: %v", err)
	}
}

// A commitment stated either side of an interruption is one commitment, and it
// is quotable — by citing the interruption too. Contiguity costs the claim
// nothing, because the quote is matched under collapsed whitespace against the
// whole joined run: the reader clicks through to the passage as it was said,
// interruption included, which is what actually happened.
func TestAQuoteMaySpanAnInterruptionByCitingIt(t *testing.T) {
	spans := []stageEvidenceSpan{{
		SourceID: evidenceSpans[0].SourceID,
		Lines: []string{
			"Ines: security have finished their review",
			"Lars: sorry, one second",
			"Ines: and we are clear to proceed.",
		},
	}}
	claim := stageEvidenceClaim{
		CriterionKey: "security_review",
		SourceID:     spans[0].SourceID,
		SourceLines:  []int{1, 2, 3},
		Quote:        "security have finished their review Lars: sorry, one second Ines: and we are clear to proceed.",
		Met:          stageEvidenceMet,
		Commitment:   deals.CommitmentAgreed,
		Confidence:   0.95,
	}
	if err := stageEvidenceValid(evidenceCriteria, spans)(oneClaim(t, claim)); err != nil {
		t.Fatalf("a commitment quoted across an interruption it cites was refused: %v", err)
	}
}

// The prompt promises 300 CHARACTERS, so the bound counts them. A byte count
// rejects a valid non-Latin quote at roughly a third of the stated length.
func TestTheQuoteBoundCountsCharactersNotBytes(t *testing.T) {
	// 200 CJK characters: ~600 bytes, comfortably inside a character bound and
	// well outside a byte one.
	long := strings.Repeat("預", 200)
	spans := []stageEvidenceSpan{{SourceID: evidenceSpans[0].SourceID, Lines: []string{long}}}
	claim := stageEvidenceClaim{
		CriterionKey: "security_review",
		SourceID:     spans[0].SourceID,
		SourceLines:  []int{1},
		Quote:        long,
		Met:          stageEvidenceMet,
		Commitment:   deals.CommitmentAgreed,
		Confidence:   0.9,
	}
	if err := stageEvidenceValid(evidenceCriteria, spans)(oneClaim(t, claim)); err != nil {
		t.Fatalf("a 200-character quote was refused: %v", err)
	}
	over := strings.Repeat("預", maxStageEvidenceQuote+1)
	spans[0].Lines = []string{over}
	claim.Quote = over
	if err := stageEvidenceValid(evidenceCriteria, spans)(oneClaim(t, claim)); err == nil {
		t.Error("a quote over the character bound was accepted")
	}
}

// The certification case grades WHICH passage settled a criterion, not merely
// that the reply named the right one.
//
// The validator's job is to prove a quote is IN the lines it cites; it cannot
// know whether those lines say the thing. So a reply can settle
// economic_buyer_identified while quoting "helpful, thank you" — grounded, on
// the right criterion, evidence of nothing — and without settled_by the
// deterministic half of the certification counts that as correct.
func TestTheCertificationGradesWhichPassageSettledTheCriterion(t *testing.T) {
	fixture := stageEvidenceFixture{
		Criteria: []stageEvidenceCriterion{{Key: "economic_buyer", Label: "The economic buyer is identified"}},
		Spans: []stageEvidenceSpan{{
			SourceID: evidenceSpans[0].SourceID,
			Lines: []string{
				"Lars: who ultimately signs off on something this size?",
				"Ines: that would be Martin, our COO.",
				"Lars: helpful, thank you.",
			},
		}},
	}
	prepared := &stageEvidenceCase{
		fixture: fixture,
		expected: []stageEvidenceExpectation{
			{CriterionKey: "economic_buyer", Met: true, SettledBy: "Martin"},
		},
	}

	naming := oneClaim(t, stageEvidenceClaim{
		CriterionKey: "economic_buyer", SourceID: fixture.Spans[0].SourceID,
		SourceLines: []int{2}, Quote: "that would be Martin, our COO.",
		Met: stageEvidenceMet, Commitment: deals.CommitmentAgreed, Confidence: 0.9,
	})
	if got := prepared.Evaluate(aitasks.Trace{Output: naming}); got.Result != aitasks.OutcomeAccepted {
		t.Fatalf("the passage that names the buyer was graded %q (%s)", got.Result, got.Detail)
	}

	pleasantry := oneClaim(t, stageEvidenceClaim{
		CriterionKey: "economic_buyer", SourceID: fixture.Spans[0].SourceID,
		SourceLines: []int{3}, Quote: "helpful, thank you.",
		Met: stageEvidenceMet, Commitment: deals.CommitmentAgreed, Confidence: 0.9,
	})
	got := prepared.Evaluate(aitasks.Trace{Output: pleasantry})
	if got.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("a grounded quote that identifies nobody was graded %q; the "+
			"certification counted a fabrication as correct", got.Result)
	}
	if !strings.Contains(got.Detail, "does not say it") {
		t.Errorf("the verdict does not name the useless citation: %s", got.Detail)
	}
}
