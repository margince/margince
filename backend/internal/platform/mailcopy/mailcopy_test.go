// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

// The catalog's own two decisions: which language a message is written in, and
// what happens to one this build has no copy for.
//
// The CONTENT is held elsewhere — TestEveryMailLabelMatchesTheScreenThatShowsIt
// compares the weekly labels against the panel's, which is the only thing that
// can say whether a translation is the right one. What is here is the lookup.

import "testing"

func TestALanguageThisBuildCarriesIsWrittenInIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		language string
		subject  string
	}{
		{language: "en", subject: "Reset your Margince password"},
		{language: "de", subject: "Margince-Passwort zurücksetzen"},
		{language: "vi", subject: "Đặt lại mật khẩu Margince"},
	} {
		t.Run(c.language, func(t *testing.T) {
			t.Parallel()
			if got := For(c.language).ResetSubject; got != c.subject {
				t.Errorf("For(%q).ResetSubject = %q, want %q", c.language, got, c.subject)
			}
			if !Known(c.language) {
				t.Errorf("Known(%q) = false for a language the catalog carries", c.language)
			}
		})
	}
}

// A language this build cannot write is answered in the fallback rather than
// refused: a password link that does not arrive is worse for its reader than an
// English one. Known says so where For cannot — For answers for everything,
// which is what makes it safe to call.
func TestALanguageThisBuildDoesNotCarryFallsBackAndSaysSo(t *testing.T) {
	t.Parallel()
	for _, language := range []string{"fr", "", "EN", "en-GB", "zz"} {
		t.Run(language, func(t *testing.T) {
			t.Parallel()
			if Known(language) {
				t.Errorf("Known(%q) = true for a language the catalog does not carry", language)
			}
			if got := For(language); got != For(string(Fallback)) {
				t.Errorf("For(%q) is not the fallback copy", language)
			}
		})
	}
}

// Every entry is filled in. The gate over the contract holds this for the
// languages the API admits; this holds it for the catalog as it stands, so a
// half-written entry fails here too — beside the data it is about.
func TestEveryEntryIsFilledIn(t *testing.T) {
	t.Parallel()
	for language := range catalog {
		t.Run(string(language), func(t *testing.T) {
			t.Parallel()
			words := For(string(language))
			for name, value := range map[string]string{
				"ResetSubject": words.ResetSubject, "ResetIntro": words.ResetIntro,
				"ResetAction": words.ResetAction, "ResetIgnore": words.ResetIgnore,
				"InviteSubject": words.InviteSubject, "InviteIntro": words.InviteIntro,
				"InviteAction": words.InviteAction, "InviteIgnore": words.InviteIgnore,
				"WeeklySubject": words.WeeklySubject, "WeeklyHeading": words.WeeklyHeading,
				"WeeklyOfDue": words.WeeklyOfDue, "WeeklyAndMore": words.WeeklyAndMore,
			} {
				if value == "" {
					t.Errorf("%s is empty in the %s copy", name, language)
				}
			}
		})
	}
}
