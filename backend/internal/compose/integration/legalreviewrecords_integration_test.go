// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The review pack's agreements, touches and tasks through the real readers.
// The pack carries what the deal holds, leaves out another deal's records, and
// shows a seat without the object grant none of that kind.

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/legalreviewrecords"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheReviewPackReadsOnlyTheDealsOwnAgreementsTouchesAndTasks(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	reader := legalreviewrecords.New(e.Contracts, e.Activities)
	company := e.SeedCompany(t, "Pack Holdings", nil)
	otherCompany := e.SeedCompany(t, "Other Holdings", nil)
	pipeline, open, _ := DealFixture(t, e)
	seedDeal := func(name string, company ids.UUID) ids.UUID {
		companyID := ids.From[ids.CompanyKind](company)
		created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Source: "manual",
			Name:   name, PipelineID: pipeline, StageID: open, CompanyID: &companyID,
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

func packSeat(e *Env, user ids.UUID, team ids.UUID) context.Context {
	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{
		"activity": {Create: true, Read: true, Update: true},
		"deal":     {Read: true},
		"contact":  {Read: true},
	}
	return e.As(user, []ids.UUID{team}, perms)
}

func packDeal(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	company := e.SeedCompany(t, "Audience Holdings", nil)
	pipeline, open, _ := DealFixture(t, e)
	companyID := ids.From[ids.CompanyKind](company)
	// Rep1 owns it, so Rep1 may file mail onto it.
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Source: "manual",
		Name:   "Audience renewal", PipelineID: pipeline, StageID: open, CompanyID: &companyID, OwnerID: &owner,
	})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	return ids.UUID(created.Id)
}

// Mail limited to its participants is discoverable but not readable by a
// colleague. The pack freezes only what the requester read, and a reviewer
// outside the audience gets no id, kind or time of it.
func TestThePackLeavesOutMailOutsideTheRequestersAndTheReviewersAudience(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	reader := legalreviewrecords.New(e.Contracts, e.Activities)
	deal := packDeal(t, e)
	author, colleague := packSeat(e, e.Rep1, e.Team1), packSeat(e, e.Rep3, e.Team2)
	subject := "Confidential pricing"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: new("outbound"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	mail := ids.UUID(logged.Id)
	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](mail),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail: %v", err)
	}

	outside, _, err := reader.ReviewActivity(colleague, deal)
	if err != nil || len(outside) != 0 {
		t.Fatalf("a requester outside the audience froze %+v (%v), want nothing", outside, err)
	}
	inside, _, err := reader.ReviewActivity(author, deal)
	if err != nil || len(inside) != 1 || inside[0].ActivityID != mail {
		t.Fatalf("a requester inside the audience froze %+v (%v), want the mail", inside, err)
	}
	seenByColleague, err := reader.OpenableActivity(colleague, deal, []ids.UUID{mail})
	if err != nil || len(seenByColleague) != 0 {
		t.Fatalf("a reviewer outside the audience was given %v (%v), want no trace", seenByColleague, err)
	}
	seenByAuthor, err := reader.OpenableActivity(author, deal, []ids.UUID{mail})
	if err != nil || seenByAuthor[mail] == nil || *seenByAuthor[mail] != subject {
		t.Fatalf("a reviewer inside the audience was given %v (%v), want the subject", seenByAuthor, err)
	}
}

// Automatic mail is filtered in the query, so a run of it newer than a manual
// touch cannot push the touch out of the five.
func TestTheLatestTouchesSkipAutomaticMailBeforeCounting(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	reader := legalreviewrecords.New(e.Contracts, e.Activities)
	deal := packDeal(t, e)
	log := func(kind, subject string, ago time.Duration) ids.UUID {
		at := clocktest.Now(t).Add(-ago)
		logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		return ids.UUID(logged.Id)
	}
	note := log("note", "Terms agreed", 72*time.Hour)
	for i := range 30 {
		bulk := log("email", "Newsletter", time.Duration(i+1)*time.Minute)
		e.WsExec(t, `UPDATE activity SET bulk_mail_attested = true WHERE id = $1`, bulk)
	}
	touches, _, err := reader.ReviewActivity(e.Admin(), deal)
	if err != nil || len(touches) != 1 || touches[0].ActivityID != note {
		t.Fatalf("the latest touches were %+v (%v), want the one manual note", touches, err)
	}
}

