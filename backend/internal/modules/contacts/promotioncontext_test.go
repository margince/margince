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

// reviewedPromotion is the rest of what a promotion's digest covers.
type reviewedPromotion struct {
	trigger  string
	evidence *ids.ActivityID
	target   ids.ContactID
}

func mustDigest(t *testing.T, facts promotionEmployerFacts, p reviewedPromotion) string {
	t.Helper()
	digest, err := facts.digest(p.trigger, p.evidence, p.target)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestEveryFactThePromotionDecidesOnMovesTheDigest(t *testing.T) {
	evidence := ids.From[ids.ActivityKind](ids.MustParse("0192f3a0-0000-7000-8000-0000000000e1"))
	asReviewed := func() reviewedPromotion { return reviewedPromotion{trigger: "meeting_booked", evidence: &evidence} }
	reviewed := mustDigest(t, reviewedPromotionFacts(), asReviewed())
	changes := map[string]func(f *promotionEmployerFacts, p *reviewedPromotion){
		"format":           func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Format = "employer.v2" },
		"linked company":   func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.LinkedCompany = new(ids.NewV7()) },
		"create company":   func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.CreateCompany = true },
		"typed name":       func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.TypedName = "Northwind Ltd" },
		"address unread":   func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain = nil },
		"host":             func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain.Host = "northwind.test" },
		"registrable":      func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain.Domain = "" },
		"consumer":         func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain.Consumer = true },
		"suppressed":       func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain.Suppressed = true },
		"company taken":    func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain.Owners = f.Domain.Owners[:1] },
		"owner reassigned": func(f *promotionEmployerFacts, _ *reviewedPromotion) { f.Domain.Owners[1].OwnerID = new(ids.NewV7()) },
		"trigger":          func(_ *promotionEmployerFacts, p *reviewedPromotion) { p.trigger = "meeting_held" },
		"evidence":         func(_ *promotionEmployerFacts, p *reviewedPromotion) { p.evidence = nil },
		"merge target":     func(_ *promotionEmployerFacts, p *reviewedPromotion) { p.target = ids.New[ids.ContactKind]() },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			facts, promotion := reviewedPromotionFacts(), asReviewed()
			change(&facts, &promotion)
			if mustDigest(t, facts, promotion) == reviewed {
				t.Errorf("changing the %s left the digest as reviewed", name)
			}
		})
	}
}

func TestTheOrderOwnersAreReadInLeavesTheDigest(t *testing.T) {
	facts := reviewedPromotionFacts()
	reversed := reviewedPromotionFacts()
	reversed.Domain.Owners[0], reversed.Domain.Owners[1] = reversed.Domain.Owners[1], reversed.Domain.Owners[0]
	qualified := reviewedPromotion{trigger: "human_qualify"}
	if mustDigest(t, facts, qualified) != mustDigest(t, reversed, qualified) {
		t.Error("the same owners read in another order moved the digest")
	}
	if facts.Domain.Owners[0].CompanyID != reviewedPromotionFacts().Domain.Owners[0].CompanyID {
		t.Error("digesting reordered the caller's owners")
	}
}
