// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package correspondence

import "testing"

// The whole point of the package: callers in three places must arrive at the
// same key for the same address, however the sender capitalised it. Two writers
// whose keys differ by a character take two different locks and serialize
// nothing, while looking exactly like code that does.
func TestOneAddressHasOneKeyHoweverItIsWritten(t *testing.T) {
	t.Parallel()

	spellings := []string{
		"First.Last@Company.Example",
		"first.last@company.example",
		"  first.last@company.example  ",
		"FIRST.LAST@COMPANY.EXAMPLE",
	}
	want := LockIdentity(spellings[0])
	if want == "" {
		t.Fatal("the key for a real address is empty — every caller would lock the same " +
			"nothing, and every address would serialize against every other")
	}
	for _, spelling := range spellings[1:] {
		if got := LockIdentity(spelling); got != want {
			t.Errorf("LockIdentity(%q) = %q, want %q — the reader and the writer would take "+
				"two different locks and serialize nothing", spelling, got, want)
		}
	}
}

// Different addresses must NOT share a key, or one sender's write blocks every
// other sender's.
func TestTwoAddressesDoNotShareAKey(t *testing.T) {
	t.Parallel()

	if LockIdentity("one@company.example") == LockIdentity("two@company.example") {
		t.Error("two addresses folded to one key — every attested send would serialize " +
			"against every unrelated verdict")
	}
}

func TestFoldLeavesAnEmptyAddressEmpty(t *testing.T) {
	t.Parallel()

	// Callers guard on "" to mean "no address", so folding whitespace into a
	// key would lock a subject that does not exist.
	if got := Fold("   "); got != "" {
		t.Errorf("Fold(whitespace) = %q, want empty", got)
	}
}
