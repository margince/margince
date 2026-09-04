// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// The language belongs to the cache key.
//
// A brief is written in the installation's language. When an admin switches
// that setting nothing about the relationship has moved — every other component
// of the key is identical — so a key that did not carry the language would
// report a hit and serve the old-language brief indefinitely, until some
// unrelated fact happened to change. The setting would appear to do nothing.
//
// The brief is cached per READER, and that is about permissions rather than
// preference: it is assembled from records that reader may see, so two people
// with different access get different facts. Language is not a permission, so
// it does not follow the reader — it follows the installation, like every other
// AI surface.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheBriefFingerprintMovesWhenTheInstallationChangesLanguage(t *testing.T) {
	t.Parallel()
	in := inputFixture()

	english, err := Fingerprint(in, "routing-1", string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	german, err := Fingerprint(in, "routing-1", string(textlang.German))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if english == german {
		t.Error("the same relationship hashes identically in English and German, so an installation " +
			"that switched language would keep serving its old-language briefs until an unrelated fact moved")
	}

	// The other direction, and it is not redundant: a key that hashed the
	// language into noise would move for two languages AND move for the same
	// one, so nothing would ever be a cache hit and every read would rewrite.
	again, err := Fingerprint(in, "routing-1", string(textlang.German))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if again != german {
		t.Error("the same relationship and the same language hashed differently twice: " +
			"no brief would ever be reused")
	}
}

// Every shipped language must key distinctly. Two that collided would serve one
// language's brief under the other's, which is the failure the whole component
// exists to prevent — and a fold that lowercased or truncated the code would
// produce exactly that for some pair without failing the two-language case
// above.
func TestEveryShippedLanguageKeysTheBriefDistinctly(t *testing.T) {
	t.Parallel()
	in := inputFixture()
	seen := map[string]string{}
	for _, lang := range textlang.Shipped {
		key, err := Fingerprint(in, "routing-1", string(lang))
		if err != nil {
			t.Fatalf("fingerprint in %q: %v", lang, err)
		}
		if other, clash := seen[key]; clash {
			t.Errorf("languages %q and %q share a cache key, so one's brief would be served as the other's",
				other, lang)
		}
		seen[key] = string(lang)
	}
}

// Re-pointing the model lane rewrites briefs rather than leaving text
// attributed to a model that no longer writes it.
func TestTheBriefFingerprintMovesWithTheModelRouting(t *testing.T) {
	t.Parallel()
	in := inputFixture()
	before, err := Fingerprint(in, "routing-1", string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	after, err := Fingerprint(in, "routing-2", string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if before == after {
		t.Error("the key ignored the routing version, so a re-pointed lane would keep serving " +
			"text the old binding wrote")
	}
}
