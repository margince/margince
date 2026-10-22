// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestStaleCommercialPreviewRefusesEveryDecisionEntryPoint(t *testing.T) {
	for _, door := range []string{"single", "edited", "policy", "bundle"} {
		t.Run(door, func(t *testing.T) {
			e := integration.Setup(t)
			company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Harbor", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":null}}`, company.Id)))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			bundle := ids.NewV7()
			id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: ids.UUID(company.Id), ProposedChange: raw, DiffHash: hash, JoinPending: true, BundleID: bundle})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.UUID(company.Id)), contacts.UpdateCompanyInput{DisplayName: new("Harbor renamed")}); err != nil {
				t.Fatal(err)
			}
			switch door {
			case "single":
				_, err = svc.Decide(e.Admin(), id, true, nil)
			case "edited":
				_, err = svc.DecideEdited(e.Admin(), id, raw)
			case "policy":
				_, err = svc.ApplyUnderPolicy(e.Admin(), id)
			case "bundle":
				members, bundleErr := svc.DecideBundle(e.Admin(), bundle, true, nil)
				if bundleErr != nil || len(members) != 1 || members[0].Outcome != approvals.BundleRefused {
					t.Fatalf("bundle outcomes: %+v, %v", members, bundleErr)
				}
			}
			attempt := notices.EmailAttempt{ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}}
			if _, _, send, err := refreshApprovalNotice(e.Admin(), e.Pool, attempt, time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC), forSingleEmail); err != nil || send {
				t.Fatalf("stale decision mail: send=%v, %v", send, err)
			}
			if door != "bundle" && !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("stale preview decision: %v", err)
			}
			stored, err := svc.GetWire(e.Admin(), id)
			if err != nil || stored.Status != "pending" || stored.Preview == nil || stored.Preview.State != "stale" {
				t.Fatalf("refused proposal: %+v, %v", stored, err)
			}
		})
	}
}

func TestRestagingRefreshesStalePendingContextWithoutForgettingDeclines(t *testing.T) {
	for _, door := range []string{"agent", "join", "unless_declined"} {
		t.Run(door, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			company := e.SeedCompany(t, "Harbor", &e.AdminUser)
			raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":null}}`, company)))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			input := approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash, JoinPending: true}
			stage := func() ids.ApprovalID {
				t.Helper()
				var id ids.ApprovalID
				var err error
				switch door {
				case "agent":
					id, _, err = svc.StageAgentCall(ctx, input)
				case "join":
					id, err = svc.Stage(ctx, input)
				case "unless_declined":
					id, _, err = svc.StageUnlessDeclined(ctx, input)
				}
				if err != nil {
					t.Fatal(err)
				}
				return id
			}
			first := stage()
			if same := stage(); same != first {
				t.Fatal("unchanged context was not joined")
			}
			if _, err := e.Contacts.UpdateCompany(ctx, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{DisplayName: new("Harbor renamed")}); err != nil {
				t.Fatal(err)
			}
			fresh := stage()
			if fresh == first || fresh.IsZero() {
				t.Fatal("stale proposal was joined")
			}
			old, err := svc.GetWire(ctx, first)
			if err != nil || old.Status != "expired" {
				t.Fatalf("old proposal: %+v, %v", old, err)
			}
			current, err := svc.GetWire(ctx, fresh)
			if err != nil || current.Preview == nil || current.Preview.State != "ready" {
				t.Fatalf("fresh proposal: %+v, %v", current.Preview, err)
			}
			if _, err := svc.Decide(ctx, fresh, false, nil); err != nil {
				t.Fatal(err)
			}
			if door == "unless_declined" && !stage().IsZero() {
				t.Fatal("restaging forgot a human decline")
			}
		})
	}
}

func TestBundlePreflightReleasesItsConnectionAndPropagatesInfrastructureFailure(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Harbor", &e.AdminUser)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":null}}`, company)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	bundle := ids.NewV7()
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash, BundleID: bundle})
	if err != nil {
		t.Fatal(err)
	}
	outage := errors.New("preview store unavailable")
	called := false
	svc.WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		called = true
		if held := e.Pool.Stat().AcquiredConns(); held != 0 {
			t.Errorf("bundle held %d connections during external preflight", held)
		}
		return nil, outage
	})
	if _, err := svc.DecideBundle(ctx, bundle, true, nil); !errors.Is(err, outage) {
		t.Fatalf("bundle hid infrastructure failure: %v", err)
	}
	if !called {
		t.Fatal("preview was not checked")
	}
	stored, err := svc.Get(ctx, id)
	if err != nil || stored.Status != "pending" {
		t.Fatalf("failed preflight decided a member: %+v, %v", stored, err)
	}
	svc.WithPreview(newApprovalPreviewReader(e.DB()).read)
	members, err := svc.DecideBundle(ctx, bundle, true, nil)
	if err != nil || len(members) != 1 || members[0].Outcome != approvals.BundleDecided {
		t.Fatalf("retry after recovery: %+v, %v", members, err)
	}
}

func TestCommittedDeclinesSurvivePreviewReadFailure(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(fmt.Sprint(bundled), func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			company := e.SeedCompany(t, "Harbor", &e.AdminUser)
			raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"record_type":"company","id":%q,"fields":{"owner_id":null}}`, company)))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			bundle := ids.NewV7()
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "update_record", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash, BundleID: bundle})
			if err != nil {
				t.Fatal(err)
			}
			svc.WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
				return nil, errors.New("preview reader unavailable")
			})
			var result crmcontracts.Approval
			if bundled {
				members, err := svc.DecideBundleWire(ctx, bundle, false, nil)
				if err != nil || len(members) != 1 {
					t.Fatalf("decline response: %+v, %v", members, err)
				}
				result = members[0].Approval
			} else {
				result, err = svc.DecideWire(ctx, id, false, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			stored, err := svc.Get(ctx, id)
			if err != nil || stored.Status != "rejected" || result.Status != "rejected" || result.Preview == nil || result.Preview.State != "unavailable" {
				t.Fatalf("committed decline lost: stored=%+v response=%+v err=%v", stored, result, err)
			}
		})
	}
}
