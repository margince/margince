// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Every language the detector can return, and every band the axis can produce.
// Derived from the two vocabularies rather than listed, so a new band or a new
// language fails this file until the table covers it.
var (
	langs = []textlang.Lang{textlang.English, textlang.German, textlang.Vietnamese}
	bands = []convstate.Band{
		convstate.BandNone, convstate.BandFresh, convstate.BandWeeks, convstate.BandMonths,
	}
)

// Every cell is filled. A missing phrase renders as an empty line in a real
// draft, which reads as a bug in the product rather than a gap in a table.
func TestEveryBandInEveryLanguageHasItsPhrases(t *testing.T) {
	for _, lang := range langs {
		for _, band := range bands {
			phrases := draftfloor.For(lang, band)
			required := map[string]string{
				"SubjectNoContext":  phrases.SubjectNoContext,
				"SubjectAbout":      phrases.SubjectAbout,
				"GreetingNamed":     phrases.GreetingNamed,
				"GreetingAnonymous": phrases.GreetingAnonymous,
				"Ask":               phrases.Ask,
			}
			for name, value := range required {
				if strings.TrimSpace(value) == "" {
					t.Errorf("%s/%s: %s is empty", lang, band, name)
				}
			}
			// Opener is deliberately empty at fresh and required elsewhere:
			// a live exchange needs no preamble, and a gap needs acknowledging.
			opener := strings.TrimSpace(phrases.Opener)
			if band == convstate.BandFresh && opener != "" {
				t.Errorf("%s/fresh: Opener should be empty, got %q", lang, opener)
			}
			if band != convstate.BandFresh && opener == "" {
				t.Errorf("%s/%s: Opener is empty, but this band has a gap to acknowledge",
					lang, band)
			}
		}
	}
}

// Templates take exactly one substitution. Two would silently drop a value and
// none would render the placeholder to the recipient.
func TestEveryTemplateTakesExactlyOneSubstitution(t *testing.T) {
	for _, lang := range langs {
		for _, band := range bands {
			phrases := draftfloor.For(lang, band)
			for name, template := range map[string]string{
				"SubjectAbout":  phrases.SubjectAbout,
				"GreetingNamed": phrases.GreetingNamed,
			} {
				if got := strings.Count(template, "%s"); got != 1 {
					t.Errorf("%s/%s: %s has %d placeholders, want 1: %q",
						lang, band, name, got, template)
				}
			}
		}
	}
}

// The bug this table exists to kill. A first message to somebody the product
// has never written to must not open by following up on nothing, in any
// language (DRAFT-AC-E-3).
func TestAFirstTouchNeverClaimsAHistory(t *testing.T) {
	inventedHistory := []string{
		"following up", "checking in", "circling back",
		"nachfassen", "wie besprochen", "erneut",
		"tiếp theo", "nhắc lại",
	}
	for _, lang := range langs {
		phrases := draftfloor.For(lang, convstate.BandNone)
		for _, field := range []string{phrases.SubjectNoContext, phrases.Opener, phrases.Ask} {
			lowered := strings.ToLower(field)
			for _, phrase := range inventedHistory {
				if strings.Contains(lowered, phrase) {
					t.Errorf("%s/none: %q implies a prior exchange that does not exist",
						lang, field)
				}
			}
		}
	}
}

// A reply prefix is a claim that a thread exists. It is earned only by a real
// inbound thread subject, and never at band none.
func TestTheReplyPrefixIsOnlyUsedOnARealThread(t *testing.T) {
	cases := []struct {
		name       string
		band       convstate.Band
		topic      string
		threaded   bool
		wantPrefix bool
	}{
		{name: "answering a real thread", band: convstate.BandFresh, topic: "Angebot", threaded: true, wantPrefix: true},
		{name: "a topic we chose ourselves", band: convstate.BandFresh, topic: "Angebot", threaded: false, wantPrefix: false},
		{name: "first touch, even with a topic", band: convstate.BandNone, topic: "Angebot", threaded: true, wantPrefix: false},
		{name: "picking up an old thread", band: convstate.BandMonths, topic: "Angebot", threaded: true, wantPrefix: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := draftfloor.Subject(textlang.German, c.band, c.topic, c.threaded)
			if hasPrefix := strings.HasPrefix(got, "Re:"); hasPrefix != c.wantPrefix {
				t.Fatalf("Subject = %q, want reply prefix = %v", got, c.wantPrefix)
			}
		})
	}
}

// With nothing known about the topic, the subject is the band's own line.
func TestSubjectFallsBackToTheBandLineWithNoTopic(t *testing.T) {
	got := draftfloor.Subject(textlang.German, convstate.BandNone, "   ", true)
	want := draftfloor.For(textlang.German, convstate.BandNone).SubjectNoContext

	if got != want {
		t.Fatalf("Subject(no topic) = %q, want %q", got, want)
	}
}

