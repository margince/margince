// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal and the agreements filed against it must name the same company.
//
// The filing check has always refused to CREATE that mismatch. These cover the
// other end of it — the deal that is moved to another company afterwards — and
// the state itself, seeded directly, because the exposure is a property of the
// row pair rather than of the route that produced it.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// crossCompanyFixture is the pairing every case here is about: an agreement
// belonging to a company the reader cannot open, and a deal belonging to one
// they can.
type crossCompanyFixture struct {
	hidden   ids.UUID // the company whose agreements these are, private to Rep1
	visible  ids.UUID // the company whose deal Rep3 works
	deal     ids.DealID
	pipeline ids.PipelineID
	open     ids.StageID
}

func crossCompanySetup(t *testing.T, e *Env) crossCompanyFixture {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	hidden := e.SeedCompany(t, "Acme", &e.Rep1)
	e.MakeCapturePrivate(t, "company", hidden, e.Rep1)
	visible := e.SeedCompany(t, "Contoso", &e.Rep3)

	visibleID := ids.From[ids.CompanyKind](visible)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Contoso expansion", PipelineID: pipeline, StageID: open,
		CompanyID: &visibleID, OwnerID: userIDPtr(&e.Rep3),
	})
	if err != nil {
		t.Fatal(err)
	}
	return crossCompanyFixture{
		hidden: hidden, visible: visible,
		deal:     ids.From[ids.DealKind](ids.UUID(deal.Id)),
		pipeline: pipeline, open: open,
	}
}

// seedAgreement files one agreement past the store's own door, which is the
// only way to reach a pairing every writer refuses.
func seedAgreement(t *testing.T, e *Env, title string, company ids.UUID, deal *ids.DealID) ids.ContractID {
	t.Helper()
	id := ids.NewV7()
	var anchor *ids.UUID
	if deal != nil {
		anchor = &deal.UUID
	}
	e.WsExec(t, `
		INSERT INTO contract (id, company_id, deal_id, title, value_minor, currency,
		                      value_basis, source, captured_by)
		VALUES ($1, $2, $3, $4, 250000, 'EUR', 'total', 'manual', 'seed')`,
		id, company, anchor, title)
	return ids.From[ids.ContractKind](id)
}

// WHY the deal write refuses: the pairing itself is a cross-tenant read, and it
// needs no route to prove it.
//
// A contract with a deal is judged visible by that DEAL alone — deliberately,
// so that widening to the company does not hand out agreements attached to
// deals nobody can see. Run from the other end, the same rule says a deal that
// names Contoso publishes Acme's agreement to everyone who works Contoso, and
// the agreement's own company is never consulted. The counterparty is withheld
// from the answer, which is the part that makes this worse rather than better:
// the reader is handed the terms and the value of an agreement, and told
// nothing about whose it is.
//
// This case stays meaningful if a route nobody has thought of appears.
func TestADealAnchorPublishesAnAgreementOfAnotherCompany(t *testing.T) {
	e := Setup(t)
	fx := crossCompanySetup(t, e)

	misfiled := seedAgreement(t, e, "Acme MSA", fx.hidden, &fx.deal)
	// The control: the same company's agreement with no deal on it. It proves
	// the read below is the DEAL arm admitting the row and not this reader
	// being able to see Acme after all.
	unanchored := seedAgreement(t, e, "Acme pilot", fx.hidden, nil)

	rep3 := e.As(e.Rep3, []ids.UUID{e.Team2}, ContractRepPerms)
	if _, err := e.Contracts.GetContract(rep3, unanchored); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("Acme's unanchored agreement answered %v; this reader must not see Acme at all", err)
	}

	leaked, err := e.Contracts.GetContract(rep3, misfiled)
	if err != nil {
		t.Fatalf("the read-through this refusal exists against did not happen (%v) — if the "+
			"visibility rule has changed, the deal write's refusal is what to re-decide", err)
	}
	if leaked.Title != "Acme MSA" {
		t.Errorf("the leaked row came back as %q, want the agreement's own title", leaked.Title)
	}
	if leaked.CompanyId != nil {
		t.Errorf("the counterparty is disclosed as %v; the mask is supposed to withhold it, "+
			"which is what leaves the reader holding terms they cannot attribute", leaked.CompanyId)
	}
}

