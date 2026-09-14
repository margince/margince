// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The two halves of the requirement seam describe the same thing.
//
// consent owns the jurisdiction packs and answers what a composed message still
// owes; comms holds the message on its way to the provider and parks it on a
// finding. They are siblings and neither may import the other, so each declares
// its own RequirementFinding and the composition root converts between them.
//
// A drift here is quieter than most. The adapter names the fields it copies, so
// a field added on one side keeps compiling and is simply dropped — and what is
// dropped is the description of an obligation a message failed to meet, so the
// delivery still parks and the operator reading it is told less about why than
// the checker knew.

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestTheRequirementFindingAgreesAcrossTheSeam holds the claim in
// consent/requirements.go's RequirementFinding doc comment.
func TestTheRequirementFindingAgreesAcrossTheSeam(t *testing.T) {
	t.Parallel()
	owner := structFields(t,
		filepath.Join(repoRoot, "backend", "internal", "modules", "consent", "requirements.go"),
		"RequirementFinding")
	mirror := structFields(t,
		filepath.Join(repoRoot, "backend", "internal", "modules", "comms", "manifest.go"),
		"RequirementFinding")

	if strings.Join(owner, ",") != strings.Join(mirror, ",") {
		t.Errorf("consent's RequirementFinding carries %v and comms' carries %v.\n\n"+
			"The compose adapter copies field by field and keeps compiling when one side "+
			"grows, so a field added on the consent side is computed, dropped at the seam, "+
			"and missing from the reason an operator reads on the parked delivery.",
			owner, mirror)
	}
}
