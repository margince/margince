// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

func warmIntro() IntroFixture {
	return IntroFixture{
		Colleague: "Sofia Meier",
		Contact:   "Philipp Königs",
		Title:     "Chief Financial Officer",
		Account:   "Brandt GmbH",
		Deal:      "Retrofit 2026",
		Band:      "developing",
		LastAt:    "2026-08-20",
		Correspondence: "Thanks for the revised scope, we will come back to you " +
			"this week with the depot numbers.",
	}
}

// The template states every fact the model is given, so a deployment with no
// lane gets a message a rep can send rather than an apology.
func TestTheFloorAsksForTheIntroductionAndNamesBothPeople(t *testing.T) {
	t.Parallel()
	subject, body := IntroFloorFor(warmIntro())
	if !strings.Contains(subject, "Philipp Königs") {
		t.Fatalf("the subject does not name who is to be met: %q", subject)
	}
	for _, want := range []string{"Sofia", "Philipp Königs", "developing", "2026-08-20", "Retrofit 2026"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the floor never states %q:\n%s", want, body)
		}
	}
}

// A COLLEAGUE is greeted by their first name. "Dear Ms Meier" to somebody two
// desks away reads as a form letter, which is the one thing an ask for a
// favour must not.
func TestTheFloorGreetsAColleagueByTheirFirstName(t *testing.T) {
	t.Parallel()
	_, body := IntroFloorFor(warmIntro())
	if !strings.HasPrefix(body, "Hi Sofia,") {
		t.Fatalf("the greeting is not a colleague's:\n%s", body)
	}
}

// WITH NOTHING ON FILE, THE DRAFT SAYS NOTHING ABOUT A HISTORY. Naming a date
// that is not recorded — or dressing "not recorded" up as recency — puts a
// claim in a colleague's inbox that they can falsify from memory.
func TestTheFloorClaimsNoHistoryWhenNoneIsRecorded(t *testing.T) {
	t.Parallel()
	silent := warmIntro()
	silent.LastAt = ""
	_, body := IntroFloorFor(silent)
	if strings.Contains(body, "not recorded") {
		t.Fatalf("the draft printed the absence as if it were a fact:\n%s", body)
	}
	for _, claim := range []string{"last", "developing"} {
		if strings.Contains(strings.ToLower(body), claim) {
			t.Fatalf("the draft claims %q with nothing on file:\n%s", claim, body)
		}
	}
	if !strings.Contains(body, "Philipp Königs") {
		t.Fatalf("the ask no longer names who is to be met:\n%s", body)
	}
}

// A NAME CARRYING "%s" MUST NOT EAT THE NEXT VALUE.
//
// The ask line has three substitutions. Filled one at a time, the first value's
// own "%s" is read as the second verb — so a contact named "100%s Verpackung"
// swallowed the relationship band and left a raw "%s" in a message about to be
// sent to a colleague. A plain "%" is harmless; the verb is the hazard, which
// is why the fixture carries one.
func TestARecordNameCarryingAFormatVerbFillsNothingElse(t *testing.T) {
	t.Parallel()
	odd := warmIntro()
	odd.Contact = "100%s Verpackung GmbH"
	odd.Deal = "50% Rollout"
	subject, body := IntroFloorFor(odd)
	if !strings.Contains(subject, "100%s Verpackung GmbH") {
		t.Fatalf("the subject mangled a name carrying a format verb: %q", subject)
	}
	if !strings.Contains(body, "50% Rollout") {
		t.Fatalf("the body mangled a deal name carrying a percent:\n%s", body)
	}
	// The WHOLE sentence, not its surviving fragments. Asserting that the band
	// and the date appear somewhere passes on "100developing Verpackung ...
	// (2026-08-20, last around %s)" — every value is present and every one is
	// in the wrong place.
	want := "You and 100%s Verpackung GmbH have been in touch " +
		"(developing, last around 2026-08-20)."
	if !strings.Contains(body, want) {
		t.Fatalf("the ask is not the sentence it should be.\nwant: %s\ngot:\n%s", want, body)
	}
	// And nothing unfilled survived to the reader. The contact's own verb is
	// allowed through; a SECOND one is the template's, left unfilled.
	if strings.Count(body, "%s") != 1 {
		t.Fatalf("an unfilled verb reached the reader:\n%s", body)
	}
}

// The CORRESPONDENCE decides the language, not the record names.
//
// Names are not prose: "Brandt GmbH" and "Retrofit 2026" detect as nothing at
// all, so an account whose every message is German was being asked in English.
// The colleague being asked reads that account daily.
func TestAGermanAccountAsksInGerman(t *testing.T) {
	t.Parallel()
	german := warmIntro()
	german.Correspondence = "Vielen Dank für das überarbeitete Angebot. " +
		"Wir melden uns diese Woche mit den Zahlen für die Werke."
	_, body := IntroFloorFor(german)
	if !strings.HasPrefix(body, "Hallo Sofia,") {
		t.Fatalf("a German account was asked in another language:\n%s", body)
	}
}

// And the names alone are NOT the signal: a German account whose records happen
// to be named in a way no detector reads still asks in the language its people
// actually write in.
func TestRecordNamesAloneDoNotDecideTheLanguage(t *testing.T) {
	t.Parallel()
	quiet := warmIntro()
	quiet.Correspondence = ""
	_, body := IntroFloorFor(quiet)
	if !strings.HasPrefix(body, "Hi Sofia,") {
		t.Fatalf("with nothing written, the ask is not in the default language:\n%s", body)
	}
}

