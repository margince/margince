// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// The ladder for a draft the rep asked for with a typed purpose: the
// correspondence, then the purpose, then the rep's own app language, then the
// installation's, then English. Every case runs on a German installation
// unless it says otherwise, because that is where a wrong fallback shows.
func TestThePurposeAndTheRepsLanguageSitBetweenCorrespondenceAndInstallation(t *testing.T) {
	t.Parallel()
	const germanMail = "Guten Tag, ich melde mich wegen der Rechnung und der offenen Posten."
	const englishPurpose = "Ask her whether the pilot can start in May and if she has the budget for it."
	const germanPurpose = "Frag sie, ob wir das Pilotprojekt im Mai starten können und ob sie das Budget hat."
	const shortPurpose = "pilot in May"

	for name, tc := range map[string]struct {
		written draftfloor.Written
		user    string
		base    string
		want    string
	}{
		"the contact's correspondence wins over the rep's purpose": {
			written: draftfloor.Written{Body: germanMail, Purpose: englishPurpose},
			user:    "en",
			base:    "de",
			want:    "de",
		},
		"an English purpose with no correspondence is English on a German installation": {
			written: draftfloor.Written{Purpose: englishPurpose},
			base:    "de",
			want:    "en",
		},
		"a German purpose with no correspondence is German": {
			written: draftfloor.Written{Purpose: germanPurpose},
			user:    "en",
			base:    "en",
			want:    "de",
		},
		"a purpose too short to read falls to the rep's app language": {
			written: draftfloor.Written{Purpose: shortPurpose},
			user:    "vi",
			base:    "de",
			want:    "vi",
		},
		"a rep who never chose a language falls to the installation": {
			written: draftfloor.Written{Purpose: shortPurpose},
			base:    "de",
			want:    "de",
		},
		"an unsupported app language is skipped": {
			written: draftfloor.Written{Purpose: shortPurpose},
			user:    "kl",
			base:    "de",
			want:    "de",
		},
		"a rewrite keeps the language of the draft it rewrites": {
			written: draftfloor.Written{
				Rewrite: "Hallo Anna, ich wollte fragen, ob wir das Pilotprojekt im Mai mit Ihnen starten können.",
				Purpose: "make it shorter",
			},
			user: "en",
			base: "en",
			want: "de",
		},
		"with nothing anywhere, English": {
			written: draftfloor.Written{Purpose: shortPurpose},
			want:    "en",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				SetClock(func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }).
				WithUserLanguage(func(context.Context) string { return tc.user }).
				WithBaseLanguage(func(context.Context) string { return tc.base })
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandNone})
			if got.Language != tc.want {
				t.Errorf("language = %q, want %q", got.Language, tc.want)
			}
		})
	}
}

// The du or Sie of a German draft comes from the same texts as its language,
// in the same order: correspondence, then the draft being rewritten, then the
// typed purpose.
func TestTheRegisterReadsTheSameTextsAsTheLanguage(t *testing.T) {
	t.Parallel()
	const formalMail = "Guten Tag, vielen Dank für Ihre Nachricht. Ich sende Ihnen die Unterlagen, wie Sie es wünschen."
	const informalDraft = "Hallo Jonas, ich wollte dich fragen, ob du im Mai Zeit hast. Ich schicke dir dann die Unterlagen."
	const informalIntent = "Schreib Jonas, dass ich dich gern treffen will und frag ihn, ob du Zeit hast und ob ich dir die Unterlagen schicken soll."

	for name, tc := range map[string]struct {
		written draftfloor.Written
		want    string
	}{
		"an informal intent on a first message stays informal": {
			written: draftfloor.Written{Purpose: informalIntent},
			want:    "du",
		},
		"an informal draft rewritten with no correspondence stays informal": {
			written: draftfloor.Written{Rewrite: informalDraft, Purpose: "mach es kürzer"},
			want:    "du",
		},
		"a subject is read before the draft being rewritten": {
			written: draftfloor.Written{Subject: "Wie besprochen: Sie erhalten Ihre Unterlagen", Rewrite: informalDraft},
			want:    "Sie",
		},
		"the correspondence still wins over the draft and the intent": {
			written: draftfloor.Written{Body: formalMail, Rewrite: informalDraft, Purpose: informalIntent},
			want:    "Sie",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				SetClock(func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }).
				WithUserLanguage(func(context.Context) string { return "de" })
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandNone})
			if got.Language != "de" || got.Register != tc.want {
				t.Errorf("envelope = %q/%q, want de/%q", got.Language, got.Register, tc.want)
			}
		})
	}
}

