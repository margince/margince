// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contract is admitted by its deal OR its company, and that disjunction
// is about ADMISSION. A reader let in through the deal may not be able to open
// the company, the delivery or even the deal itself as a KIND of record — so
// the three references the projection carries are withheld from a reader who
// could not open them, and named in masked_fields.
//
// The sibling of dealreferences_integration_test.go, and the half that closes a
// hole that one left: `deal_project_same_company` forces a deal's project and its
// company to name one company, so a caller who reads the project and not
// the company recovered in one hop, through the contract, the id the deal read
// had just withheld.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// contractReferenceFixture is one agreement per reference under test, each
// reached through an anchor the reader CAN see so the mask is the only thing
// standing between them and the id.
type contractReferenceFixture struct {
	// onPrivateCompany is anchored on a capture-private company, admitted through
	// its deal.
	onPrivateCompany ids.ContractID
	// onOtherTeamsProject names a delivery, on a company the reader can open.
	onOtherTeamsProject ids.ContractID
	openCompany         ids.UUID
	privateCompany      ids.UUID
	project             ids.ProjectID
}

func seedContractReferenceFixture(t *testing.T, e *Env) contractReferenceFixture {
	t.Helper()
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)

	// Seeded workspace-visible so the admin's writes pass their own
	// EnsureLinkTarget gate; capture privacy lands afterwards, which is the
	// order a connector-captured company reaches this state in anyway.
	privateCompany := e.SeedCompany(t, "Meridian Labs", &e.Rep3)
	openCompany := e.SeedCompany(t, "Kestrel Foods", nil)

	privateDeal := e.SeedDeal(t, "Meridian renewal", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, privateDeal, privateCompany)
	onPrivateCompany := seedContract(t, e, contracts.CreateContractInput{
		CompanyID:  companyIDOf(privateCompany),
		DealID:     dealIDPtr(privateDeal),
		Title:      "An agreement with a company the reader cannot open",
		ValueBasis: contracts.BasisTotal,
		Source:     "manual",
	})
	e.MakeCapturePrivate(t, "company", privateCompany, e.Rep3)

	project := seedProject(admin, t, e, "Kestrel rollout", openCompany, &e.Rep3)
	openDeal := e.SeedDeal(t, "Kestrel expansion", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, openDeal, openCompany)
	onOtherTeamsProject := seedContract(t, e, contracts.CreateContractInput{
		CompanyID:  companyIDOf(openCompany),
		DealID:     dealIDPtr(openDeal),
		ProjectID:  &project.ID,
		Title:      "An agreement funding a delivery",
		ValueBasis: contracts.BasisTotal,
		Source:     "manual",
	})
	return contractReferenceFixture{
		onPrivateCompany:    onPrivateCompany,
		onOtherTeamsProject: onOtherTeamsProject,
		openCompany:         openCompany,
		privateCompany:      privateCompany,
		project:             project.ID,
	}
}

func seedContract(t *testing.T, e *Env, in contracts.CreateContractInput) ids.ContractID {
	t.Helper()
	created, err := e.Contracts.CreateContract(e.Admin(), in)
	if err != nil {
		t.Fatalf("create contract %q: %v", in.Title, err)
	}
	return ids.From[ids.ContractKind](ids.UUID(created.Id))
}

func dealIDPtr(deal ids.UUID) *ids.DealID {
	id := ids.From[ids.DealKind](deal)
	return &id
}

