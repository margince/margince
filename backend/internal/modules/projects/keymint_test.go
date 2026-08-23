// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"regexp"
	"testing"
)

// The shape the project_key_shape CHECK admits. Spelled here from the
// constraint rather than imported, so a key the generator mints that the
// database would refuse fails in this package instead of at the INSERT.
var keyShape = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{1,23}$`)

func TestTheStemAProjectNameGives(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, want, why string
	}{
		{"ERP rollout Acme", "ERA", "several words give their initials"},
		{"Datenmigration", "DATENMIG", "one word gives its opening letters, capped"},
		{"S4", "S4", "a short single word is used whole"},
		{"Ärztehaus Umbau", "RU", "a leading non-ASCII letter is skipped, not transliterated"},
		{"移行プロジェクト", keyFallbackStem, "a name with no ASCII letters falls back"},
		{"— / —", keyFallbackStem, "a name of punctuation falls back"},
		{"7 Eleven Rollout", "ER", "a digit-led word cannot open a key, so the letters carry it"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := keyStem(c.name)
			if got != c.want {
				t.Fatalf("keyStem(%q) = %q, want %q — %s", c.name, got, c.want, c.why)
			}
		})
	}
}

// Every stem this generator can produce must be able to carry a number and
// still satisfy the column's CHECK. A stem that cannot is a create that fails
// at the INSERT with a constraint name, for a key the caller never chose.
func TestEveryMintedKeyFitsTheColumnsShape(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ERP rollout Acme", "Datenmigration", "S4", "Ärztehaus Umbau",
		"移行プロジェクト", "— / —", "7 Eleven Rollout", "a", "1",
		"A very long project name with a great many separate words in it",
	} {
		key := keyStem(name) + "-1"
		if !keyShape.MatchString(key) {
			t.Errorf("the key minted for %q is %q, which project_key_shape refuses", name, key)
		}
	}
}
