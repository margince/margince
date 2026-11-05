// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The account page counts every proposal on the account the reader could
// decide. That is the number the approvals inbox narrowed to the account
// answers. The list of them stays a capped page.
func TestTheAccountPageCountsEveryPendingProposalPastTheScanCap(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Harbor", nil)
	elsewhere := e.SeedCompany(t, "Elsewhere", nil)
	svc := NewApprovalsService(e.DB())
	for i := range approvals.PendingScanCap + 25 {
		target := company
		if i%10 == 0 {
			target = elsewhere
		}
		raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(
			`{"record_type":"company","id":%q,"fields":{"display_name":"Harbor %d"}}`, target, i)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Stage(e.Admin(), approvals.StageInput{
			Kind: "update_record", TargetType: "company", TargetID: target, ProposedChange: raw, DiffHash: hash,
		}); err != nil {
			t.Fatalf("staging proposal %d: %v", i, err)
		}
	}
	companyType := "company"
	inbox, err := svc.CountPending(e.Admin(), approvals.ListInput{TargetType: &companyType, TargetID: &company, IncludeMuted: true})
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Count <= approvals.PendingScanCap {
		t.Fatalf("the inbox holds %d for the account, not past the scan cap %d, so a capped count would pass",
			inbox.Count, approvals.PendingScanCap)
	}

	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, svc,
		func() time.Time { return clocktest.Now(t) }).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.SinceLastVisit == nil || page.SinceLastVisit.PendingProposals == nil {
		t.Fatalf("the account page counted no pending proposals: %+v", page.SinceLastVisit)
	}
	if got := *page.SinceLastVisit.PendingProposals; got != inbox.Count {
		t.Errorf("the account page counts %d pending proposals, the inbox for the account holds %d", got, inbox.Count)
	}
	if page.PendingApprovals == nil || !page.PendingApprovals.Page.HasMore {
		t.Errorf("the capped list of %d proposals does not say more exist", inbox.Count)
	}
}
