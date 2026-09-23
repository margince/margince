// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The seat that answers an assurance finding, read off the seeded policy the
// way an installation reads it.
//
// `assurance.Store.Resolve` gates on `forecast.update`, and no seeded role held
// it. Nothing failed: the door refuses, which is what a door does, and that
// refusal is indistinguishable from a correct one.
//
// TestEveryGrantAHandlerRequiresIsHeldBySomeSeededRole holds the general class;
// this holds the specific decision — the seats decided on hold it, the one
// decided against does not. Through Parse and Merge, because the marshalled
// document is what an installation seeds.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheSeatsThatAnswerAnAssuranceFindingHoldTheGrantItSpends(t *testing.T) {
	t.Parallel()
	// Rep is on this list because row scope bounds what they reach: the grant
	// widens what they may SAY about their own work, not what they can see.
	for _, role := range []string{"admin", "management", "manager", "ops", "rep"} {
		t.Run(role, func(t *testing.T) {
			if !seededGrant(t, role).Update {
				t.Errorf("%s cannot answer an assurance finding: the seeded document grants no "+
					"forecast update, so Resolve refuses before it looks the finding up", role)
			}
		})
	}

	// The control: without it this passes on a policy granting the verb to
	// everybody, which is a different decision from the one taken.
	if seededGrant(t, "read_only").Update {
		t.Error("read_only holds forecast update; a seat that may not change the forecast may not " +
			"answer a finding about it either")
	}

	// Delete stays unheld: a call that was made is a thing that happened.
	for _, role := range []string{"admin", "management", "manager", "ops", "rep", "read_only"} {
		if seededGrant(t, role).Delete {
			t.Errorf("%s holds forecast delete — a forecast reading is superseded, never removed", role)
		}
	}
}

// seededGrant reads one role's forecast grant the way an installation does:
// the marshalled document, parsed and merged.
func seededGrant(t *testing.T, role string) principal.ObjectGrant {
	t.Helper()
	doc, err := policy.Parse(policy.MustDefaultJSON(role))
	if err != nil {
		t.Fatalf("parsing the seeded %s document: %v", role, err)
	}
	return policy.Merge(map[string]policy.Document{role: doc}).Objects["forecast"]
}
