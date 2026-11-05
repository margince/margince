// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The claim on normalizedPurposeKey, held rather than asserted.
//
// A purpose key arrives from a request body or a public form, so its casing and
// whitespace are the caller's, and it is then matched against
// consent_purpose.key. Two surfaces normalizing it differently resolve the same
// purpose to different rows or to none — and the preference center's
// duplicate-key check reads the same value to decide whether a withdrawal and a
// grant name one purpose, so a second spelling there lets a grant slip past the
// rule that makes suppression win.
//
// This existed as two hand-copied lines before, which is why the claim is worth
// a gate and not a sentence.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThePurposeKeyIsNormalizedInExactlyOnePlace(t *testing.T) {
	// The idiom itself, spelled as the tree spells it. Matched as text rather
	// than through the AST because the point is that nobody RETYPES these two
	// calls — a caller that reaches the same result some other way is not the
	// drift this guards against.
	const idiom = "strings.TrimSpace(strings.ToLower("

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the consent package: %v", err)
	}
	var carriers []string
	for _, e := range entries {
		// This file names the idiom to look for, so counting itself would make
		// the gate permanently red for the one occurrence that is the search
		// term rather than a spelling of the rule.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || e.Name() == "purposekey_test.go" {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for range strings.Count(string(raw), idiom) {
			carriers = append(carriers, e.Name())
		}
	}
	if len(carriers) != 1 || carriers[0] != "state.go" {
		t.Errorf("the purpose-key normalization is spelled in %v, want exactly one spelling in state.go — "+
			"a second copy resolves the same purpose to a different row, and the preference center's "+
			"duplicate-key check reads it to decide whether a withdrawal and a grant name one purpose; "+
			"call normalizedPurposeKey instead", carriers)
	}
}
