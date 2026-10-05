// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"context"
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
				WithClock(func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }).
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
		"the correspondence still wins over the draft and the intent": {
			written: draftfloor.Written{Body: formalMail, Rewrite: informalDraft, Purpose: informalIntent},
			want:    "Sie",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				WithClock(func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }).
				WithUserLanguage(func(context.Context) string { return "de" })
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandNone})
			if got.Language != "de" || got.Register != tc.want {
				t.Errorf("envelope = %q/%q, want de/%q", got.Language, got.Register, tc.want)
			}
		})
	}
}
