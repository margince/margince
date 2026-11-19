// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"
)

// Every served version reads back with its own words, and only the current two
// may name a new instruction.
func TestEveryServedWarningVersionReadsBackAndOnlyTheCurrentOnesAreAccepted(t *testing.T) {
	t.Parallel()
	old, known := OverrideWarningFor("override-v1")
	if !known || !strings.HasPrefix(old.Text, "The engine refused this message") {
		t.Fatalf("override-v1 read back as %+v (known=%v), want its own words", old, known)
	}
	if current, _ := OverrideWarningFor(OverrideWarningVersion); current != TheOverrideWarning() {
		t.Errorf("the served warning %+v is not the registry's %+v", TheOverrideWarning(), current)
	}
	if _, known := OverrideWarningFor("override-v0"); known {
		t.Error("a version this build never served reads back as known")
	}
	if servedWarningVersion("override-v1") {
		t.Error("a new instruction may name the retired override-v1")
	}
	for _, current := range []string{OverrideWarningVersion, InlineWarningVersion} {
		if !servedWarningVersion(current) {
			t.Errorf("a new instruction may not name the current %s", current)
		}
	}
}
