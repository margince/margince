// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// newRecordOwnerSQL swaps the status clause and nothing else. A swap that
// stopped matching would refuse an invited owner again. A wider swap would
// loosen the seat rule with it.
func TestANewRecordMayNameAnInvitedOwner(t *testing.T) {
	p := principal.Principal{Type: principal.PrincipalHuman, ID: "user:test"}
	arg := func(any) int { return 1 }
	routing := AssigneeEligibleSQL("u")
	owner := newRecordOwnerSQL(p, "u", arg)

	if !strings.Contains(owner, workforce.ActivatableMemberSQL("u")) {
		t.Fatalf("a new record's owner check does not admit an invited seat:\n%s", owner)
	}
	if strings.Contains(routing, "invited") {
		t.Errorf("routing admits an invited seat; only a new record's owner may:\n%s", routing)
	}
	if strings.Replace(owner, workforce.ActivatableMemberSQL("u"), workforce.LiveMemberSQL("u"), 1) != routing {
		t.Errorf("the owner check differs from the routing check beyond the status clause:\nrouting: %s\nowner:   %s", routing, owner)
	}
}

// Only an owner the record runs as is held to the caller's own write scope. A
// record's owner is any seat that can do the work, so a seat on the own scope
// can hand its records to a colleague.
func TestOnlyARunAsOwnerIsHeldToTheCallersScope(t *testing.T) {
	p := principal.Principal{Type: principal.PrincipalHuman, ID: "user:test"}
	p.Permissions.RowScope = principal.RowScopeOwn
	arg := func(any) int { return 1 }
	record := recordOwnerSQL(p, "u", arg)
	runAs := runAsOwnerSQL(p, "u", arg)

	if strings.Contains(record, "owner_id") {
		t.Errorf("a record's owner is scoped to the caller's reach:\n%s", record)
	}
	if !strings.HasPrefix(runAs, record+" AND ") || !strings.Contains(runAs, "u.owner_id") {
		t.Errorf("a run-as owner is not the record rule plus the caller's write scope:\nrecord: %s\nrun-as: %s", record, runAs)
	}
}