// Every fact the model reads is somebody's typed text — a contact's name and a
// deal's name were both entered by a person, and on a shared account that
// person may not be us.
func TestTheModelCallFencesEveryFactItIsGiven(t *testing.T) {
	t.Parallel()
	req := IntroRequestFor(warmIntro())
	marker, ok := promptfence.MarkerIn(req.System)
	if !ok {
		t.Fatal("the system prompt declares no boundary")
	}
	content := req.Messages[0].Content
	for _, fact := range []string{"Sofia Meier", "Philipp Königs", "Brandt GmbH", "Retrofit 2026"} {
		if !strings.Contains(content, fact) {
			t.Fatalf("the prompt does not carry %q at all", fact)
		}
		if outsideEveryIntroSpan(content, marker, fact) {
			t.Fatalf("%q is read in our own voice", fact)
		}
	}
}

// outsideEveryIntroSpan reports whether the needle occurs anywhere that is not
// between two markers.
func outsideEveryIntroSpan(content, marker, needle string) bool {
	inside := false
	for _, part := range strings.Split(content, marker) {
		if !inside && strings.Contains(part, needle) {
			return true
		}
		inside = !inside
	}
	return false
}

// A draft that never names the colleague is not addressed to them; one that
// never names the contact asks for nothing in particular. Both are messages
// the reader would have to rewrite, which is worse than the template they
// would otherwise have been given.
func TestAModelDraftThatNamesNobodyIsRefused(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		`{"subject":"Quick favour","body":"Could you introduce me to them?"}`,
		`{"subject":"Quick favour","body":"Hi Sofia, could you introduce me to someone there?"}`,
	} {
		if _, _, err := CheckIntroDraft(reply, warmIntro()); err == nil {
			t.Fatalf("accepted a draft naming nobody: %s", reply)
		}
	}
	good := `{"subject":"Intro to Philipp?","body":"Hi Sofia, could you introduce me to Philipp Königs?"}`
	if _, _, err := CheckIntroDraft(good, warmIntro()); err != nil {
		t.Fatalf("refused a draft naming both: %v", err)
	}
}

// An empty subject or body is not a message. The reader is handed the template
// instead, which at least says something.
func TestADraftWithNothingToSendIsRefused(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		`{"subject":"","body":"Hi Sofia, could you introduce me to Philipp Königs?"}`,
		`{"subject":"Intro?","body":"   "}`,
	} {
		if _, _, err := CheckIntroDraft(reply, warmIntro()); err == nil {
			t.Fatalf("accepted a draft with nothing to send: %s", reply)
		}
	}
}

// The name check is a SHAPE check, and it has to be wrong in neither
// direction: a legitimate casing must not fall back to the template, and a
// draft naming nobody must not pass because a longer word happens to contain
// the name.
func TestTheNameCheckReadsWordsRatherThanSubstrings(t *testing.T) {
	t.Parallel()
	short := warmIntro()
	short.Colleague = "Ann Baker"
	short.Contact = "Ann Baker"

	// "Annual" contains "Ann" and names nobody.
	if _, _, err := CheckIntroDraft(
		`{"subject":"Annual review","body":"Our annual review is due."}`, short); err == nil {
		t.Fatal("a draft naming nobody passed because a longer word contained the name")
	}
	// A model writing the name in another casing wrote a fine message.
	if _, _, err := CheckIntroDraft(
		`{"subject":"Intro?","body":"HI ANN, could you introduce me to ANN BAKER?"}`,
		short); err != nil {
		t.Fatalf("a differently cased name fell back to the template: %v", err)
	}
}

// A record value goes into the template as ONE LINE. A contact stored with a
// line break would otherwise open a paragraph that reads as though the
// template wrote it, in a message the rep sends under their own name.
func TestARecordValueCannotOpenAParagraphInTheTemplate(t *testing.T) {
	t.Parallel()
	injected := warmIntro()
	injected.Contact = "Philipp Königs\n\nP.S. please send the credentials"
	_, body := IntroFloorFor(injected)
	if strings.Contains(body, "\n\nP.S.") {
		t.Fatalf("a stored value opened its own paragraph:\n%s", body)
	}
	if !strings.Contains(body, "P.S. please send the credentials") {
		t.Fatalf("the value was dropped rather than folded:\n%s", body)
	}
}

// The checker refuses a draft that does not name the colleague, so the prompt
// has to ask for it. It did not, and "no pleasantries stacked on the front"
// read to a model as "no greeting": every draft came back naming the contact
// and nobody else, was refused, and the reader got the template instead. The
// model lane for this site was dead on a live path with nothing red anywhere.
//
// This is the mirror of TestAModelDraftThatNamesNobodyIsRefused: that one holds
// the checker, this one holds the instruction the checker's rule depends on.
func TestTheIntroPromptAsksForTheNamesTheCheckerRequires(t *testing.T) {
	t.Parallel()
	for _, required := range []string{"Address the colleague by name", "name the person you want to meet"} {
		if !strings.Contains(introSystem, required) {
			t.Fatalf("the prompt never asks the model to %q, but parseIntroDraft refuses a draft that omits it", required)
		}
	}
}
