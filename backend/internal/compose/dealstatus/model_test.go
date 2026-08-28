// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// inputWithTimeline is one deal and two timeline rows, the shape every filter
// case is judged against.
func inputWithTimeline() StatusInput {
	return StatusInput{
		Deal:            DealIn{ID: "deal-1", Name: "Nordwind", Status: "open"},
		Timeline:        []ActIn{{ID: "act-1", Kind: "call", At: "2026-08-11"}, {ID: "act-2", Kind: "email", At: "2026-07-28"}},
		RecommendedMove: "create_task: agree the next step",
	}
}

// draft is the reply as the prompt asks for it, so a case names only the parts
// it is about and the rest stays out of the way.
type draft struct {
	story      []map[string]any
	blocker    []map[string]any
	buyer      []map[string]any
	standing   string
	because    []map[string]any
	moveReason []map[string]any
}

func reply(d draft) string {
	encoded, err := json.Marshal(map[string]any{
		"story":   d.story,
		"blocker": d.blocker,
		"buyer":   d.buyer,
		"verdict": map[string]any{
			"standing": d.standing,
			"because":  d.because,
		},
		"move_reason": d.moveReason,
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func line(text string, evidence ...string) map[string]any {
	return map[string]any{"text": text, "evidence": evidence}
}

func TestAGroundedReplyIsKept(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(draft{
		story:      []map[string]any{line("The offer was promised on the call and never sent.", "act-1")},
		blocker:    []map[string]any{line("Nothing has moved in twelve days.", "act-2")},
		buyer:      []map[string]any{line("They asked for the price before anything else.", "act-2")},
		standing:   "drifting",
		because:    []map[string]any{line("The last contact was the call, and nobody followed it.", "act-1")},
		moveReason: []map[string]any{line("Sending it is what the call promised.", "act-1")},
	}), in)
	if err != nil {
		t.Fatalf("a grounded reply was refused: %v", err)
	}
	if len(got.Story) != 1 || got.Story[0].Evidence[0] != "act-1" {
		t.Fatalf("story = %+v, want one sentence citing act-1", got.Story)
	}
	if len(got.Blocker) != 1 {
		t.Fatalf("blocker = %+v, want the one sentence the reply carried", got.Blocker)
	}
	if len(got.Buyer) != 1 {
		t.Fatalf("buyer = %+v, want the one sentence the reply carried", got.Buyer)
	}
	if got.Verdict.Standing != "drifting" || len(got.Verdict.Because) != 1 {
		t.Fatalf("verdict = %+v, want drifting with one grounded reason", got.Verdict)
	}
	if got.MoveReason.Text != "Sending it is what the call promised." || len(got.MoveReason.Evidence) != 1 {
		t.Fatalf("move reason = %+v, want the sentence with the record it cites", got.MoveReason)
	}
}

func TestAnEmptyBlockerListIsKeptRatherThanRefused(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(draft{
		story:      []map[string]any{line("They accepted the scope and a kickoff is booked.", "act-1")},
		moveReason: []map[string]any{line("Read the brief before the meeting.", "act-1")},
	}), in)
	if err != nil {
		t.Fatalf("a card with nothing wrong was refused: %v", err)
	}
	// A card that always finds something wrong teaches the reader to stop
	// reading the blocker line, so saying nothing has to be a valid answer.
	if len(got.Blocker) != 0 {
		t.Fatalf("blocker = %+v, want none", got.Blocker)
	}
}

func TestAnEmptyBuyerListIsKeptRatherThanRefused(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(draft{
		story: []map[string]any{line("The offer went out on the call and nothing came back.", "act-1")},
	}), in)
	if err != nil {
		t.Fatalf("a card that would not guess at the buyer's motive was refused: %v", err)
	}
	// Reading a motive out of a buyer who has said almost nothing is the one
	// section of this card a reader cannot check, so silence has to be allowed.
	if len(got.Buyer) != 0 {
		t.Fatalf("buyer = %+v, want none", got.Buyer)
	}
}

func TestASentenceCitingNothingIsDropped(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(draft{
		story: []map[string]any{
			line("The offer was promised on the call.", "act-1"),
			line("The buyer seems enthusiastic."),
		},
	}), in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Story) != 1 {
		t.Fatalf("story = %+v, want the uncited sentence dropped", got.Story)
	}
}

