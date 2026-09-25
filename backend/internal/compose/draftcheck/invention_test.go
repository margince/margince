// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The draft a rep was actually served from a company page, verbatim.
//
// The account had a live support thread and no call of any kind. Every sentence
// of substance here is invented: no meeting happened, the recipient raised no
// challenges, and no record describes an operational setup or any technical
// constraint. It scored zero findings at all four bands, which is what this
// test exists to keep from happening again.
const servedInventedDraft = "Marine, it was a pleasure connecting earlier this week. " +
	"I have been giving some thought to the challenges you mentioned regarding your " +
	"current operational setup and believe there may be some targeted ways to reduce " +
	"those technical constraints.\n\n" +
	"Would you be open to a brief call to explore how we could identify potential " +
	"efficiency gains for your team?"

// A claim about the world is wrong at every band. The gap that let this through
// was band gating: assumed-memory ran only at weeks and months, invention only
// at band none, and a thread two days old sits at fresh, where neither ran.
func TestTheServedInventedDraftIsRefusedAtEveryBand(t *testing.T) {
	for _, band := range []convstate.Band{
		convstate.BandNone, convstate.BandFresh,
		convstate.BandWeeks, convstate.BandMonths,
	} {
		t.Run(string(band), func(t *testing.T) {
			findings := Body(servedInventedDraft, textlang.English, band, Grounds{})
			if len(findings) == 0 {
				t.Fatalf("the served draft passed clean at band %s", band)
			}
			if !hasRule(findings, RuleInventedConversation) {
				t.Errorf("the invented call was not caught at band %s: %+v", band, findings)
			}
			if !hasRule(findings, RuleAttributedClaim) {
				t.Errorf("the invented attribution was not caught at band %s: %+v", band, findings)
			}
		})
	}
}

// A rep reads the correction, so it has to name the phrase back. A finding that
// says only "something is wrong" leaves the model guessing on the retry.
func TestTheCorrectionNamesTheInventedPhrase(t *testing.T) {
	feedback := Feedback(Body(servedInventedDraft, textlang.English, convstate.BandFresh, Grounds{}))

	if !strings.Contains(feedback, "pleasure connecting") {
		t.Errorf("the correction did not name the invented meeting: %q", feedback)
	}
	if !strings.Contains(feedback, "you mentioned") {
		t.Errorf("the correction did not name the invented attribution: %q", feedback)
	}
}

// The lists must not swallow the ordinary drafts they sit beside. A message
// that asks for a call, or names a topic without attributing it, is the correct
// output and has to stay clean at the band a live thread sits in.
func TestAnHonestDraftStaysClean(t *testing.T) {
	for name, body := range map[string]string{
		"asks for a call without claiming one happened": "Marine, the pricing question on " +
			"the thread is easiest to answer live. Would a short call this week suit you?",
		"names the topic without attributing it": "Marine, on the question about the " +
			"August deadline: we can hold the current terms until the end of the month.",
		"refers to a message rather than a conversation": "Marine, following the note about " +
			"the provider comparison, here is what our side can commit to.",
	} {
		t.Run(name, func(t *testing.T) {
			if findings := Body(body, textlang.English, convstate.BandFresh, Grounds{}); len(findings) > 0 {
				t.Fatalf("an honest draft was refused: %+v", findings)
			}
		})
	}
}

// A reply is written FROM the counterparty's own message. If they wrote "as I
// mentioned on our call", answering the call they named is grounded in text the
// drafter can see — so the world-claim rules stand down on a threaded draft and
// hold on one that opens a new conversation.
func TestAThreadedReplyMayAnswerACallTheCounterpartyNamed(t *testing.T) {
	body := "Thanks — after our call I pulled the figures you mentioned, and the " +
		"August timeline still works on our side."

	if findings := Body(body, textlang.English, convstate.BandFresh, Grounds{Threaded: true}); len(findings) > 0 {
		t.Fatalf("a grounded reply was refused: %+v", findings)
	}
	if findings := Body(body, textlang.English, convstate.BandFresh, Grounds{}); len(findings) == 0 {
		t.Fatal("the same words opening a new conversation must still be refused")
	}
}

// The lists must assert a COMPLETED exchange on their own. A draft proposing a
// call, or naming one still to come, is the output this product exists to write
// — refusing it would be worse than the defect the lists were added for.
func TestForwardLookingAndUnrelatedTextIsNotAnInventedCall(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		lang textlang.Lang
	}{
		"proposes a call": {"It would be great to connect next week.", textlang.English},
		// Note what this one does NOT say. It offers to cover the topic on a
		// call; it does not assert that a call is booked for a named day.
		// "our call tomorrow" with nothing on the record is refused by
		// unscheduled-arrangement, which is a different rule and a real defect
		// — a customer reads it as an appointment they agreed to.
		"names a call still to come": {"We can cover that on a call.", textlang.English},
		"connects two contacts":      {"Good to connect you with Anna, who runs delivery.", textlang.English},
		"asks for a call":            {"Shall we set up a call?", textlang.English},
		"proposes a German call":     {"Es freut mich, dass wir nächste Woche sprechen können.", textlang.German},
		"a German call to come":      {"Unser Telefonat nächsten Dienstag passt mir gut.", textlang.German},
	} {
		t.Run(name, func(t *testing.T) {
			if findings := Body(tc.body, tc.lang, convstate.BandFresh, Grounds{}); len(findings) > 0 {
				t.Fatalf("a legitimate draft was refused: %+v", findings)
			}
		})
	}
}

