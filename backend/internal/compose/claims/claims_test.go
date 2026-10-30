// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package claims

// The grounding filter is the product's answer to "how do I know this is true",
// and it is one implementation precisely so it cannot be lenient in one place
// and strict in another. These cases are DOSS-FORM-1 read back as behaviour.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	companyID = "019fd000-0000-7000-8000-000000000001"
	dealID    = "019fd000-0000-7000-8000-000000000002"
	ghost     = "019fd000-0000-7000-8000-0000000000ff"
)

// supplied is what the assembler put in front of the model: the records, and
// what each of them said. The texts carry the facts the sentences below state,
// so these cases stay about CITATION — the extractive half has its own tests.
func supplied() map[Evidence]string {
	return map[Evidence]string{
		{EntityType: "company", EntityID: companyID}: `{"name":"Glazed Frog GmbH","country":"Germany"}`,
		{EntityType: "deal", EntityID: dealID}:       `{"name":"Renewal","amount":"1200.00","currency":"EUR"}`,
	}
}

func TestASentenceCitingNothingIsDropped(t *testing.T) {
	// DOSS-AC-1. An unattributed sentence is the one thing this surface must
	// never render: the reader has no way to check it and no way to know that.
	if Grounded(Sentence{Text: "They operate in Germany."}, supplied()) {
		t.Error("a sentence with no citation was kept")
	}
	if Grounded(Sentence{Text: "", Evidence: []Evidence{{EntityType: "deal", EntityID: dealID}}}, supplied()) {
		t.Error("an empty sentence was kept")
	}
}

func TestASentenceCitingAnUnsuppliedRecordIsDroppedWhole(t *testing.T) {
	// DOSS-AC-2. Dropped WHOLE, not shown with the bad citation stripped: the
	// claim may rest on the invented half, so keeping it with the good citation
	// attached presents it as checked when it is not.
	sentence := Sentence{
		Text: "They renewed last quarter.",
		Evidence: []Evidence{
			{EntityType: "deal", EntityID: dealID},
			{EntityType: "deal", EntityID: ghost},
		},
	}
	if Grounded(sentence, supplied()) {
		t.Error("a sentence citing an unsupplied record was kept")
	}
	kept := Keep([]Sentence{sentence}, supplied(), map[string]bool{"fact": true}, "fact")
	if len(kept) != 0 {
		t.Errorf("Keep returned %d sentences, want the whole sentence dropped rather than trimmed", len(kept))
	}
}

func TestAValidIdentityOfTheWrongKindIsStillDropped(t *testing.T) {
	// The check is on the (kind, identity) PAIR. Keying on the id alone accepts
	// a real deal id cited as a contact, and the chip then routes the reader to
	// the wrong screen — or to a record of a kind they were never shown.
	sentence := Sentence{
		Text:     "The buyer replied.",
		Evidence: []Evidence{{EntityType: "contact", EntityID: dealID}},
	}
	if Grounded(sentence, supplied()) {
		t.Error("a real id cited under the wrong kind was kept")
	}
}

func TestASentenceSpellingARecordIDAtTheReaderIsDropped(t *testing.T) {
	sentence := Sentence{
		Text:     "See deal " + dealID + " for the renewal.",
		Evidence: []Evidence{{EntityType: "deal", EntityID: dealID}},
	}
	// Grounded in the citation sense, and still developer output. It is dropped
	// rather than stripped: the id sits mid-clause and cutting it leaves broken
	// grammar, while every id the sentence needed is already in its evidence.
	if Grounded(sentence, supplied()) {
		t.Error("a sentence spelling an id in its prose was kept")
	}
	if !SpellsRecordID(sentence.Text) {
		t.Error("SpellsRecordID missed an id the filter caught — the two must agree")
	}
}

func TestAnUndeclaredNatureReducesToFact(t *testing.T) {
	// DOSS-PARAM-1: fact is the only permitted compatibility default. Forwarding
	// the model's own string would render a label the contract does not define.
	kept := Keep(
		[]Sentence{{
			Text:     "They look like a strong fit.",
			Nature:   "hunch",
			Evidence: []Evidence{{EntityType: "company", EntityID: companyID}},
		}},
		supplied(),
		map[string]bool{"fact": true, "assessment": true},
		"fact",
	)
	if len(kept) != 1 {
		t.Fatalf("kept %d sentences, want the grounded one", len(kept))
	}
	if kept[0].Nature != "fact" {
		t.Errorf("nature = %q, want it reduced to fact — the strictest reading", kept[0].Nature)
	}
}