// contractReaderPerms is a rep who reads agreements: the object grid
// AccountRepPerms carries, plus `contract`, minus whatever the case removes.
func contractReaderPerms(without ...string) principal.Permissions {
	perms := AccountRepPerms
	perms.Objects = make(map[string]principal.ObjectGrant, len(AccountRepPerms.Objects)+1)
	for object, grant := range AccountRepPerms.Objects {
		perms.Objects[object] = grant
	}
	perms.Objects["contract"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	for _, object := range without {
		delete(perms.Objects, object)
	}
	return perms
}

func TestAContractDoesNotNameRecordsItsReaderCannotRead(t *testing.T) {
	e := Setup(t)
	fx := seedContractReferenceFixture(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, contractReaderPerms())

	// Admitted through the deal, which every seat reads. The company it names
	// is capture-private to a colleague, so the id does not travel with it.
	got, err := e.Contracts.GetContract(rep, fx.onPrivateCompany)
	if err != nil {
		t.Fatalf("a rep reading a contract whose company is capture-private: %v", err)
	}
	if got.CompanyId != nil {
		t.Errorf("company_id = %v, want withheld: the reader was admitted through the deal "+
			"and cannot open the company", got.CompanyId)
	}
	assertContractMaskNames(t, got, "company_id")

	// A project is read by every seat HOLDING THE OBJECT GRANT, and this rep
	// holds no project grant at all. Row scope is not the only gate on a
	// reference: object RBAC answers whether the caller may read that KIND of
	// record, and a contract must not become the door to an id from a table
	// its reader may not open.
	delivery, err := e.Contracts.GetContract(rep, fx.onOtherTeamsProject)
	if err != nil {
		t.Fatalf("a rep reading a contract that funds another team's delivery: %v", err)
	}
	if delivery.ProjectId != nil {
		t.Errorf("project_id = %v, want withheld: this rep holds no project.read grant", delivery.ProjectId)
	}
	if delivery.CompanyId == nil || ids.UUID(*delivery.CompanyId) != fx.openCompany {
		t.Errorf("company_id = %v, want the workspace-visible company the reader CAN open",
			delivery.CompanyId)
	}
	assertContractMaskNames(t, delivery, "project_id")

	// Add the project grant and the SAME contract names its delivery. Without
	// this half the assertion above would pass against a mask that withheld
	// every project from everybody.
	withProject := contractReaderPerms()
	withProject.Objects["project"] = principal.ObjectGrant{Read: true}
	granted, err := e.Contracts.GetContract(e.As(e.Rep1, []ids.UUID{e.Team1}, withProject), fx.onOtherTeamsProject)
	if err != nil {
		t.Fatalf("a rep holding project.read reading the same contract: %v", err)
	}
	if granted.ProjectId == nil {
		t.Error("project_id is withheld from a rep who holds project.read; a project carries " +
			"no owner scope and no capture privacy")
	}

	// The deal arm is the one that ADMITTED this row, and it still says nothing
	// about whether the reader may read deals as a kind.
	noDeals := e.As(e.Rep1, []ids.UUID{e.Team1}, contractReaderPerms("deal"))
	withoutDeal, err := e.Contracts.GetContract(noDeals, fx.onOtherTeamsProject)
	if err != nil {
		t.Fatalf("a rep holding no deal.read reading a contract admitted through its deal: %v", err)
	}
	if withoutDeal.DealId != nil {
		t.Errorf("deal_id = %v, want withheld: this rep holds no deal.read grant", withoutDeal.DealId)
	}

	// A reader who can see all three still receives all three, or the fix
	// closed the oracle by breaking the feature.
	full, err := e.Contracts.GetContract(e.Admin(), fx.onOtherTeamsProject)
	if err != nil || full.CompanyId == nil || full.DealId == nil ||
		full.ProjectId == nil || full.MaskedFields != nil {
		t.Errorf("the admin's read = company %v deal %v project %v masked %v (%v), want every reference",
			full.CompanyId, full.DealId, full.ProjectId, full.MaskedFields, err)
	}
}

// The one-hop recovery this ticket exists to close: `deal_project_same_company`
// forces a deal's project and its company to name one company, so a caller
// who reads the PROJECT and not the company must not be handed that company's
// id by the paper hanging off it.
func TestAContractIsNotTheWayBackToACompanyTheProjectAlreadyWithholds(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)

	company := e.SeedCompany(t, "Halden Industries", &e.Rep3)
	project := seedProject(admin, t, e, "Halden migration", company, &e.Rep3)
	deal := e.SeedDeal(t, "Halden platform", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, deal, company)
	contract := seedContract(t, e, contracts.CreateContractInput{
		CompanyID:  companyIDOf(company),
		DealID:     dealIDPtr(deal),
		ProjectID:  &project.ID,
		Title:      "The paper on a company the reader cannot open",
		ValueBasis: contracts.BasisTotal,
		Source:     "manual",
	})
	e.MakeCapturePrivate(t, "company", company, e.Rep3)

	perms := contractReaderPerms()
	perms.Objects["project"] = principal.ObjectGrant{Read: true}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	seen, err := e.Projects.GetProject(reader, project.ID, 0)
	if err != nil {
		t.Fatalf("a rep reading the project itself: %v", err)
	}
	if seen.CompanyId != nil {
		t.Fatalf("the project already hands back company_id = %v; this test's premise is that "+
			"it does not, so the contract is the only remaining hop", seen.CompanyId)
	}

	got, err := e.Contracts.GetContract(reader, contract)
	if err != nil {
		t.Fatalf("the same rep reading the contract: %v", err)
	}
	if got.CompanyId != nil {
		t.Errorf("company_id = %v from the contract, want withheld — the project read just "+
			"refused it, and one hop through the paper recovers it", got.CompanyId)
	}
	if got.ProjectId == nil || ids.UUID(*got.ProjectId) != project.ID.UUID {
		t.Errorf("project_id = %v, want the delivery this reader CAN open", got.ProjectId)
	}
	assertContractMaskNames(t, got, "company_id")
}