// The rep's and the installation's language are settings reads, so a draft
// whose own text decides its language makes neither.
func TestTextThatDecidesTheLanguageReadsNoSetting(t *testing.T) {
	t.Parallel()
	var reads atomic.Int32
	read := func(context.Context) string {
		reads.Add(1)
		return "vi"
	}
	resolver := draftfloor.NewResolver().WithUserLanguage(read).WithBaseLanguage(read)

	decided := resolver.Resolve(context.Background(),
		draftfloor.Written{Purpose: "Ask her whether the pilot can start in May and if she has the budget for it."},
		convstate.State{Band: convstate.BandNone})
	if decided.Language != "en" || reads.Load() != 0 {
		t.Errorf("a readable purpose = %q after %d setting reads, want en after none", decided.Language, reads.Load())
	}

	fallen := resolver.Resolve(context.Background(), draftfloor.Written{Purpose: "pilot in May"},
		convstate.State{Band: convstate.BandNone})
	if fallen.Language != "vi" || reads.Load() != 1 {
		t.Errorf("an unreadable purpose = %q after %d setting reads, want vi after one", fallen.Language, reads.Load())
	}
}

// The rest of the conversation is correspondence too: it outranks the rep's
// app language. The addressee's own text, the stored label and the answered
// message outrank it, in that order.
func TestTheThreadNamesTheLanguageWhenTheAnsweredMessageDoesNot(t *testing.T) {
	t.Parallel()
	const englishEarlier = "Thanks for the update, we plan to review the proposal with our team next week and come back to you."
	const germanEarlier = "Guten Tag, wir haben das Angebot geprüft und melden uns wegen der offenen Punkte bei Ihnen."
	const englishAnswered = "Thanks, we will review the proposal with our team and come back to you with the open points soon."
	const duThread = "Hallo Anna, ich wollte dich fragen, ob wir das Pilotprojekt im Mai mit dir starten können und ob du Zeit hast."

	for name, tc := range map[string]struct {
		written draftfloor.Written
		want    string
	}{
		"a short answered message follows the thread, not the app language": {
			written: draftfloor.Written{Body: "ok, thanks", Thread: []string{englishEarlier}},
			want:    "en",
		},
		"nothing names a language: the app language": {
			written: draftfloor.Written{Body: "ok", Thread: []string{"fine"}},
			want:    "vi",
		},
		"the addressee's own German outranks a stored English label": {
			written: draftfloor.Written{Addressee: []string{germanEarlier}, Stored: "en", Body: englishAnswered},
			want:    "de",
		},
		"an addressee text that names no language leaves the stored label": {
			written: draftfloor.Written{Addressee: []string{"ok"}, Stored: "de", Body: englishAnswered},
			want:    "de",
		},
		"the stored label wins over the thread": {
			written: draftfloor.Written{Stored: "de", Body: "ok", Thread: []string{englishEarlier}},
			want:    "de",
		},
		"a clear answered message wins over the thread": {
			written: draftfloor.Written{Body: englishAnswered, Thread: []string{germanEarlier}},
			want:    "en",
		},
		"the first thread entry that names a language decides": {
			written: draftfloor.Written{Body: "ok", Thread: []string{"fine", germanEarlier, englishEarlier}},
			want:    "de",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				WithUserLanguage(func(context.Context) string { return "vi" })
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandNone})
			if got.Language != tc.want {
				t.Errorf("language = %q, want %q", got.Language, tc.want)
			}
		})
	}

	resolver := draftfloor.NewResolver()
	du := resolver.Resolve(context.Background(), draftfloor.Written{Body: "ok", Thread: []string{duThread}}, convstate.State{Band: convstate.BandNone})
	sie := resolver.Resolve(context.Background(), draftfloor.Written{Body: "ok"}, convstate.State{Band: convstate.BandNone})
	if du.Language != "de" || du.Register == sie.Register {
		t.Errorf("a du thread gave language %q register %q, same as a bare message (%q)", du.Language, du.Register, sie.Register)
	}
}

