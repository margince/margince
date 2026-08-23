// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgdossier

// The language belongs to the cache key, and this is the case that says so.
//
// A dossier is written in the installation's base language. When an admin
// switches that setting, nothing about the company has moved — every other
// component of the key is identical — so a key that did not carry the language
// would report a hit and serve the old-language text indefinitely, until some
// unrelated fact happened to change. The setting would appear to do nothing,
// which is the exact complaint that made base language worth having.
//
// It sits beside the other entries in fingerprint_test.go for the reason that
// file's own comment gives: these are the changes that alter the answer without
// altering the company's rows.

import (
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/textlang"
)

func TestTheDossierFingerprintMovesWhenTheInstallationChangesLanguage(t *testing.T) {
	in := fourOfSeven()

	english, err := Fingerprint(in, "routing-1", string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	german, err := Fingerprint(in, "routing-1", string(textlang.German))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	if english == german {
		t.Error("the same company hashes identically in English and German, so an installation that " +
			"switched language would keep serving its old-language dossiers until an unrelated fact moved")
	}

	// The other direction, and it is not redundant: a key that hashed the
	// language into noise would move for two languages AND move for the same
	// one, so every read would rewrite and nothing would ever be a cache hit.
	againGerman, err := Fingerprint(in, "routing-1", string(textlang.German))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if againGerman != german {
		t.Error("the same company and the same language hashed differently twice: no dossier would ever be reused")
	}
}

func TestTheGrowthFitFingerprintMovesWhenTheInstallationChangesLanguage(t *testing.T) {
	in := fourOfSeven()
	offering := Offering{Confirmed: true, Fingerprint: "offering-1"}

	english, err := growthFitFingerprint(in, "routing-1", offering, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	german, err := growthFitFingerprint(in, "routing-1", offering, string(textlang.German))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	if english == german {
		t.Error("the same assessment hashes identically in English and German, so an installation that " +
			"switched language would keep serving its old-language growth fits")
	}

	againGerman, err := growthFitFingerprint(in, "routing-1", offering, string(textlang.German))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if againGerman != german {
		t.Error("the same inputs and the same language hashed differently twice: no assessment would ever be reused")
	}
}

// The two keys must not collide with each other. They cover different prompts
// over the same company, and a language component appended to both in the same
// position is exactly the kind of edit that can make two keys agree by
// accident — at which point a dossier would be served as a growth fit.
func TestTheDossierAndGrowthFitKeysStayDistinctInEveryLanguage(t *testing.T) {
	in := fourOfSeven()
	offering := Offering{Confirmed: true, Fingerprint: "offering-1"}

	for _, lang := range textlang.Shipped {
		dossier, err := Fingerprint(in, "routing-1", string(lang))
		if err != nil {
			t.Fatalf("dossier fingerprint in %q: %v", lang, err)
		}
		fit, err := growthFitFingerprint(in, "routing-1", offering, string(lang))
		if err != nil {
			t.Fatalf("growth-fit fingerprint in %q: %v", lang, err)
		}
		if dossier == fit {
			t.Errorf("the dossier and growth-fit keys collide in %q, so one surface's cached answer "+
				"could be served as the other's", lang)
		}
	}
}
