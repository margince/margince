// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// One lock, two modules, and no import between them.
//
// people.RetractCaptureOnlyPersonTx must not read a sender override that a
// human is committing underneath it, so it takes the same advisory lock
// capture's OverrideForTx takes. A module may not import a sibling, so the key
// is spelled twice — and two spellings of one lock that drift apart do not
// fail, they simply stop excluding each other, which is the silent version of
// having no lock at all.

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestTheSenderOverrideLockIsSpelledTheSameOnBothSides derives both spellings
// from the source and fails when they stop matching.
func TestTheSenderOverrideLockIsSpelledTheSameOnBothSides(t *testing.T) {
	t.Parallel()
	const entity = `"capture_sender_override"`
	root := moduleRoot(t)
	capturedIn := readSource(t, filepath.Join(root, "internal/modules/capture/senderoverride.go"))
	peopleIn := readSource(t, filepath.Join(root, "internal/modules/people/personprivate.go"))

	for _, side := range []struct{ name, source string }{
		{"capture", capturedIn}, {"people", peopleIn},
	} {
		if !strings.Contains(side.source, "senderOverrideEntity = "+entity) {
			t.Fatalf("%s no longer names the lock entity %s. Both sides build the advisory lock key "+
				"from it, and a rename on one side alone leaves two locks that never exclude each "+
				"other — a race with nothing to fail", side.name, entity)
		}
	}
	// The identity is `<owner uuid>:<folded address>` on both sides. capture
	// builds it in senderOverrideIdentity; people builds it inline because it
	// holds the person rather than one address.
	if !strings.Contains(capturedIn, `user.String() + ":" + foldedAddress`) {
		t.Fatal("capture's sender-override lock identity changed shape. people/personprivate.go " +
			"builds the same key by hand and cannot see this change")
	}
	if !strings.Contains(peopleIn, `ownerID.String()+":"+address`) {
		t.Fatal("people's sender-override lock identity changed shape and no longer matches " +
			"capture's senderOverrideIdentity — the two stop excluding each other silently")
	}
}
