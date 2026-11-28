// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyChannelIsIndependentOfItsDeals(t *testing.T) {
	e := setupLinks(t)
	grant := principal.ObjectGrant{Read: true, Update: true}
	ctx := e.seatWith(e.rep, principal.RowScopeOwn, map[string]principal.ObjectGrant{RecordCompany: grant, RecordDeal: grant, RecordProject: grant})
	company, err := e.store.Link(ctx, RecordCompany, e.company, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if company.Link == nil || company.Link.CompanyID == nil || *company.Link.CompanyID != e.company || company.Link.DealID != nil || company.Link.ProjectID != nil {
		t.Fatalf("company association: %+v", company.Link)
	}
	deal, err := e.store.Link(ctx, RecordDeal, e.deal, LinkRequest{Mode: ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if deal.Link.CompanyID != nil || deal.Link.ID == company.Link.ID {
		t.Fatalf("deal adopted company conversation: %+v", deal.Link)
	}
	if err := e.store.Unlink(ctx, RecordCompany, e.company); err != nil {
		t.Fatal(err)
	}
	state, err := e.store.GetState(ctx, RecordDeal, e.deal)
	if err != nil || state.Link == nil || state.Link.ID != deal.Link.ID {
		t.Fatalf("company unlink changed deal: %+v %v", state, err)
	}
	denied := e.seatWith(e.other, principal.RowScopeOwn, map[string]principal.ObjectGrant{})
	if _, err := e.store.GetState(denied, RecordCompany, e.company); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("account read without a company read grant: %v", err)
	}
}