// German drafts fail the same way in their own words, and a list that only
// knows English lets every German draft through — the failure mode the wellbeing
// list already learned once.
func TestGermanInventionIsCaught(t *testing.T) {
	for name, body := range map[string]string{
		"an invented call":        "Marine, es freute mich sehr, letzte Woche mit Ihnen zu sprechen.",
		"an invented attribution": "Marine, Sie erwähnten den Zeitplan für August.",
		"after a phone call":      "Marine, nach unserem Telefonat habe ich die Zahlen geprüft.",
	} {
		t.Run(name, func(t *testing.T) {
			if findings := Body(body, textlang.German, convstate.BandFresh, Grounds{}); len(findings) == 0 {
				t.Fatalf("a German invention passed clean")
			}
		})
	}
}

func hasRule(findings []Finding, rule Rule) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// The caller knows whether a meeting happened, and the input does not. An intent
// naming one grounds "it was a pleasure meeting you"; an intent that only asks
// for a call leaves the same sentence invented.
func TestAMeetingIsSayableOnlyWhenTheIntentNamesIt(t *testing.T) {
	body := "Hello,\n\nIt was a pleasure meeting you at the trade fair. Would a short call next month suit you?"

	named := Grounds{Met: IntentNamesMeeting("Introduce ourselves after meeting at the trade fair and ask for a call.")}
	if findings := Body(body, textlang.English, convstate.BandFresh, named); len(findings) > 0 {
		t.Fatalf("a meeting the caller named was refused: %+v", findings)
	}
	unnamed := Grounds{Met: IntentNamesMeeting("Introduce ourselves and ask for a short call.")}
	if findings := Body(body, textlang.English, convstate.BandFresh, unnamed); len(findings) == 0 {
		t.Fatal("a meeting the intent never named must still be refused")
	}
}

// Only a PAST encounter grounds the claim. An intent proposing a meeting names
// one that has not happened, and reading it as met would ground the invention.
func TestOnlyAnEncounterThatHappenedCounts(t *testing.T) {
	for intent, want := range map[string]bool{
		"Introduce ourselves after meeting at the trade fair": true,
		"We met at the conference last week; ask for a demo":  true,
		"Wir haben uns auf der Messe kennengelernt":           true,
		"Nach unserem Gespräch auf der Konferenz nachhaken":   true,
		"Ask for a short call":                                false,
		"Propose a meeting next week":                         false,
		"Ein Treffen auf der Messe vorschlagen":               false,
		"Mich kurz vorstellen und ein Gespräch dazu anbieten": false,
		"Wir haben uns auf der Messe getroffen":               true,
		"We met at the fair and have not spoken since":        true,
		"We have not met yet; introduce ourselves":            false,
		"We haven't met, so introduce ourselves":              false,
		"Wir haben noch nicht gesprochen":                     false,
		"Wir haben uns noch nicht kennengelernt":              false,
		"Wir haben eine Entscheidung getroffen":               false,
	} {
		if got := IntentNamesMeeting(intent); got != want {
			t.Errorf("IntentNamesMeeting(%q) = %v, want %v", intent, got, want)
		}
	}
}

// A named meeting grounds the ENCOUNTER, not every conversation claim: the
// call the intent never mentioned is as invented as it was without it.
func TestAMeetingTheIntentNamesGroundsNoCall(t *testing.T) {
	met := Grounds{Met: IntentNamesMeeting("We met at the trade fair; ask for a demo")}
	for name, tc := range map[string]struct {
		body    string
		lang    textlang.Lang
		refused bool
	}{
		"the meeting":           {"Hello,\n\nIt was a pleasure meeting you at the fair. Would a demo suit you?", textlang.English, false},
		"a call nobody named":   {"Hello,\n\nAfter our call I put the figures together. Would a demo suit you?", textlang.English, true},
		"the German meeting":    {"Hallo,\n\nes freute mich, Sie kennenzulernen. Passt Ihnen eine Demo?", textlang.German, false},
		"a German conversation": {"Hallo,\n\nes freute mich sehr, letzte Woche mit Ihnen zu sprechen.", textlang.German, true},
	} {
		t.Run(name, func(t *testing.T) {
			findings := Body(tc.body, tc.lang, convstate.BandFresh, met)
			if got := hasRule(findings, RuleInventedConversation); got != tc.refused {
				t.Fatalf("refused = %v, want %v: %+v", got, tc.refused, findings)
			}
		})
	}
}

// A chip labelled with the rep's own purpose trips the introduction stem, and
// the correction has to say the fault was in the LABEL: told only "do not write
// vorstell", the retry strips the sender's self-introduction from a good body.
func TestALabelFindingIsCorrectedAsALabel(t *testing.T) {
	findings := Reasoning([]string{"Mich kurz vorstellen"}, textlang.German, convstate.BandNone)
	if len(findings) == 0 || !findings[0].InLabel {
		t.Fatalf("the chip finding should be marked as a label finding, got %+v", findings)
	}
	feedback := Feedback(findings)
	for _, want := range []string{"reasoning label", "the body may still"} {
		if !strings.Contains(feedback, want) {
			t.Errorf("label feedback should say %q:\n%s", want, feedback)
		}
	}
	if body := Feedback(Body("Hello,\n\nI hope you are doing well.", textlang.English, convstate.BandFresh, Grounds{})); strings.Contains(body, "reasoning label") {
		t.Errorf("a body finding must not be corrected as a label:\n%s", body)
	}
}