func TestADeclaredNatureSurvives(t *testing.T) {
	// The reduction above must not flatten the distinction the vocabulary
	// exists for: a reader forgives a wrong opinion and not a wrong fact, so an
	// assessment that arrives labelled stays labelled.
	kept := Keep(
		[]Sentence{{
			Text:     "They look like a strong fit.",
			Nature:   "assessment",
			Evidence: []Evidence{{EntityType: "company", EntityID: companyID}},
		}},
		supplied(),
		map[string]bool{"fact": true, "assessment": true},
		"fact",
	)
	if len(kept) != 1 || kept[0].Nature != "assessment" {
		t.Errorf("kept = %+v, want the assessment label preserved", kept)
	}
}

func TestRepeatedCitationsCollapse(t *testing.T) {
	kept := Dedupe([]Sentence{{
		Text: "Two mentions, one record.",
		Evidence: []Evidence{
			{EntityType: "deal", EntityID: dealID},
			{EntityType: "deal", EntityID: dealID},
			{EntityType: "company", EntityID: companyID},
		},
	}})
	if len(kept[0].Evidence) != 2 {
		t.Fatalf("evidence = %+v, want the duplicate collapsed", kept[0].Evidence)
	}
	// First-seen order, so the chips do not reshuffle between reads.
	if kept[0].Evidence[0].EntityID != dealID {
		t.Errorf("evidence order = %+v, want first-seen order kept", kept[0].Evidence)
	}
}

func TestDedupeDoesNotMutateItsInput(t *testing.T) {
	// The brief hands the same slice to more than one writer, and a filter that
	// rewrote its caller's backing array would change what the OTHER writer saw
	// depending on which ran first.
	in := []Sentence{{
		Text: "Two mentions, one record.",
		Evidence: []Evidence{
			{EntityType: "deal", EntityID: dealID},
			{EntityType: "deal", EntityID: dealID},
		},
	}}
	Dedupe(in)
	if len(in[0].Evidence) != 2 {
		t.Errorf("Dedupe rewrote its caller's slice: %+v", in[0].Evidence)
	}
}