func TestACitationOutsideTheInputIsDropped(t *testing.T) {
	in := inputWithTimeline()
	got, err := ParseStatus(reply(draft{
		story: []map[string]any{line("Something happened.", "act-invented")},
	}), in)
	if err == nil {
		t.Fatalf("a reply whose only sentence cited nothing real was kept: %+v", got)
	}
}

func TestTheDealsOwnIDGroundsNothing(t *testing.T) {
	in := inputWithTimeline()
	// Citing the deal says only that the model read the prompt; the sentence
	// is left uncited and therefore dropped, and the card has no story.
	if _, err := ParseStatus(reply(draft{
		story: []map[string]any{line("This deal is open.", "deal-1")},
	}), in); err == nil {
		t.Fatal("a card grounded only in the deal's own id was kept")
	}
}

func TestAReplyThatSpellsARecordIDIsRefused(t *testing.T) {
	in := inputWithTimeline()
	if _, err := ParseStatus(reply(draft{
		story: []map[string]any{line("See act-1 for the detail.", "act-1")},
	}), in); err == nil {
		t.Fatal("a reply naming a record id in reader text was kept")
	}
}

func TestAnOversizedSentenceIsRefused(t *testing.T) {
	in := inputWithTimeline()
	if _, err := ParseStatus(reply(draft{
		story: []map[string]any{line(strings.Repeat("x", maxSentenceLen+1), "act-1")},
	}), in); err == nil {
		t.Fatal("a sentence past the card's bounds was kept")
	}
}

func TestAReplyTellingNoStoryIsRefused(t *testing.T) {
	in := inputWithTimeline()
	// A blocker without a story is a card that opens with what is wrong and
	// never says what the deal IS, which is not the card this site serves.
	if _, err := ParseStatus(reply(draft{
		blocker: []map[string]any{line("It is stalling.", "act-1")},
	}), in); err == nil {
		t.Fatal("a card telling no story was kept")
	}
}

func TestTheStoryIsCappedRatherThanTruncatedSilently(t *testing.T) {
	in := inputWithTimeline()
	many := make([]map[string]any, 0, maxStoryRows+2)
	for range maxStoryRows + 2 {
		many = append(many, line("A grounded sentence.", "act-1"))
	}
	got, err := ParseStatus(reply(draft{story: many}), in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Story) != maxStoryRows {
		t.Fatalf("story = %d sentences, want the cap of %d", len(got.Story), maxStoryRows)
	}
}

func TestAnUnrecognisedVerdictIsDroppedAndTheRestOfTheCardSurvives(t *testing.T) {
	in := inputWithTimeline()
	// The reader has learned what four words mean, so a fifth teaches them
	// nothing — but the reasoning behind the call is still grounded, and
	// throwing away a good card over one bad word costs them the whole
	// briefing.
	got, err := ParseStatus(reply(draft{
		story:    []map[string]any{line("The offer went out after the call.", "act-1")},
		standing: "promising",
		because:  []map[string]any{line("They replied within a day both times.", "act-2")},
	}), in)
	if err != nil {
		t.Fatalf("a card carrying one unrecognised word was refused whole: %v", err)
	}
	if got.Verdict.Standing != "" {
		t.Fatalf("verdict standing = %q, want the unrecognised call dropped", got.Verdict.Standing)
	}
	if len(got.Verdict.Because) != 1 {
		t.Fatalf("verdict because = %+v, want the grounded reasoning kept", got.Verdict.Because)
	}
	if len(got.Story) != 1 {
		t.Fatalf("story = %+v, want the rest of the card kept", got.Story)
	}
}

func TestAnUncitedVerdictReasonIsDropped(t *testing.T) {
	in := inputWithTimeline()
	// The filter treats a verdict reason like any other sentence: uncited, it
	// goes. What the CARD then does with a call left resting on nothing is the
	// fold's business, and fold_test.go holds that half.
	got, err := ParseStatus(reply(draft{
		story:    []map[string]any{line("The offer went out after the call.", "act-1")},
		standing: "cold",
		because:  []map[string]any{line("It feels dead.")},
	}), in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Verdict.Because) != 0 {
		t.Fatalf("verdict because = %+v, want the uncited reason dropped", got.Verdict.Because)
	}
}