// The deal's list is one page, newest term first. A frozen agreement past it
// still opens by id, and more than a page of ids is read in full. The read
// keeps the deal, archive, grant and row-scope rules.
func TestAFrozenAgreementOpensPastTheDealsNewestPage(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	reader := legalreviewrecords.New(e.Contracts, e.Activities)
	pipeline, open, _ := DealFixture(t, e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Paper Holdings", nil))
	seedDeal := func(name string) ids.UUID {
		created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Source: "manual",
			Name:   name, PipelineID: pipeline, StageID: open, CompanyID: &company,
		})
		if err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	deal := seedDeal("Paper renewal")
	agreement := func(title string, deal ids.UUID) ids.UUID {
		return ids.UUID(seedContract(t, e, contracts.CreateContractInput{
			CompanyID: company, DealID: dealIDPtr(deal), Title: title, ValueBasis: contracts.BasisTotal, Source: "manual",
		}).UUID)
	}
	frozen := agreement("Master agreement", deal)
	asked := []ids.UUID{frozen}
	for i := range 200 {
		asked = append(asked, agreement(fmt.Sprintf("Order form %d", i), deal))
	}
	// No writer files an agreement under a company other than its deal's, so
	// the legacy mismatch is set by hand. Only its own private company admits it.
	hidden := agreement("Legacy side letter", deal)
	private := e.SeedCompany(t, "Private Holdings", &e.Rep3)
	e.WsExec(t, `UPDATE contract SET company_id = $1 WHERE id = $2`, private, hidden)
	e.MakeCapturePrivate(t, "company", private, e.Rep3)
	archived := agreement("Withdrawn draft", deal)
	if err := e.Contracts.ArchiveContract(e.Admin(), ids.From[ids.ContractKind](archived)); err != nil {
		t.Fatalf("archiving the draft: %v", err)
	}
	elsewhere := agreement("The other deal's agreement", seedDeal("Paper side deal"))
	asked = append(asked, hidden, archived, elsewhere)

	page, err := reader.ReviewContracts(e.Admin(), deal)
	if err != nil || len(page) != 200 || slices.ContainsFunc(page, func(c deals.PacketContract) bool { return c.ContractID == frozen }) {
		t.Fatalf("the deal's page held %d agreements (%v), want 200 without the oldest", len(page), err)
	}
	owner := e.As(e.Rep3, []ids.UUID{e.Team2}, contractReaderPerms())
	named, err := reader.OpenableContracts(owner, deal, asked)
	if err != nil || len(named) != 202 || named[frozen] != "Master agreement" || named[hidden] != "Legacy side letter" {
		t.Fatalf("the private company's owner read %d of %d ids, frozen %q (%v), want the 202 live ones on the deal", len(named), len(asked), named[frozen], err)
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, contractReaderPerms())
	seen, err := reader.OpenableContracts(rep, deal, asked)
	if _, leaked := seen[hidden]; err != nil || len(seen) != 201 || seen[frozen] != "Master agreement" || leaked {
		t.Fatalf("a rep read %d ids, hidden one leaked %t (%v), want 201 without the private company's", len(seen), leaked, err)
	}
	blind := e.As(e.AdminUser, nil, contractReaderPerms("contract"))
	if named, err := reader.OpenableContracts(blind, deal, []ids.UUID{frozen}); err != nil || len(named) != 0 {
		t.Fatalf("a seat without contract.read was handed %v (%v)", named, err)
	}
}