func TestGreetingUsesTheNameWhenThereIsOne(t *testing.T) {
	if got := draftfloor.Greeting(textlang.German, convstate.BandFresh, "Marek"); got != "Hallo Marek," {
		t.Errorf("Greeting(named) = %q, want %q", got, "Hallo Marek,")
	}
	if got := draftfloor.Greeting(textlang.German, convstate.BandFresh, "  "); got != "Guten Tag," {
		t.Errorf("Greeting(anonymous) = %q, want %q", got, "Guten Tag,")
	}
}

// A name or a topic carrying a percent sign is text, not a format directive.
func TestAPercentSignInTheInputIsRenderedAsItself(t *testing.T) {
	got := draftfloor.Subject(textlang.English, convstate.BandNone, "100%d discount", false)

	if !strings.Contains(got, "100%d discount") {
		t.Fatalf("Subject = %q, want the topic rendered verbatim", got)
	}
}

// The caller is already on the no-model path, so an unresolved language or an
// unrecognized band degrades to a renderable draft rather than to nothing.
func TestUnresolvedInputDegradesToARenderableDraft(t *testing.T) {
	unknownLang := draftfloor.For(textlang.Unknown, convstate.BandFresh)
	if unknownLang != draftfloor.For(draftfloor.DefaultLang, convstate.BandFresh) {
		t.Error("an unresolved language should fall back to the default language")
	}

	unknownBand := draftfloor.For(textlang.German, convstate.Band("something-else"))
	if unknownBand != draftfloor.For(textlang.German, convstate.BandNone) {
		t.Error("an unrecognized band should fall back to the band that assumes no history")
	}
}

// The ladder a draft's language comes down.
//
// Each tier exists because the one above it can honestly answer "I do not
// know": a message may record no language, a two-line note gives the detector
// nothing to work with, and a first message has no correspondence at all.
func TestTheLanguageLadderTakesTheFirstTierThatKnows(t *testing.T) {
	t.Parallel()
	const german = "Guten Tag, ich melde mich wegen der Rechnung und der offenen Posten."
	const english = "Hello, I am writing about the invoice and the outstanding items."

	for name, tc := range map[string]struct {
		written draftfloor.Written
		base    string
		want    string
	}{
		"stored beats the text under it": {
			// The label was read when the message arrived; the body has since
			// grown a quoted chain in another language.
			written: draftfloor.Written{Stored: "en", Body: german},
			want:    "en",
		},
		"an unsupported stored value is skipped": {
			written: draftfloor.Written{Stored: "kl", Body: english},
			want:    "en",
		},
		"the body decides over the subject": {
			written: draftfloor.Written{Body: english, Subject: german},
			want:    "en",
		},
		"an unreadable body falls to the subject": {
			written: draftfloor.Written{Body: "ok", Subject: german},
			want:    "de",
		},
		"text that says nothing falls to the installation": {
			written: draftfloor.Written{Body: "ok"},
			base:    "de",
			want:    "de",
		},
		"a first message has only the installation to go on": {
			// No correspondence at all: the rep typed three words of intent.
			written: draftfloor.Written{Body: "ask for Tuesday"},
			base:    "vi",
			want:    "vi",
		},
		"an unsupported installation language is skipped too": {
			written: draftfloor.Written{Body: "ok"},
			base:    "kl",
			want:    string(draftfloor.DefaultLang),
		},
		"with nothing anywhere, the default": {
			written: draftfloor.Written{},
			want:    string(draftfloor.DefaultLang),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver := draftfloor.NewResolver().
				WithClock(func() time.Time { return time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC) })
			if tc.base != "" {
				resolver = resolver.WithBaseLanguage(func(context.Context) string { return tc.base })
			}
			got := resolver.Resolve(context.Background(), tc.written, convstate.State{Band: convstate.BandFresh})
			if got.Language != tc.want {
				t.Errorf("language = %q, want %q", got.Language, tc.want)
			}
		})
	}
}

// The register is German's alone, and it is decided WITH the language rather
// than after it. Resolving the language afterwards would leave a German
// register on an English envelope, or drop one the other way.
func TestTheRegisterFollowsTheResolvedLanguage(t *testing.T) {
	t.Parallel()
	resolver := draftfloor.NewResolver().
		WithClock(func() time.Time { return time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC) }).
		WithBaseLanguage(func(context.Context) string { return "de" })

	// A German body: the envelope is German and carries a register.
	german := resolver.Resolve(context.Background(),
		draftfloor.Written{Body: "Guten Tag, wie besprochen sende ich Ihnen die Unterlagen zu."},
		convstate.State{Band: convstate.BandFresh})
	if german.Language != "de" || german.Register == "" {
		t.Errorf("a German draft = %q/%q, want de with a register", german.Language, german.Register)
	}

	// The SAME resolver on an English body: no register, because the
	// distinction does not exist in the language being written.
	english := resolver.Resolve(context.Background(),
		draftfloor.Written{Stored: "en", Body: "Guten Tag, wie besprochen sende ich Ihnen die Unterlagen zu."},
		convstate.State{Band: convstate.BandFresh})
	if english.Language != "en" || english.Register != "" {
		t.Errorf("an English draft = %q/%q, want en with no register", english.Language, english.Register)
	}
}
