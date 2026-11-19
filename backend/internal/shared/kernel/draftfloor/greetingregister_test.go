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

// A register a rep stored on the contact decides the German form whatever the
// thread reads like, and an unset one leaves the reading of the thread alone.
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