// The page path, not only the single-row one: a list is where an existence
// oracle is cheapest, so it must not be the looser door.
func TestTheContractListWithholdsTheSameReferencesAsTheGet(t *testing.T) {
	e := Setup(t)
	fx := seedContractReferenceFixture(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, contractReaderPerms())

	page, err := e.Contracts.ListCompanyContracts(rep, contracts.ListContractsInput{
		CompanyID: companyIDOf(fx.openCompany),
	})
	if err != nil {
		t.Fatalf("listing a readable company's contracts: %v", err)
	}
	var found bool
	for _, c := range page.Data {
		if ids.UUID(c.Id) != fx.onOtherTeamsProject.UUID {
			continue
		}
		found = true
		if c.ProjectId != nil {
			t.Errorf("the list handed out a project id to a rep holding no project grant: %v", c.ProjectId)
		}
		assertContractMaskNames(t, c, "project_id")
	}
	if !found {
		t.Error("the list does not show the contract at all — withholding a reference must not drop the row")
	}
}

// A mutation RESPONSE is a read. Every entry point that hands a contract back
// must withhold the same references the GET does, or a no-op PATCH becomes the
// second door onto the id the GET just refused. Being allowed to change the
// CONTRACT says nothing about being allowed to read the company it names.
//
// A table over the entry points rather than one case each, so a further
// contract mutation is a compile-time addition to this list rather than a
// silent omission.
func TestEveryContractMutationResponseWithholdsTheSameReferences(t *testing.T) {
	e := Setup(t)
	fx := seedContractReferenceFixture(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, contractReaderPerms())

	retitled := "Meridian renewal, retitled"
	cases := []struct {
		name string
		call func() (crmcontracts.Contract, error)
	}{
		{"a patch that changes nothing still echoes the row", func() (crmcontracts.Contract, error) {
			return e.Contracts.UpdateContract(rep, fx.onPrivateCompany, crmcontracts.UpdateContractRequest{}, nil)
		}},
		{"a patch that changes something", func() (crmcontracts.Contract, error) {
			return e.Contracts.UpdateContract(rep, fx.onPrivateCompany,
				crmcontracts.UpdateContractRequest{Title: &retitled}, nil)
		}},
		{"activating the agreement", func() (crmcontracts.Contract, error) {
			return e.Contracts.ChangeStatus(rep, fx.onPrivateCompany, contracts.StatusActive, nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.CompanyId != nil {
				t.Errorf("%s handed back company_id %v, want it withheld", tc.name, got.CompanyId)
			}
			assertContractMaskNames(t, got, "company_id")
		})
	}
}

// assertContractMaskNames checks masked_fields carries exactly the given names:
// a null a reader cannot distinguish from an empty field is the half-fix this
// whole seam exists to avoid.
func assertContractMaskNames(t *testing.T, c crmcontracts.Contract, want ...string) {
	t.Helper()
	if c.MaskedFields == nil {
		if len(want) > 0 {
			t.Errorf("masked_fields is absent, want %v named — a withheld null must say it was withheld", want)
		}
		return
	}
	got := map[string]bool{}
	for _, field := range *c.MaskedFields {
		got[field] = true
	}
	for _, field := range want {
		if !got[field] {
			t.Errorf("masked_fields = %v, want it to name %s", *c.MaskedFields, field)
		}
	}
	if len(got) != len(want) {
		t.Errorf("masked_fields = %v, want exactly %v", *c.MaskedFields, want)
	}
}