// The subject the rep wrote for the draft names its language over everything
// the server reads for itself. A subject that names no language leaves the
// ladder below it as it was.
func TestTheRepsSubjectDecidesTheLanguageOfTheDraft(t *testing.T) {
	t.Parallel()
	const englishMail = "Thank you for the proposal. Please send the project references and the delivery options."
	const germanMail = "Hallo Anna, wir haben das Angebot geprüft und melden uns bei Ihnen wegen der offenen Punkte."
	const germanSubject = "Unsere nächsten Schritte für das gemeinsame Projekt"
	const englishSubject = "Your proposal and our next steps"

	for name, tc := range map[string]struct {
		written draftfloor.Written
		want    string
	}{
		"a German subject over English correspondence": {
			written: draftfloor.Written{Thread: []string{englishMail}, DraftSubject: germanSubject},
			want:    "de",
		},
		"an English subject over German correspondence": {
			written: draftfloor.Written{Thread: []string{germanMail}, DraftSubject: englishSubject, Purpose: "Please ask for a meeting"},
			want:    "en",
		},
		"over the addressee's own mail": {
			written: draftfloor.Written{Addressee: []string{englishMail}, DraftSubject: germanSubject},
			want:    "de",
		},
		"over a stored label": {
			written: draftfloor.Written{Stored: "de", Body: germanMail, DraftSubject: englishSubject},
			want:    "en",
		},
		"over the draft being rewritten": {
			written: draftfloor.Written{Rewrite: englishMail, Purpose: "make it shorter", DraftSubject: germanSubject},
			want:    "de",
		},
		"under the language the rep chose": {
			written: draftfloor.Written{Chosen: "vi", DraftSubject: germanSubject},
			want:    "vi",
		},
		"an empty subject leaves the correspondence": {
			written: draftfloor.Written{Thread: []string{germanMail}},
			want:    "de",
		},
		"a project tag alone leaves the correspondence": {
			written: draftfloor.Written{Thread: []string{germanMail}, DraftSubject: "[ACME-12]"},
			want:    "de",
		},
		"a name alone leaves the stored label": {
			written: draftfloor.Written{Stored: "de", DraftSubject: "Anna Schmidt"},
			want:    "de",
		},
		"a short ambiguous phrase falls to the app language": {
			written: draftfloor.Written{DraftSubject: "Pilot im Mai"},
			want:    "vi",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				WithUserLanguage(func(context.Context) string { return "vi" })
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandFresh})
			if got.Language != tc.want {
				t.Errorf("language = %q, want %q", got.Language, tc.want)
			}
		})
	}
}

// A German subject on English correspondence gives a German draft whose
// register is read from the subject. An English draft carries no register.
func TestASubjectThatChoosesGermanCarriesACoherentRegister(t *testing.T) {
	t.Parallel()
	resolver := draftfloor.NewResolver()
	informal := resolver.Resolve(context.Background(), draftfloor.Written{
		Thread:       []string{"Thanks, we will review the proposal with our team and come back to you soon."},
		DraftSubject: "Hast du Zeit? Ich schicke dir unsere nächsten Schritte",
	}, convstate.State{Band: convstate.BandFresh})
	if informal.Language != "de" || informal.Register != "du" {
		t.Errorf("envelope = %q/%q, want de/du", informal.Language, informal.Register)
	}

	english := resolver.Resolve(context.Background(), draftfloor.Written{
		Thread:       []string{"Hallo Jonas, ich wollte dich fragen, ob du im Mai Zeit hast und ob ich dir die Unterlagen schicken soll."},
		DraftSubject: "Your proposal and our next steps",
	}, convstate.State{Band: convstate.BandFresh})
	if english.Language != "en" || english.Register != "" {
		t.Errorf("envelope = %q/%q, want en with no register", english.Language, english.Register)
	}
}

// A language the rep chose outranks every tier the server reads for itself, and
// a German choice still carries a register.
func TestAChosenLanguageOutranksEveryDerivedTier(t *testing.T) {
	t.Parallel()
	const germanMail = "Guten Tag, wir haben das Angebot geprüft und melden uns wegen der offenen Punkte bei Ihnen."
	const englishThread = "Thanks for the update, we plan to review the proposal with our team next week and come back to you."

	for name, tc := range map[string]struct {
		written draftfloor.Written
		want    string
	}{
		"over the addressee's own German": {
			written: draftfloor.Written{Chosen: "vi", Addressee: []string{germanMail}},
			want:    "vi",
		},
		"over a stored German label": {
			written: draftfloor.Written{Chosen: "en", Stored: "de", Body: germanMail},
			want:    "en",
		},
		"over an English thread": {
			written: draftfloor.Written{Chosen: "vi", Body: englishThread, Thread: []string{englishThread}},
			want:    "vi",
		},
		"over the draft being rewritten and the typed purpose": {
			written: draftfloor.Written{Chosen: "de", Rewrite: englishThread, Purpose: englishThread},
			want:    "de",
		},
		"a code the product does not write is no choice": {
			written: draftfloor.Written{Chosen: "fr", Body: germanMail},
			want:    "de",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				WithUserLanguage(func(context.Context) string { return "en" }).
				WithBaseLanguage(func(context.Context) string { return "en" })
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandNone})
			if got.Language != tc.want {
				t.Errorf("language = %q, want %q", got.Language, tc.want)
			}
		})
	}

	german := draftfloor.NewResolver().Resolve(context.Background(),
		draftfloor.Written{Chosen: "de", Body: englishThread}, convstate.State{Band: convstate.BandNone})
	if german.Language != "de" || german.Register != "Sie" {
		t.Errorf("German chosen on an English thread = %q/%q, want de/Sie", german.Language, german.Register)
	}
}
