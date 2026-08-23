// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The refusal that names what on_duplicate accepts must name ALL of it.
//
// duplicatePolicies is a hand-written list, and the message it feeds is the only
// thing a caller who guessed wrong ever sees. It went stale once already: the
// text said "create or skip" after `update` had shipped, so a caller asking for
// a policy that works would have been told it does not exist. A list that
// describes a contract enum has to be checked against that enum.

import (
	"strings"
	"testing"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

func TestEveryDuplicatePolicyTheContractDeclaresIsNamedInTheRefusal(t *testing.T) {
	named := strings.Join(duplicatePolicies(), ",")
	// Every value the generated enum accepts. Valid() is the contract's own
	// answer, so a value added to crm.yaml appears here without this test being
	// edited — which is the point.
	for _, candidate := range []string{"create", "skip", "update", "merge", "replace", "ignore"} {
		policy := crmcontracts.ImportOnDuplicate(candidate)
		accepted := policy.Valid()
		mentioned := strings.Contains(named, candidate)
		if accepted && !mentioned {
			t.Errorf("on_duplicate accepts %q and the refusal message lists only %q — a caller who "+
				"guessed wrong is told a working policy does not exist", candidate, named)
		}
		if !accepted && mentioned {
			t.Errorf("the refusal message offers %q and on_duplicate does not accept it — a caller "+
				"following the message is refused again", candidate)
		}
	}
}
