// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A commission entry is the partner's compensation on a deal, and a rep sees
// what THEIR deals earned — not the ledger. The deal itself is readable by every
// seat, which is why the entry cannot simply follow the deal's read.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// commissionRepPerms is the seeded rep's reach into the ledger: commission and
// deal read, bounded to their own rows.
var commissionRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"commission": {Read: true},
		"deal":       {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeOwn,
}

func TestARepSeesWhatTheirOwnDealEarnedAndNotTheLedger(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20") // the deal is Rep1's
	winAndDeliver(t, e, fx)

	owner := e.As(e.Rep1, nil, commissionRepPerms)
	colleague := e.As(e.Rep3, nil, commissionRepPerms)

	// The positive control: the rep who owns the deal reads its entry.
	mine := ledgerOf(owner, t, fx.ledger)
	if len(mine) != 1 {
		t.Fatalf("the deal's owner reads %d entries, want the one their deal accrued", len(mine))
	}
	entry := ids.From[ids.CommissionEntryKind](ids.UUID(mine[0].Id))

	// A colleague may read the deal — it is workspace identity — but not what
	// its partner earned, in the list or through the single read.
	if got := ledgerOf(colleague, t, fx.ledger); len(got) != 0 {
		t.Errorf("a colleague with row scope `own` listed %d entries of a deal they do not work", len(got))
	}
	if _, err := fx.ledger.GetCommissionEntry(colleague, entry); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague's read of the entry answered %v, want not-found", err)
	}

	// A share of the deal is what makes it theirs to see, as it is for the deal.
	if _, err := identity.NewServiceFor(e.DB()).CreateRecordGrant(e.As(e.AdminUser, nil, AdminPerms), identity.CreateGrantInput{
		RecordType: "deal", RecordID: fx.deal.UUID, SubjectType: "user", SubjectID: e.Rep3, Access: "read",
	}); err != nil {
		t.Fatalf("sharing the deal: %v", err)
	}
	if got := ledgerOf(colleague, t, fx.ledger); len(got) != 1 {
		t.Errorf("a colleague granted the deal lists %d entries, want its one", len(got))
	}
}

// A deal nobody owns is nobody's ledger either. An owner's deletion leaves the
// deal ownerless (the foreign key sets it NULL), and reading "unowned" as
// "everyone's" would hand that deal's commissions to every rep.
func TestTheCommissionOfAnOwnerlessDealIsNotEveryRepsToRead(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)

	// The state the owner's deletion leaves, written the way the foreign key
	// writes it: the deal row stays and its owner goes.
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE deal SET owner_id = NULL WHERE id = $1`, fx.deal); err != nil {
		t.Fatal(err)
	}
	if got := ledgerOf(e.As(e.Rep3, nil, commissionRepPerms), t, fx.ledger); len(got) != 0 {
		t.Errorf("a rep listed %d entries of a deal nobody owns", len(got))
	}
	// The unbounded seat still reads it, so the entry did not simply vanish.
	if got := ledgerOf(e.As(e.AdminUser, nil, commissionAdminPerms), t, fx.ledger); len(got) != 1 {
		t.Errorf("the admin lists %d entries of the ownerless deal, want its one", len(got))
	}
}

// ledgerOf lists the whole ledger the caller is shown, unfiltered — the
// enumeration a rep reaches with a bare GET.
func ledgerOf(ctx context.Context, t *testing.T, ledger *commissions.Store) []crmcontracts.CommissionEntry {
	t.Helper()
	page, err := ledger.List(ctx, commissions.ListInput{})
	if err != nil {
		t.Fatalf("listing the ledger: %v", err)
	}
	return page.Data
}
