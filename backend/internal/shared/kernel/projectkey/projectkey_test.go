// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projectkey

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Shaped and the CHECK's pattern must answer alike on every token. The corpus
// walks each length from 0 to past the ceiling with every kind of character,
// so a rule one side drops shows as a disagreement.
func TestShapedAgreesWithTheChecksPattern(t *testing.T) {
	t.Parallel()
	check := regexp.MustCompile(CheckPattern)
	kinds := []string{"a", "Z", "7", "_", "-", " ", ".", "é", "[", "0"}
	for length := 0; length <= MaxLen+2; length++ {
		for _, first := range kinds {
			for _, rest := range kinds {
				token := first
				if length == 0 {
					token = ""
				} else if length > 1 {
					token += strings.Repeat(rest, length-1)
				}
				if Shaped(token) != check.MatchString(token) {
					t.Errorf("Shaped(%q) = %v, the CHECK's pattern says %v", token, Shaped(token), !Shaped(token))
				}
			}
		}
	}
}

// The pattern here must be the one the database enforces, read from the
// schema the migrations build, so the CHECK cannot change alone.
func TestThePatternIsTheOneTheDatabaseEnforces(t *testing.T) {
	t.Parallel()
	catalog, err := os.ReadFile("../../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(catalog), "\n") {
		if strings.HasPrefix(line, "public.project.project_key_shape ") {
			if !strings.Contains(line, "'"+CheckPattern+"'") {
				t.Fatalf("the database's project_key_shape is %q, which does not carry %q", line, CheckPattern)
			}
			return
		}
	}
	t.Fatal("head_catalog.txt names no project_key_shape CHECK; the census read the wrong file")
}
