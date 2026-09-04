// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The posture an installation ships with.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// EVERY CATEGORY ENFORCES, and the default is derived from the vocabulary
// rather than typed out.
//
// A hand-written map is a second list. A category added to commsauthz and
// forgotten here would ship observing — recorded, not binding — which is the
// state this default exists to leave behind, and nothing would say so.
//
// Mutation: return a partial map from enforceEveryCategory and this fails,
// naming the category left behind.
func TestTheShippedPostureEnforcesEveryCategory(t *testing.T) {
	t.Parallel()

	modes := enforceEveryCategory()
	for _, c := range commsauthz.Categories() {
		if got := ModeFor(modes, c); got != commsauthz.ModeEnforce {
			t.Errorf("%s ships in %s, want enforce: it would be recorded and not binding, "+
				"and nothing would report that", c, got)
		}
	}
	if len(modes) != len(commsauthz.Categories()) {
		t.Errorf("the shipped posture names %d categories and the vocabulary holds %d",
			len(modes), len(commsauthz.Categories()))
	}
}

// THE DEFAULT IS STILL A VALID SETTING. It goes through the same validator an
// operator's map does, so a vocabulary drift that made the shipped posture
// unstorable would fail here rather than at an installation's first write.
func TestTheShippedPostureValidates(t *testing.T) {
	t.Parallel()

	if err := validateAuthorizationModes(enforceEveryCategory()); err != nil {
		t.Fatalf("the shipped posture is not a storable setting: %v", err)
	}
}
