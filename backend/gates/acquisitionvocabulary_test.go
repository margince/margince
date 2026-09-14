// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Two modules spell "we do not know how this contact arrived" the same way.
//
// contacts owns contact_acquisition_evidence and defines the vocabulary;
// consent reads it to assemble the Art. 14 disclosure and cannot import a
// sibling, so it holds its own copy of the one value it needs by name.
//
// A drift here is quiet and unpleasant. The page falls back to this string when
// a contact has no evidence row, and the disclosure then reads "we cannot say
// how your details reached us" — correct. If contacts renamed its constant, the
// page would keep emitting the old token, the frontend's label map would miss
// it, and the reader would be shown a fallback sentence about a value that no
// longer exists.

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestTheUnknownAcquisitionIsSpelledTheSameOnBothSides holds the claim in
// consent/privacyinfo.go's acquiredUnknownLegacy doc comment.
func TestTheUnknownAcquisitionIsSpelledTheSameOnBothSides(t *testing.T) {
	t.Parallel()
	owner := stringConstantIn(t,
		filepath.Join(repoRoot, "backend", "internal", "modules", "contacts", "acquisition.go"),
		`AcquiredUnknownLegacy\s*=\s*"([^"]+)"`)
	mirror := stringConstantIn(t,
		filepath.Join(repoRoot, "backend", "internal", "modules", "consent", "privacyinfo.go"),
		`acquiredUnknownLegacy\s*=\s*"([^"]+)"`)
	if owner != mirror {
		t.Errorf("contacts spells the unknown acquisition %q and consent spells it %q.\n"+
			"The privacy notice falls back to consent's value when a contact has no "+
			"acquisition evidence, so a drift shows the reader a sentence about a token "+
			"the vocabulary no longer has.", owner, mirror)
	}
}

// stringConstantIn reads one string constant out of a file by pattern.
func stringConstantIn(t *testing.T, path, pattern string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	match := regexp.MustCompile(pattern).FindSubmatch(src)
	if match == nil {
		t.Fatalf("%s no longer declares a constant matching %s — this gate is reading a "+
			"shape that is gone; point it at what replaced it", path, pattern)
	}
	return string(match[1])
}
