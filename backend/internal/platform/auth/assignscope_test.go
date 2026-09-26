// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// newRecordOwnerSQL swaps the status clause and nothing else. If the swap
// stopped matching (AssigneeEligibleSQL respelled its status clause), a new
// record would silently refuse an invited owner again; if it swapped more, the
// seat and scope halves of the rule would loosen with it.
func TestANewRecordMayNameAnInvitedOwner(t *testing.T) {
	p := principal.Principal{Type: principal.PrincipalHuman, ID: "user:test"}
	arg := func(any) int { return 1 }
	routing := AssigneeEligibleSQL(p, "u", arg)
	owner := newRecordOwnerSQL(p, "u", arg)

	if !strings.Contains(owner, "u.status IN ('active', 'invited')") {
		t.Fatalf("a new record's owner check does not admit an invited seat:\n%s", owner)
	}
	if strings.Contains(routing, "invited") {
		t.Errorf("routing admits an invited seat; only a new record's owner may:\n%s", routing)
	}
	if strings.Replace(owner, "u.status IN ('active', 'invited')", "u.status = 'active'", 1) != routing {
		t.Errorf("the owner check differs from the routing check beyond the status clause:\nrouting: %s\nowner:   %s", routing, owner)
	}
}
