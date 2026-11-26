// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAStoredEarlierWordingReadsAsThePhraseInItsOwnLanguage(t *testing.T) {
	code := "booking.lets_meet"
	definition, _ := definitionOf(code)
	for _, language := range textlang.Shipped {
		if len(definition.Retired[language]) == 0 {
			t.Fatalf("%s keeps no earlier %s wording to read", code, language)
		}
		for _, stored := range definition.Retired[language] {
			if !Says(code, language, stored) || Reworded(code, stored) != Copy(code, language) {
				t.Errorf("%s %q did not read as today's %q", language, stored, Copy(code, language))
			}
		}
	}
	if Says(code, textlang.German, "Let's meet") || Reworded(code, "My own title") != "My own title" {
		t.Error("an earlier wording matched outside its language, or a member's title was reworded")
	}
}

// A kept wording that some language shows today lets Reworded move a title into the wrong language.
func TestEveryRetiredWordingIsNoLongerShown(t *testing.T) {
	for code, definition := range catalog {
		for language, retired := range definition.Retired {
			if !slices.Contains(textlang.Shipped, language) || len(retired) == 0 {
				t.Errorf("%s keeps wordings for %q, which ships no earlier wording", code, language)
			}
			for _, shipped := range textlang.Shipped {
				if slices.Contains(retired, Copy(code, shipped)) {
					t.Errorf("%s keeps %q as earlier wording, but %s shows it today", code, Copy(code, shipped), shipped)
				}
			}
		}
	}
}

func TestTextStoredInAnEarlierEnglishWordingRecoversToItsCodeAndShowsTodaysWords(t *testing.T) {
	kept := 0
	for code, definition := range catalog {
		for _, retired := range definition.Retired[textlang.English] {
			kept++
			component, values, recoverable := sampleSentence(t, code, definition)
			stored := retired
			if len(values) > 0 {
				stored = fmt.Sprintf(retired, values...)
			}
			bound, err := Bind(&Message{Subject: component}, stored, "")
			if err != nil || bound == nil || bound.Subject == nil {
				t.Errorf("%s does not bind its earlier wording %q: %v", code, stored, err)
				continue
			}
			if shown := bound.Subject.Display(stored, textlang.English); shown == nil || *shown != Canonical(component, textlang.English) {
				t.Errorf("%s stored as %q shows %v, not today's words", code, stored, shown)
			}
			if !recoverable {
				continue
			}
			recovered, err := RecoverCanonicalText(stored)
			if err != nil || recovered == nil || recovered.Code != code {
				t.Errorf("%q recovered as %+v, not %s: %v", stored, recovered, code, err)
			}
		}
	}
	if kept == 0 {
		t.Fatal("no code keeps an earlier English wording, so this test reads nothing")
	}
}

// sampleSentence fills code's arguments and gives the values they print, and
// reports whether recovery can read them back: it reads text, labels and counts.
func sampleSentence(t *testing.T, code string, definition Definition) (*Component, []any, bool) {
	t.Helper()
	component, recoverable := Sentence(code), true
	var values []any
	for _, kind := range definition.Arguments {
		switch kind {
		case argumentText:
			component.Args, values = append(component.Args, Text("Example")), append(values, "Example")
		case argumentLabel:
			component.Args, values = append(component.Args, Label("Lena")), append(values, "Lena")
		case argumentCount:
			component.Args, values = append(component.Args, Count(2)), append(values, int64(2))
		case argumentDate:
			component.Args, values = append(component.Args, Date("2026-11-09")), append(values, "2026-11-09")
			recoverable = false
		default:
			t.Fatalf("%s keeps an earlier wording with an argument this test cannot fill: %s", code, kind)
		}
	}
	return component, values, recoverable
}