// Both known routes into that state land in the same writer, so one check
// covers both. Route 1 was reachable before any recent change; route 2 opened
// when attaching a contract to a company-less deal stopped being a 500.
func TestMovingADealsCompanyIsRefusedWhileItsAgreementsNameTheOldOne(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	acme := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", nil))
	contoso := ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso", nil))

	routes := map[string]*ids.CompanyID{
		"the deal was filed against Acme and is corrected to Contoso": &acme,
		"the deal named nobody when the agreement was filed":          nil,
	}
	for name, born := range routes {
		t.Run(name, func(t *testing.T) {
			deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
				Name: "Expansion", PipelineID: pipeline, StageID: open, CompanyID: born,
			})
			if err != nil {
				t.Fatal(err)
			}
			dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
			number := "MSA-2026"
			if _, err := e.Contracts.CreateContract(admin, contracts.CreateContractInput{
				CompanyID: acme, DealID: &dealID, Title: "Acme MSA",
				ContractNumber: &number, ValueBasis: contracts.BasisTotal, Source: "manual",
			}); err != nil {
				t.Fatalf("filing Acme's agreement against this deal: %v", err)
			}

			_, err = e.Deals.UpdateDeal(admin, dealID, deals.UpdateDealInput{CompanyID: &contoso})

			var blocked *contracts.DealContractsCrossCompanyError
			if !errors.As(err, &blocked) {
				t.Fatalf("the move was admitted (err = %v); Acme's agreement is now readable "+
					"by everyone who can see Contoso's deal", err)
			}
			if len(blocked.Named) != 1 || blocked.Named[0] != number {
				t.Errorf("the refusal names %v, want the agreement's own number — a rep who is "+
					"not told which one blocks cannot clear it", blocked.Named)
			}
			held := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND company_id IS NOT DISTINCT FROM $2`,
				dealID, born)
			if held != 1 {
				t.Error("the deal moved anyway; the refusal ran after the write rather than before it")
			}
		})
	}
}

// The refusal has to be clearable, or it is the objection to refusing at all:
// a rep who mis-filed a deal would be left unable to correct it on the very
// record whose paperwork they were fixing. Detaching the agreement is the move
// the message names, and this is it working.
func TestDetachingAnAgreementClearsTheRefusedCompanyMove(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	acme := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", nil))
	contoso := ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso", nil))

	deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Expansion", PipelineID: pipeline, StageID: open, CompanyID: &acme,
	})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	filed, err := e.Contracts.CreateContract(admin, contracts.CreateContractInput{
		CompanyID: acme, DealID: &dealID, Title: "Acme MSA",
		ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	filedID := ids.From[ids.ContractKind](ids.UUID(filed.Id))
	if _, err := e.Deals.UpdateDeal(admin, dealID, deals.UpdateDealInput{CompanyID: &contoso}); err == nil {
		t.Fatal("the move was admitted while the agreement still named Acme")
	}

	detached, err := e.Contracts.UpdateContract(admin, filedID,
		crmcontracts.UpdateContractRequest{}, []string{"deal_id"}, nil)
	if err != nil {
		t.Fatalf("detaching the agreement from the deal: %v", err)
	}
	if detached.DealId != nil {
		t.Fatalf("the agreement still names deal %v, so the clear reported success and wrote nothing",
			detached.DealId)
	}
	if _, err := e.Deals.UpdateDeal(admin, dealID, deals.UpdateDealInput{CompanyID: &contoso}); err != nil {
		t.Fatalf("the move is still refused with nothing left filed against the deal: %v", err)
	}
}

// What the check must NOT refuse. Each of these is a move the rule does not
// speak to, and a check that widened to them would break ordinary work while
// closing nothing — the leak is one company's agreements reaching ANOTHER
// company's readers.
func TestTheCompanyMovesThisRuleDoesNotSpeakTo(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	acme := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", nil))
	contoso := ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso", nil))

	withAgreement := func(t *testing.T) ids.DealID {
		t.Helper()
		deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
			Name: "Expansion", PipelineID: pipeline, StageID: open, CompanyID: &acme,
		})
		if err != nil {
			t.Fatal(err)
		}
		dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
		if _, err := e.Contracts.CreateContract(admin, contracts.CreateContractInput{
			CompanyID: acme, DealID: &dealID, Title: "Acme MSA",
			ValueBasis: contracts.BasisTotal, Source: "manual",
		}); err != nil {
			t.Fatal(err)
		}
		return dealID
	}

	t.Run("a deal with no agreements moves", func(t *testing.T) {
		deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
			Name: "Nothing filed", PipelineID: pipeline, StageID: open, CompanyID: &acme,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(deal.Id)),
			deals.UpdateDealInput{CompanyID: &contoso}); err != nil {
			t.Errorf("a deal nothing is filed against was refused: %v", err)
		}
	})

	t.Run("re-sending the company the deal already names", func(t *testing.T) {
		if _, err := e.Deals.UpdateDeal(admin, withAgreement(t),
			deals.UpdateDealInput{CompanyID: &acme}); err != nil {
			t.Errorf("a no-op company assignment was refused: %v", err)
		}
	})

	// Forgetting the company is a different act. A deal naming nobody publishes
	// its agreements to exactly the readers its own scope already admits — which
	// is why an agreement may be filed against a company-less deal in the first
	// place — so refusing this would close nothing and cost a reversal.
	t.Run("forgetting the company altogether", func(t *testing.T) {
		if _, err := e.Deals.UpdateDeal(admin, withAgreement(t),
			deals.UpdateDealInput{Clear: []string{"company_id"}}); err != nil {
			t.Errorf("clearing the company was refused: %v", err)
		}
	})
}

// An ARCHIVED agreement blocks too, and that is the census refusing to fail
// short rather than an oversight. A contract read by id is not filtered by its
// own archived_at — the visibility clause is about its ANCHOR — so an archived
// agreement on a moved deal is read through exactly as a live one is.
func TestAnArchivedAgreementStillBlocksTheMove(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	acme := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", nil))
	contoso := ids.From[ids.CompanyKind](e.SeedCompany(t, "Contoso", nil))

	deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Expansion", PipelineID: pipeline, StageID: open, CompanyID: &acme,
	})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	filed, err := e.Contracts.CreateContract(admin, contracts.CreateContractInput{
		CompanyID: acme, DealID: &dealID, Title: "Acme MSA",
		ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	contractID := ids.From[ids.ContractKind](ids.UUID(filed.Id))
	if err := e.Contracts.ArchiveContract(admin, contractID); err != nil {
		t.Fatal(err)
	}
	// The archived row is still readable by id, which is the whole reason it
	// still counts.
	if _, err := e.Contracts.GetContract(admin, contractID); err != nil {
		t.Fatalf("the archived agreement is no longer readable (%v); if that has changed, "+
			"whether it should still block is what to re-decide", err)
	}

	_, err = e.Deals.UpdateDeal(admin, dealID, deals.UpdateDealInput{CompanyID: &contoso})
	var blocked *contracts.DealContractsCrossCompanyError
	if !errors.As(err, &blocked) {
		t.Fatalf("the move was admitted over an archived agreement (err = %v)", err)
	}
}
