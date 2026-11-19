// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
)

// A stored register decides the German form whatever the thread reads like.
// An unset one leaves the reading of the thread alone.
func TestAStoredGreetingRegisterOutranksTheThread(t *testing.T) {
	t.Parallel()
	const familiarThread = "Hallo Jonas, ich wollte dich fragen, ob du im Mai Zeit hast. Ich schicke dir dann die Unterlagen."
	const formalThread = "Guten Tag, vielen Dank für Ihre Nachricht. Ich sende Ihnen die Unterlagen, wie Sie es wünschen."

	for name, tc := range map[string]struct {
		written      draftfloor.Written
		wantRegister string
		wantStored   string
	}{
		"formal wins over a thread that reads familiar": {
			written:      draftfloor.Written{Body: familiarThread, Greeting: greetingregister.Formal},
			wantRegister: "Sie", wantStored: "formal",
		},
		"familiar wins over a thread that reads formal": {
			written:      draftfloor.Written{Body: formalThread, Greeting: greetingregister.Familiar},
			wantRegister: "du", wantStored: "familiar",
		},
		"unset leaves a familiar thread familiar": {
			written:      draftfloor.Written{Body: familiarThread},
			wantRegister: "du",
		},
		"unset leaves a formal thread formal": {
			written:      draftfloor.Written{Body: formalThread},
			wantRegister: "Sie",
		},
		"a value this build does not know is ignored": {
			written:      draftfloor.Written{Body: familiarThread, Greeting: "casual"},
			wantRegister: "du",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := draftfloor.NewResolver().
				SetClock(func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }).
				Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandNone})
			if got.Register != tc.wantRegister || got.GreetingRegister != tc.wantStored {
				t.Errorf("register/greeting_register = %q/%q, want %q/%q",
					got.Register, got.GreetingRegister, tc.wantRegister, tc.wantStored)
			}
		})
	}
}

// The stored register is not a German distinction: an English draft carries it
// too, and carries no du or Sie.
func TestAStoredGreetingRegisterReachesANonGermanDraft(t *testing.T) {
	t.Parallel()
	got := draftfloor.NewResolver().
		SetClock(func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }).
		Resolve(context.Background(), draftfloor.Written{
			Chosen:   "en",
			Greeting: greetingregister.Formal,
		}, convstate.State{Band: convstate.BandNone})
	if got.Language != "en" || got.Register != "" || got.GreetingRegister != "formal" {
		t.Errorf("envelope = %q/%q/%q, want en, no register, formal", got.Language, got.Register, got.GreetingRegister)
	}
}

// A stored register shapes the no-model greeting in every language: formal names
// the surname beside the first name, familiar names the first name only.
func TestTheFallbackGreetingHonoursTheStoredRegisterInEveryLanguage(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		lang, register, first, last, want string
	}{
		"en formal":                  {"en", "formal", "Dietmar", "Rietsch", "Dear Dietmar Rietsch,"},
		"en familiar":                {"en", "familiar", "Dietmar", "Rietsch", "Hi Dietmar,"},
		"de formal":                  {"de", "formal", "Dietmar", "Rietsch", "Guten Tag Dietmar Rietsch,"},
		"de familiar":                {"de", "familiar", "Dietmar", "Rietsch", "Hallo Dietmar,"},
		"vi formal":                  {"vi", "formal", "Dũng", "Nguyễn", "Kính gửi Dũng Nguyễn,"},
		"vi familiar":                {"vi", "familiar", "Dũng", "Nguyễn", "Chào Dũng,"},
		"formal without a surname":   {"en", "formal", "Dietmar", "", "Hi Dietmar,"},
		"unset English stays casual": {"en", "", "Dietmar", "Rietsch", "Hi Dietmar,"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			envelope := draftfloor.Envelope{Language: tc.lang, GreetingRegister: tc.register}
			if got := envelope.Greeting(tc.first, tc.last); got != tc.want {
				t.Errorf("Greeting = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAFamiliarRegisterBeatsAGermanSieThreadInTheFallbackGreeting(t *testing.T) {
	t.Parallel()
	envelope := draftfloor.NewEnvelopeWithRegister("de", "Sie", convstate.State{Band: convstate.BandFresh},
		time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "", "")
	envelope.GreetingRegister = "familiar"
	if got := envelope.Greeting("Dietmar", "Rietsch"); got != "Hallo Dietmar," {
		t.Errorf("Greeting = %q, want the familiar form", got)
	}
}