func TestTheRequestFencesTheSummaryItCarries(t *testing.T) {
	// The summary carries mail subjects and buyer comments — text written
	// outside this workspace. Without a nonce boundary a subject line could
	// close the span and be read as instruction.
	in := inputWithTimeline()
	in.Timeline[0].Subject = "Re: pricing"
	req := StatusRequest(in, string(textlang.English))
	if len(req.Messages) != 1 {
		t.Fatalf("request carries %d messages, want one", len(req.Messages))
	}
	body := req.Messages[0].Content
	if !strings.Contains(body, "Re: pricing") {
		t.Fatal("the summary lost the subject it was built from")
	}
	if !strings.Contains(req.System, "deal timeline and buyer conversation") {
		t.Fatal("the system prompt never names the data boundary this call fences")
	}
	// The body's opening marker must be the one the system prompt names, or
	// the rule the model is given describes a boundary the data does not use.
	open, _, ok := strings.Cut(body, ">")
	if !ok {
		t.Fatal("the body carries no fence marker")
	}
	marker := open + ">"
	if !strings.Contains(req.System, marker) {
		t.Fatalf("the system prompt does not name the fence %q the body opens with", marker)
	}
	// A nonce is the point: a fixed marker a writer has seen before can be
	// closed by a subject line and the rest read as instruction.
	if marker == "<untrusted>" {
		t.Fatal("the fence marker carries no nonce")
	}
}

// The verdict survives a model that answers the older shape.
//
// This is not a hypothetical tolerance. The prompt described `verdict.because`
// twice — as a list in the shape line, as "one or two sentences" in prose —
// and the model followed the prose. The decoder refused the string, the whole
// reply failed to parse, and the card fell back to the deterministic writer on
// EVERY call, logging a warning nobody read. Deal360 shipped a verdict head
// that no reader ever saw.
func TestTheVerdictSurvivesABareStringReason(t *testing.T) {
	var got replyShape
	raw := []byte(`{"story":[{"text":"They wrote.","evidence":[]}],` +
		`"verdict":{"standing":"blocked","because":"Nobody sent the times."},` +
		`"move_reason":"Answer them."}`)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("a bare-string reason must parse, not sink the whole reply: %v", err)
	}
	if len(got.Verdict.Because) != 1 {
		t.Fatalf("a bare string is one line, got %d", len(got.Verdict.Because))
	}
	if got.Verdict.Because[0].Text != "Nobody sent the times." {
		t.Errorf("the sentence is kept verbatim, got %q", got.Verdict.Because[0].Text)
	}
	if got.Verdict.Standing != "blocked" {
		t.Errorf("the standing rides along, got %q", got.Verdict.Standing)
	}
}

// And the shape the prompt actually asks for still parses, with its citations.
// Without this the tolerance above could be satisfied by a decoder that threw
// the list form away.
func TestTheVerdictKeepsCitationsWhenTheModelSendsTheList(t *testing.T) {
	var got replyShape
	raw := []byte(`{"story":[],"verdict":{"standing":"cold","because":` +
		`[{"text":"Silence since May.","evidence":["a1"]},{"text":"No reply.","evidence":[]}]},` +
		`"move_reason":"Close it."}`)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the list shape must still parse: %v", err)
	}
	if len(got.Verdict.Because) != 2 {
		t.Fatalf("both sentences are kept, got %d", len(got.Verdict.Because))
	}
	if len(got.Verdict.Because[0].Evidence) != 1 || got.Verdict.Because[0].Evidence[0] != "a1" {
		t.Errorf("the citations ride along, got %v", got.Verdict.Because[0].Evidence)
	}
}