func TestTerminateSentence(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"appends a full stop to a bare statement", "Load-shifting software", "Load-shifting software."},
		{
			"keeps the author's own terminator rather than doubling it",
			"What is their pricing model?", "What is their pricing model?",
		},
		{"does not double an existing full stop", "Voltaq Systems GmbH.", "Voltaq Systems GmbH."},
		{"does not turn a trailing ellipsis into two stops", "The rollout is still…", "The rollout is still…"},
		{"strips a dangling list separator rather than terminating it", "Berlin, Munich, ", "Berlin, Munich."},
		{"reduces punctuation-only input to nothing", "  ; : ,  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TerminateSentence(c.value); got != c.want {
				t.Errorf("TerminateSentence(%q) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}

func TestEveryShippedLanguageNamesTheSubjectsThatMakeAPromise(t *testing.T) {
	for _, lang := range textlang.Shipped {
		if len(writerSubjects[lang].all()) == 0 {
			t.Errorf("%q ships with no writer subjects, so a recommendation written as a promise in it is shown", lang)
		}
	}
}

func TestEveryListedWriterSubjectIsRefused(t *testing.T) {
	for lang, subjects := range writerSubjects {
		for _, subject := range subjects.all() {
			if !OpensWithWriterSubject(subject + " send the agreement.") {
				t.Errorf("%s subject %q is listed and still not refused", lang, subject)
			}
		}
	}
}

func TestEveryShippedLanguageNamesTheWriterAlone(t *testing.T) {
	for _, lang := range textlang.Shipped {
		if len(writerSubjects[lang].singular) == 0 {
			t.Errorf("%q ships with no singular writer subject, so an advice line written as the writer's own in it is shown", lang)
		}
	}
}

func TestTheWriterAloneIsRefusedAndTheWriterWithOthersIsNot(t *testing.T) {
	for _, text := range []string{
		"I think we should lead with their plant managers.",
		"I'd lead with their plant managers.",
		"I'll send the agreement.",
		"Ich würde mit den Werksleitern beginnen.",
		"Tôi sẽ gửi hợp đồng.",
		"To\u0302i se\u0303 gu\u031B\u0309i",
		"- I'd lead with their plant managers.",
	} {
		if !OpensWithSingularWriterSubject(text) {
			t.Errorf("OpensWithSingularWriterSubject(%q) = false, want the writer alone refused", text)
		}
	}
	for _, text := range []string{
		"We should lead with their plant managers.",
		"Wir sollten mit den Werksleitern beginnen.",
		"Chúng tôi sẽ gửi hợp đồng.",
		"Let's send the agreement.",
		"Send the agreement.",
		"I/O bottlenecks stall their plants; lead with that.",
	} {
		if OpensWithSingularWriterSubject(text) {
			t.Errorf("OpensWithSingularWriterSubject(%q) = true, want it kept", text)
		}
	}
	for lang, subjects := range writerSubjects {
		for _, subject := range subjects.singular {
			if !OpensWithSingularWriterSubject(subject + " send the agreement.") {
				t.Errorf("%s singular subject %q is listed and still not refused", lang, subject)
			}
		}
		for _, subject := range subjects.plural {
			if OpensWithSingularWriterSubject(subject + " send the agreement.") {
				t.Errorf("%s plural subject %q is refused as the writer alone", lang, subject)
			}
		}
	}
}

func TestARecommendationOpeningWithIOrWeIsAPromise(t *testing.T) {
	for _, text := range []string{
		"I'll send the finalized partnership agreement once legal completes its review.",
		"I will send the agreement.",
		"We should send the agreement today.",
		"Let's send the agreement.",
		"I’ll send the agreement.",
		`"I'll send the agreement," you said.`,
		"'We'll follow up on Monday.'",
		"- I'll send the agreement.",
		"Ich schicke den Vertrag morgen.",
		"„Ich schicke den Vertrag morgen.“",
		"WIR senden den Vertrag.",
		"Tôi sẽ gửi hợp đồng.",
		"Chúng tôi sẽ gửi hợp đồng.",
		"chúng   tôi sẽ gửi hợp đồng.",
		"Let us send the agreement.",
		"Lass uns den Vertrag schicken.",
		"Lasst uns den Vertrag schicken.",
		"Chúng ta sẽ gửi hợp đồng.",
		"Chúng mình sẽ gửi hợp đồng.",
		"1. I'll send it.",
		"2) We'll call them.",
		"To\u0302i se\u0303 gu\u031B\u0309i",
		"Iʼll send it.",
		"I. Send the agreement.",
	} {
		if !OpensWithWriterSubject(text) {
			t.Errorf("OpensWithWriterSubject(%q) = false, want the promise refused", text)
		}
	}
}

func TestAnInstructionOrAFactIsNotAPromise(t *testing.T) {
	for _, text := range []string{
		"Send them the final partnership agreement for signature once legal finishes its review.",
		"Ask them which sub-processor blocks sign-off.",
		"Iceland is their next market.",
		"Wire the deposit before Friday.",
		"Ichthys Energy signed in May.",
		"I-beam supply is their bottleneck.",
		"They asked for the agreement; I couldn't see their reply.",
		"Schicke ihnen den Vertrag.",
		"I.e. send the agreement.", // a "." between two letters stays inside the word
		"I/O bottlenecks stall their plants; lead with that.",
		"",
	} {
		if OpensWithWriterSubject(text) {
			t.Errorf("OpensWithWriterSubject(%q) = true, want it kept", text)
		}
	}
}

func TestKeepBriefRefusesAMoveWrittenAsAPromiseWithoutSpendingTheBudget(t *testing.T) {
	cite := []Evidence{{EntityType: "deal", EntityID: dealID}}
	kept := KeepBrief([]Sentence{
		{Text: "I couldn't see their mailbox.", Nature: "fact", Evidence: cite},
		{Text: "I'll send the renewal terms.", Nature: "recommendation", Evidence: cite},
		{Text: "Send the renewal terms.", Nature: "recommendation", Evidence: cite},
	}, supplied(), BriefBounds{
		Sentences: 5, Recommendations: 1,
		Natures: map[string]bool{"fact": true, "recommendation": true},
		Fact:    "fact", Recommendation: "recommendation",
	})
	if len(kept) != 2 || kept[0].Text != "I couldn't see their mailbox." || kept[1].Text != "Send the renewal terms." {
		t.Fatalf("kept %+v, want the fact and the instruction, and the promise dropped", kept)
	}
}
