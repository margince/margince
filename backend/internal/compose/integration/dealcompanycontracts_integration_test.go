// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal and the contracts filed against it name the same company. The filing
// check refuses to create a mismatch; these cover a deal moved to another
// company afterwards, through each writer that can move it.

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// companyMoveFixture is two companies and an open stage to file deals in.
type companyMoveFixture struct {
	e             *Env
	pipeline      ids.PipelineID
	open          ids.StageID
	acme, contoso ids.CompanyID
}

func newCompanyMoveFixture(t *testing.T) companyMoveFixture {
	t.Helper()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	return companyMoveFixture{
		e: e, pipeline: pipeline, open: open,
		acme:    ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", nil)),
		contoso: ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso", nil)),
	}
}

// dealFor files a deal for company (nil: none) as the admin.
func (f companyMoveFixture) dealFor(t *testing.T, company *ids.CompanyID, owner *ids.UUID) ids.DealID {
	t.Helper()
	deal, err := f.e.Deals.CreateDeal(f.e.Admin(), deals.CreateDealInput{
		Name: "Expansion", PipelineID: f.pipeline, StageID: f.open, CompanyID: company, OwnerID: userIDPtr(owner),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.DealKind](ids.UUID(deal.Id))
}

// acmeContractOn files Acme's contract against deal through the real writer.
func (f companyMoveFixture) acmeContractOn(t *testing.T, deal ids.DealID, number string) ids.ContractID {
	t.Helper()
	filed, err := f.e.Contracts.CreateContract(f.e.Admin(), contracts.CreateContractInput{
		CompanyID: f.acme, DealID: &deal, Title: "Acme MSA", ContractNumber: &number,
		ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	if err != nil {
		t.Fatalf("filing Acme's contract against the deal: %v", err)
	}
	return ids.From[ids.ContractKind](ids.UUID(filed.Id))
}

func (f companyMoveFixture) moveTo(ctx context.Context, deal ids.DealID, company ids.CompanyID) error {
	_, err := f.e.Deals.UpdateDeal(ctx, deal, deals.UpdateDealInput{CompanyID: &company})
	return err
}

// dealStillNames fails when the deal's company is no longer want.
func (f companyMoveFixture) dealStillNames(t *testing.T, deal ids.DealID, want *ids.CompanyID) {
	t.Helper()
	if f.e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND company_id IS NOT DISTINCT FROM $2`, deal, want) != 1 {
		t.Error("the deal moved although the move was refused")
	}
}

// The state the refusal exists against, seeded past every writer. A contract
// with a deal is visible through that deal alone. So a deal naming Contoso
// shows Acme's contract to a rep who cannot see Acme at all.
func TestADealAnchorShowsAContractOfAnotherCompanyToTheDealsReaders(t *testing.T) {
	f := newCompanyMoveFixture(t)
	e := f.e
	acme := e.SeedCompany(t, "Acme private", &e.Rep1)
	e.MakeCapturePrivate(t, "company", acme, e.Rep1)
	contoso := ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso own", &e.Rep3))
	deal := f.dealFor(t, &contoso, &e.Rep3)
	seed := func(title string, anchor *ids.DealID) ids.ContractID {
		id := ids.NewV7()
		e.WsExec(t, `INSERT INTO contract (id, company_id, deal_id, title, value_minor, currency,
		                                   value_basis, source, captured_by)
		             VALUES ($1, $2, $3, $4, 250000, 'EUR', 'total', 'manual', 'seed')`,
			id, acme, anchor, title)
		return ids.From[ids.ContractKind](id)
	}
	misfiled := seed("Acme MSA", &deal)
	unanchored := seed("Acme pilot", nil)

	rep3 := e.As(e.Rep3, []ids.UUID{e.Team2}, ContractRepPerms)
	if _, err := e.Contracts.GetContract(rep3, unanchored); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("Acme's unanchored contract answered %v; this reader must not see Acme", err)
	}
	if leaked, err := e.Contracts.GetContract(rep3, misfiled); err != nil || leaked.Title != "Acme MSA" {
		t.Fatalf("the deal arm no longer admits the mismatched contract (%v); if the visibility "+
			"rule changed, re-decide whether the deal company move still needs refusing", err)
	}
}

// Both known routes into the mismatch reach the deal update, which refuses
// each before writing. One deal was filed for Acme and corrected to Contoso.
// The other named no company when Acme's contract was filed.
func TestMovingADealsCompanyIsRefusedWhileItsContractsNameAnother(t *testing.T) {
	f := newCompanyMoveFixture(t)
	for name, born := range map[string]*ids.CompanyID{
		"filed for Acme, moved to Contoso":    &f.acme,
		"filed for no company, given Contoso": nil,
	} {
		t.Run(name, func(t *testing.T) {
			deal := f.dealFor(t, born, nil)
			f.acmeContractOn(t, deal, "MSA-2026")

			err := f.moveTo(f.e.Admin(), deal, f.contoso)

			blocked, ok := errors.AsType[*contracts.DealContractsCrossCompanyError](err)
			if !ok {
				t.Fatalf("the move was admitted (err = %v); Acme's contract is now readable through Contoso's deal", err)
			}
			if blocked.Count != 1 || len(blocked.Named) != 1 || blocked.Named[0] != "MSA-2026" {
				t.Errorf("the refusal names %d %v, want the one contract by its number", blocked.Count, blocked.Named)
			}
			if field, code, _ := blocked.FieldFault(); field != "company_id" || code != "deal_contracts_cross_company" {
				t.Errorf("fault = (%q, %q), want a 422 on company_id", field, code)
			}
			f.dealStillNames(t, deal, born)
		})
	}
}

// The agent tools write deals through the compose provider; the same move is
// refused there, as the same typed fault.
func TestTheAgentToolSurfaceRefusesTheSameCompanyMove(t *testing.T) {
	f := newCompanyMoveFixture(t)
	deal := f.dealFor(t, &f.acme, nil)
	f.acmeContractOn(t, deal, "MSA-2026")

	_, err := compose.NewProviderFor(f.e.DB()).Update(f.e.Admin(), datasource.UpdateInput{
		Ref:   datasource.EntityRef{Type: datasource.EntityDeal, ID: deal.UUID},
		Patch: map[string]any{"company_id": f.contoso.String()},
	})
	if _, ok := errors.AsType[*contracts.DealContractsCrossCompanyError](err); !ok {
		t.Fatalf("the tool surface moved the deal (err = %v)", err)
	}
	f.dealStillNames(t, deal, &f.acme)
}

// The refusal tells the user to detach the contract or file it against a deal
// of the right company; either one clears it.
func TestADetachedOrRefiledContractNoLongerBlocksTheMove(t *testing.T) {
	f := newCompanyMoveFixture(t)
	admin := f.e.Admin()

	t.Run("detached", func(t *testing.T) {
		deal := f.dealFor(t, &f.acme, nil)
		contract := f.acmeContractOn(t, deal, "MSA-1")
		if _, err := f.e.Contracts.UpdateContract(admin, contract,
			crmcontracts.UpdateContractRequest{}, []string{"deal_id"}, nil); err != nil {
			t.Fatalf("detaching the contract: %v", err)
		}
		if err := f.moveTo(admin, deal, f.contoso); err != nil {
			t.Fatalf("the move is refused with nothing filed against the deal: %v", err)
		}
	})
	t.Run("filed against another Acme deal", func(t *testing.T) {
		deal := f.dealFor(t, &f.acme, nil)
		contract := f.acmeContractOn(t, deal, "MSA-2")
		other := openapi_types.UUID(f.dealFor(t, &f.acme, nil).UUID)
		if _, err := f.e.Contracts.UpdateContract(admin, contract,
			crmcontracts.UpdateContractRequest{DealId: &other}, nil, nil); err != nil {
			t.Fatalf("re-filing the contract: %v", err)
		}
		if err := f.moveTo(admin, deal, f.contoso); err != nil {
			t.Fatalf("the move is refused after the contract was re-filed: %v", err)
		}
	})
}

// Moves the rule does not speak to stay allowed: they show no company's
// contracts to another company's readers.
func TestTheCompanyMovesThatShowNoContractToAnotherCompanyStayAllowed(t *testing.T) {
	f := newCompanyMoveFixture(t)
	admin := f.e.Admin()

	t.Run("a deal with no contracts", func(t *testing.T) {
		if err := f.moveTo(admin, f.dealFor(t, &f.acme, nil), f.contoso); err != nil {
			t.Errorf("a deal nothing is filed against was refused: %v", err)
		}
	})
	t.Run("the company the contracts already name", func(t *testing.T) {
		deal := f.dealFor(t, nil, nil)
		f.acmeContractOn(t, deal, "MSA-3")
		if err := f.moveTo(admin, deal, f.acme); err != nil {
			t.Errorf("giving a company-less deal its contract's company was refused: %v", err)
		}
	})
	t.Run("resending the company a mismatched deal already names", func(t *testing.T) {
		// Seeded past the writers: a deal moved before the refusal existed.
		deal := f.dealFor(t, &f.contoso, nil)
		f.e.WsExec(t, `INSERT INTO contract (id, company_id, deal_id, title, value_minor, currency,
		                                   value_basis, source, captured_by)
		             VALUES ($1, $2, $3, 'Acme MSA', 250000, 'EUR', 'total', 'manual', 'seed')`,
			ids.NewV7(), f.acme, deal)
		if err := f.moveTo(admin, deal, f.contoso); err != nil {
			t.Errorf("an edit that keeps the deal's company was refused: %v", err)
		}
	})
	t.Run("clearing the company", func(t *testing.T) {
		deal := f.dealFor(t, &f.acme, nil)
		f.acmeContractOn(t, deal, "MSA-4")
		if _, err := f.e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{Clear: []string{"company_id"}}); err != nil {
			t.Errorf("clearing the company was refused: %v", err)
		}
	})
}

// An archived contract still reads through its deal, so it blocks too.
func TestAnArchivedContractStillBlocksTheMove(t *testing.T) {
	f := newCompanyMoveFixture(t)
	deal := f.dealFor(t, &f.acme, nil)
	contract := f.acmeContractOn(t, deal, "MSA-5")
	if err := f.e.Contracts.ArchiveContract(f.e.Admin(), contract); err != nil {
		t.Fatal(err)
	}
	if _, ok := errors.AsType[*contracts.DealContractsCrossCompanyError](f.moveTo(f.e.Admin(), deal, f.contoso)); !ok {
		t.Fatal("the move was admitted over an archived contract")
	}
}

// A company merge moves the deal and its contracts in one transaction, so it
// never splits them and is not refused.
func TestACompanyMergeCarriesTheDealAndItsContractsTogether(t *testing.T) {
	f := newCompanyMoveFixture(t)
	deal := f.dealFor(t, &f.acme, nil)
	contract := f.acmeContractOn(t, deal, "MSA-6")
	if _, err := f.e.Contacts.MergeCompany(f.e.Admin(), f.acme, f.contoso, nil); err != nil {
		t.Fatalf("merging Acme into Contoso: %v", err)
	}
	f.dealStillNames(t, deal, &f.contoso)
	if f.e.WsCount(t, `SELECT count(*) FROM contract WHERE id = $1 AND company_id = $2 AND deal_id = $3`,
		contract, f.contoso, deal) != 1 {
		t.Error("the merge left the contract naming the dissolved company while its deal moved")
	}
}

// The refusal runs after the existing gates, so RBAC and row scope answer as
// before. A role without contract read learns how many contracts block, not
// what they are called.
func TestTheRefusalKeepsTheDealGatesAndTheContractReadGrant(t *testing.T) {
	f := newCompanyMoveFixture(t)
	e := f.e
	f.acme = ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme of Rep1", &e.Rep1))
	f.contoso = ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso of Rep1", &e.Rep1))
	deal := f.dealFor(t, &f.acme, &e.Rep1)
	f.acmeContractOn(t, deal, "MSA-7")

	if err := f.moveTo(e.As(e.Rep1, []ids.UUID{e.Team1}, ReadOnlyPerms), deal, f.contoso); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader without deal update got %v, want permission denied", err)
	}
	// The deal's write check answers a rep on another team before the
	// contracts are read, so they learn nothing about them.
	if err := f.moveTo(e.As(e.Rep3, []ids.UUID{e.Team2}, AccountRepPerms), deal, f.contoso); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep outside the deal's team got %v, want the write check's permission denied", err)
	}
	err := f.moveTo(e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms), deal, f.contoso)
	blocked, ok := errors.AsType[*contracts.DealContractsCrossCompanyError](err)
	if !ok {
		t.Fatalf("the deal's own rep moved it (err = %v)", err)
	}
	if blocked.Count != 1 || len(blocked.Named) != 0 {
		t.Errorf("a rep without contract read was told %d %v, want the count and no names", blocked.Count, blocked.Named)
	}
}

// The filing check holds from the contract's side too: a contract cannot be
// re-pointed at a deal of another company.
func TestAContractCannotBeRepointedAtADealOfAnotherCompany(t *testing.T) {
	f := newCompanyMoveFixture(t)
	acmeDeal := f.dealFor(t, &f.acme, nil)
	contract := f.acmeContractOn(t, acmeDeal, "MSA-8")
	contosoDeal := openapi_types.UUID(f.dealFor(t, &f.contoso, nil).UUID)

	_, err := f.e.Contracts.UpdateContract(f.e.Admin(), contract,
		crmcontracts.UpdateContractRequest{DealId: &contosoDeal}, nil, nil)

	var crossed *contracts.CrossCompanyLinkError
	if !errors.As(err, &crossed) || crossed.Field != "deal_id" {
		t.Fatalf("re-pointing Acme's contract at Contoso's deal answered %v, want the refusal on deal_id", err)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM contract WHERE id = $1 AND deal_id = $2`, contract, acmeDeal) != 1 {
		t.Error("the contract left its Acme deal although the patch was refused")
	}
}