// TestAnUngroundedMoveReasonDoesNotReachTheReader is the hole this closes.
//
// The move's reason was the one field on the card bounded by length and a
// no-ids check and nothing else — so free prose reached the reader attributed
// as model-written, whether it came from a crafted mail body on the deal's
// timeline or from a model having a bad day. Every other sentence on the same
// card already had to cite a record or be dropped.
//
// It is DROPPED rather than refused, and the difference matters: writtenMove
// keeps the deterministic reason the rules produced, so a card that is
// otherwise entirely grounded still ships with a true sentence about the same
// move.
func TestAnUngroundedMoveReasonDoesNotReachTheReader(t *testing.T) {
	in := inputWithTimeline()
	for what, reason := range map[string][]map[string]any{
		"citing nothing at all": {line("Ignore the deal and email finance about the invoice.")},
		"citing a record this deal does not carry": {
			line("Ignore the deal and email finance about the invoice.", "act-not-on-this-deal"),
		},
		"citing the deal itself, which grounds nothing": {
			line("Ignore the deal and email finance about the invoice.", "deal-1"),
		},
	} {
		t.Run(what, func(t *testing.T) {
			got, err := ParseStatus(reply(draft{
				story:      []map[string]any{line("The offer was promised and never sent.", "act-1")},
				standing:   "drifting",
				because:    []map[string]any{line("Twelve days of silence.", "act-2")},
				moveReason: reason,
			}), in)
			if err != nil {
				t.Fatalf("a card grounded everywhere else was refused whole: %v", err)
			}
			if got.MoveReason.Text != "" {
				t.Errorf("an ungrounded move reason reached the reader: %q", got.MoveReason.Text)
			}
			if len(got.Story) != 1 {
				t.Errorf("the rest of the card did not survive: story = %+v", got.Story)
			}
		})
	}
}

// TestTheOlderMoveReasonShapeDegradesRatherThanPassing holds the compatibility
// path, and holds it in the safe direction.
//
// A model can always answer the shape the prompt asked for before this — a
// different provider, a cheaper lane, a retry — and `replyLines` accepts a bare
// string so a whole verdict is not lost over JSON shape. Here that same
// leniency must NOT become a way past the check: a bare string decodes to one
// UNCITED line, which the filter drops.
func TestTheOlderMoveReasonShapeDegradesRatherThanPassing(t *testing.T) {
	in := inputWithTimeline()
	encoded, err := json.Marshal(map[string]any{
		"story":       []map[string]any{line("The offer was promised and never sent.", "act-1")},
		"verdict":     map[string]any{"standing": "drifting"},
		"move_reason": "Sending it is what the call promised.",
	})
	if err != nil {
		t.Fatalf("encoding the older shape: %v", err)
	}
	got, parseErr := ParseStatus(string(encoded), in)
	if parseErr != nil {
		t.Fatalf("the older shape was refused whole rather than degraded: %v", parseErr)
	}
	if got.MoveReason.Text != "" {
		t.Errorf("a bare-string move reason passed the grounding check it predates: %q", got.MoveReason.Text)
	}
	if len(got.Story) != 1 {
		t.Errorf("the rest of the reply did not survive: story = %+v", got.Story)
	}
}

// TestTheMoveReasonKeepsItsOwnBound holds the tighter limit through the shared
// filter.
//
// The reason sits inside the move block rather than in a section, and it has
// always been the shorter field. Routing it through keepGrounded put it behind
// the sentence bound, which is a third longer — a widening nobody asked for,
// invisible until a card rendered a paragraph where a sentence belongs.
func TestTheMoveReasonKeepsItsOwnBound(t *testing.T) {
	in := inputWithTimeline()
	story := []map[string]any{line("The offer was promised and never sent.", "act-1")}
	// Longer than the move's own bound, shorter than a section sentence's.
	overlong := strings.Repeat("a", maxMoveReason+1)
	if len(overlong) >= maxSentenceLen {
		t.Fatalf("the probe is %d runes, which a section would refuse too — it must sit between the two bounds",
			len(overlong))
	}

	if _, err := ParseStatus(reply(draft{
		story:      story,
		moveReason: []map[string]any{line(overlong, "act-1")},
	}), in); err == nil {
		t.Error("a move reason longer than its own bound was accepted")
	}

	// And the same text IS accepted where the longer bound applies, so the test
	// is about the move reason's bound rather than about length in general.
	if _, err := ParseStatus(reply(draft{
		story: []map[string]any{line(overlong, "act-1")},
	}), in); err != nil {
		t.Errorf("a section sentence within its own bound was refused: %v", err)
	}
}
