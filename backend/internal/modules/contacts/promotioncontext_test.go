// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func reviewedPromotionFacts() promotionEmployerFacts {
	return promotionEmployerFacts{
		Format: promotionContextFormat, TypedName: "Northwind",
		Domain: &employerDomainFacts{Host: "mail.northwind.test", Domain: "northwind.test", Owners: []domainOwnerFacts{
			{CompanyID: ids.MustParse("0192f3a0-0000-7000-8000-000000000001"), OwnerID: new(ids.MustParse("0192f3a0-0000-7000-8000-0000000000aa"))},
			{CompanyID: ids.MustParse("0192f3a0-0000-7000-8000-000000000002")},
		}},
	}
}

func mustDigest(t *testing.T, facts promotionEmployerFacts, trigger string, evidence *ids.ActivityID) string {
	t.Helper()
	digest, err := facts.digest(trigger, evidence)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestEveryFactThePromotionDecidesOnMovesTheDigest(t *testing.T) {
	evidence := ids.From[ids.ActivityKind](ids.MustParse("0192f3a0-0000-7000-8000-0000000000e1"))
	reviewed := mustDigest(t, reviewedPromotionFacts(), "meeting_booked", &evidence)
	changes := map[string]func(f *promotionEmployerFacts, trigger *string, evidence **ids.ActivityID){
		"format":         func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Format = "employer.v2" },
		"linked company": func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.LinkedCompany = new(ids.NewV7()) },
		"create company": func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.CreateCompany = true },
		"typed name":     func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.TypedName = "Northwind Ltd" },
		"address unread": func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Domain = nil },
		"host":           func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Domain.Host = "northwind.test" },
		"registrable":    func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Domain.Domain = "" },
		"consumer":       func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Domain.Consumer = true },
		"suppressed":     func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Domain.Suppressed = true },
		"company taken":  func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) { f.Domain.Owners = f.Domain.Owners[:1] },
		"owner reassigned": func(f *promotionEmployerFacts, _ *string, _ **ids.ActivityID) {
			f.Domain.Owners[1].OwnerID = new(ids.NewV7())
		},
		"trigger":  func(_ *promotionEmployerFacts, trigger *string, _ **ids.ActivityID) { *trigger = "meeting_held" },
		"evidence": func(_ *promotionEmployerFacts, _ *string, evidence **ids.ActivityID) { *evidence = nil },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			facts, trigger, cited := reviewedPromotionFacts(), "meeting_booked", &evidence
			change(&facts, &trigger, &cited)
			if mustDigest(t, facts, trigger, cited) == reviewed {
				t.Errorf("changing the %s left the digest as reviewed", name)
			}
		})
	}
}

func TestTheOrderOwnersAreReadInLeavesTheDigest(t *testing.T) {
	facts := reviewedPromotionFacts()
	reversed := reviewedPromotionFacts()
	reversed.Domain.Owners[0], reversed.Domain.Owners[1] = reversed.Domain.Owners[1], reversed.Domain.Owners[0]
	if mustDigest(t, facts, "human_qualify", nil) != mustDigest(t, reversed, "human_qualify", nil) {
		t.Error("the same owners read in another order moved the digest")
	}
	if facts.Domain.Owners[0].CompanyID != reviewedPromotionFacts().Domain.Owners[0].CompanyID {
		t.Error("digesting reordered the caller's owners")
	}
}
