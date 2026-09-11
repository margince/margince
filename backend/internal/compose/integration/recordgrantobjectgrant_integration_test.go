// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record-grant list discloses a share only to a seat that could read the
// record it shares.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A seat whose role holds no deal.read lists no deal shares.
//
// Row scope alone cannot say this: a deal is read whole by every seat, so its
// scope probe admits any deal id without asking anything. The OBJECT grant is
// the half that answers, and a role configured without it is how an installation
// says this seat does not see deals — including which deals were shared, with
// whom, and why.
func TestTheGrantListWithholdsSharesOfARecordTypeTheSeatCannotRead(t *testing.T) {
	e := Setup(t)
	admin := e.As(e.AdminUser, nil, AdminPerms)
	shares := identity.NewServiceFor(e.DB())

	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Quiet expansion", pipeline, open, &e.Rep1)
	org := e.SeedOrg(t, "Shared Holding", &e.Rep1)
	for _, share := range []identity.CreateGrantInput{
		{RecordType: "deal", RecordID: deal, SubjectType: "user", SubjectID: e.Rep3, Access: "read"},
		{RecordType: "organization", RecordID: org, SubjectType: "user", SubjectID: e.Rep3, Access: "read"},
	} {
		if _, err := shares.CreateRecordGrant(admin, share); err != nil {
			t.Fatalf("sharing the %s: %v", share.RecordType, err)
		}
	}

	// Organizations yes, deals no — the configuration an admin reaches through
	// the role editor, at an unbounded scope so row scope cannot be the reason.
	noDeals := principal.Permissions{
		RoleKeys: []string{"custom"},
		Objects:  map[string]principal.ObjectGrant{"organization": {Read: true}},
		RowScope: principal.RowScopeAll,
	}
	seen := grantedRecords(e.As(ids.NewV7(), nil, noDeals), t, shares)
	if seen[deal] {
		t.Error("a seat with no deal.read listed a deal share — the deal id, who holds it and why")
	}
	if !seen[org] {
		t.Error("the same seat lost the organization share it may read — the list withholds by type, not wholesale")
	}

	// The admin, holding both grants, still reads both shares.
	all := grantedRecords(admin, t, shares)
	if !all[deal] || !all[org] {
		t.Fatalf("the admin listed %v, want both shares", all)
	}
}

// grantedRecords lists every live share the caller is shown, keyed by the
// record each one names.
func grantedRecords(ctx context.Context, t *testing.T, shares *identity.Service) map[ids.UUID]bool {
	t.Helper()
	grants, _, err := shares.ListRecordGrants(ctx, identity.ListGrantsInput{})
	if err != nil {
		t.Fatalf("listing the record grants: %v", err)
	}
	seen := make(map[ids.UUID]bool, len(grants))
	for _, g := range grants {
		seen[g.RecordID] = true
	}
	return seen
}
