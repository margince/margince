// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The review pack's agreements, touches and tasks through the real readers.
// The pack carries what the deal holds, leaves out another deal's records, and
// shows a seat without the object grant none of that kind.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/legalreviewrecords"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheReviewPackReadsOnlyTheDealsOwnAgreementsTouchesAndTasks(t *testing.T) {
	e := Setup(t)
	reader := legalreviewrecords.New(e.Contracts, e.Activities)
	company := e.SeedCompany(t, "Pack Holdings", nil)
	otherCompany := e.SeedCompany(t, "Other Holdings", nil)
	pipeline, open, _ := DealFixture(t, e)
	seedDeal := func(name string, company ids.UUID) ids.UUID {
		companyID := ids.From[ids.CompanyKind](company)
		created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open, CompanyID: &companyID,
		})
		if err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	deal, otherDeal := seedDeal("Pack renewal", company), seedDeal("Pack side deal", otherCompany)
	dealOnly := seedContract(t, e, contracts.CreateContractInput{
		CompanyID: companyIDOf(company), DealID: dealIDPtr(deal), Title: "Master agreement",
		ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	seedContract(t, e, contracts.CreateContractInput{
		CompanyID: companyIDOf(otherCompany), DealID: dealIDPtr(otherDeal), Title: "The other deal's agreement",
		ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	log := func(kind, subject string, deal ids.UUID) ids.UUID {
		logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: kind, Subject: &subject, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		return ids.UUID(logged.Id)
	}
	note := log("note", "Legal terms discussed", deal)
	task := log("task", "Send the DPA", deal)
	elsewhere := log("note", "Another deal's note", otherDeal)

	packed, err := reader.ReviewContracts(e.Admin(), deal)
	if err != nil || len(packed) != 1 || packed[0].ContractID != ids.UUID(dealOnly.UUID) || packed[0].Title != "Master agreement" {
		t.Fatalf("the pack read agreements %+v (%v), want the deal's own alone", packed, err)
	}
	touches, tasks, err := reader.ReviewActivity(e.Admin(), deal)
	if err != nil || len(touches) != 1 || touches[0].ActivityID != note || len(tasks) != 1 || tasks[0].ActivityID != task {
		t.Fatalf("the pack read touches %+v and tasks %+v (%v), want the deal's note and task", touches, tasks, err)
	}
	named, err := reader.OpenableActivity(e.Admin(), deal, []ids.UUID{note, task, elsewhere})
	if err != nil || len(named) != 2 || named[note] == nil || *named[note] != "Legal terms discussed" {
		t.Fatalf("the deal's activity read as %v (%v), want the note and task named and the other deal's note absent", named, err)
	}

	blind := e.As(e.AdminUser, nil, contractReaderPerms("contract", "activity"))
	if packed, err := reader.ReviewContracts(blind, deal); err != nil || len(packed) != 0 {
		t.Fatalf("a seat without contract.read was handed agreements %+v (%v)", packed, err)
	}
	if touches, tasks, err := reader.ReviewActivity(blind, deal); err != nil || len(touches)+len(tasks) != 0 {
		t.Fatalf("a seat without activity.read was handed %d touches and %d tasks (%v)", len(touches), len(tasks), err)
	}
}
