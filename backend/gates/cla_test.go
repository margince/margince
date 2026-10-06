// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// An acceptance of the CLA covers the text it was shown, under the version that
// text carries. Three places spell that version — CLA.md's header, the sentence
// a contributor posts, and the signatures file their acceptance is written to —
// and the text itself is pinned per version, so it cannot change under a version
// somebody has already accepted.

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"testing"
)

// acceptedTexts maps a CLA version to the SHA-256 of CLA.md at that version.
// A new text is a new version: add an entry here, never rewrite one.
//
// gatekit:fixture the digest of each accepted CLA text
var acceptedTexts = map[string]string{
	"1.0": "abea8ca73c954fd1a8a45d5b99a3156d877edf14a729c1f2d475a2b6c912e0cb",
}

var (
	claVersionHeader = regexp.MustCompile(`(?m)^\*\*Version (\d+\.\d+),`)
	signaturesPath   = regexp.MustCompile(`(?m)^\s*path-to-signatures:\s*(\S+)\s*$`)
	signSentence     = regexp.MustCompile(`(?m)^\s*custom-pr-sign-comment:\s*(.+?)\s*$`)
)

func TestTheCLATextIsPinnedToItsVersion(t *testing.T) {
	t.Parallel()
	text := readRepoFile(t, filepath.Join(repoRoot, "CLA.md"))
	version := claVersion(t, text)

	digest := sha256.Sum256([]byte(text))
	got := hex.EncodeToString(digest[:])
	want, known := acceptedTexts[version]
	if !known {
		t.Fatalf("CLA.md is version %s, which acceptedTexts does not pin; add it with digest %s", version, got)
	}
	if got != want {
		t.Errorf("CLA.md changed under version %s, which contributors may already have accepted.\n"+
			"A new text is a new version: raise the version in CLA.md, pin it here (digest %s), and move "+
			"the sign sentence and signatures file in .github/workflows/cla.yml with it.", version, got)
	}
}

func TestTheCLAWorkflowRecordsTheVersionCLAMdCarries(t *testing.T) {
	t.Parallel()
	version := claVersion(t, readRepoFile(t, filepath.Join(repoRoot, "CLA.md")))
	workflow := readRepoFile(t, filepath.Join(repoRoot, ".github/workflows/cla.yml"))

	wantPath := "signatures/v" + version + ".json"
	if got := soleMatch(t, signaturesPath, workflow, "path-to-signatures"); got != wantPath {
		t.Errorf("cla.yml writes acceptances to %s, but CLA.md is version %s: want %s", got, version, wantPath)
	}
	wantSentence := "I have read the Margince Contributor License Agreement version " + version +
		" and I accept its terms."
	if got := soleMatch(t, signSentence, workflow, "custom-pr-sign-comment"); got != wantSentence {
		t.Errorf("cla.yml asks contributors to post %q, but CLA.md is version %s: want %q", got, version, wantSentence)
	}
}

func claVersion(t *testing.T, text string) string {
	t.Helper()
	m := claVersionHeader.FindStringSubmatch(text)
	if m == nil {
		t.Fatal("CLA.md has no **Version N.N, header line, so no acceptance can say which text it covers")
	}
	return m[1]
}

func soleMatch(t *testing.T, pattern *regexp.Regexp, text, key string) string {
	t.Helper()
	matches := pattern.FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		t.Fatalf("cla.yml sets %s %d times; the gate reads exactly one", key, len(matches))
	}
	return matches[0][1]
}
