// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The base language is what a model is told to write in when what it writes is
// read by the whole team. These pin the two properties a caller depends on: it
// refuses a language the product cannot render, and an installation that never
// named one still resolves to something.
//
// They go through ValidateJSON rather than the validator directly, because that
// is the door the write path uses — a validator that is right and unwired would
// pass a narrower test.

// languageJSON encodes a candidate the way a settings write carries it.
func languageJSON(t *testing.T, code string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(code)
	if err != nil {
		t.Fatalf("encoding %q: %v", code, err)
	}
	return raw
}

func TestBaseLanguageAcceptsEveryShippedLanguage(t *testing.T) {
	// Driven off textlang.Shipped rather than a hand-written list: a fourth
	// language added to the product must not need this test edited to pass, and
	// when it does fail, it names the language nobody wired.
	for _, lang := range textlang.Shipped {
		if err := BaseLanguage.ValidateJSON(languageJSON(t, string(lang))); err != nil {
			t.Errorf("language %q is shipped but the setting refuses it: %v", lang, err)
		}
	}
}

func TestBaseLanguageRefusesWhatTheProductCannotWrite(t *testing.T) {
	// Each of these is a plausible thing to type or send, and each would leave
	// the installation naming a language no catalog and no prompt can produce.
	for _, bad := range []string{
		"",      // never chosen
		"EN",    // the right language, the wrong case
		"en-GB", // a BCP-47 tag rather than the bare code
		"fr",    // a language the product does not speak
		"xx",
	} {
		err := BaseLanguage.ValidateJSON(languageJSON(t, bad))
		if err == nil {
			t.Errorf("language %q was accepted; the product cannot write it", bad)
			continue
		}
		// The refusal has to say what WOULD work. One that only says "no"
		// leaves the operator guessing at the spelling.
		if !strings.Contains(err.Error(), "en, de, vi") {
			t.Errorf("refusing %q said %q, which does not name the languages that would work", bad, err)
		}
	}
}

func TestBaseLanguageDefaultsToALanguageTheProductSpeaks(t *testing.T) {
	// The default is what an installation bootstrapped before this setting
	// existed reads, because BaseLanguageOf falls back instead of refusing. A
	// default its own validator would reject would make every such
	// installation unwritable through the settings screen.
	def, err := BaseLanguage.DefaultJSON()
	if err != nil {
		t.Fatalf("encoding the registered default: %v", err)
	}
	if err := BaseLanguage.ValidateJSON(def); err != nil {
		t.Fatalf("the registered default %s does not pass this setting's own validator: %v", def, err)
	}
	if want := languageJSON(t, string(textlang.English)); string(def) != string(want) {
		t.Errorf("default is %s, expected %s — an installation that never chose reads the language its prompts were already written in", def, want)
	}
}
