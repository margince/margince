// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"time"
)

func TestForwardedBindingReviewUsesStoredDossierEvidence(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	domain := "review-source.test"
	e.openTriageFirst(ctx, t, "hello@"+domain, "Sender", domain)
	readID := e.startTriageRead(ctx, t, domain)
	claim, err := e.store.BeginSiteRead(ctx, readID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{Domain: domain, Status: DomainCompany, Source: DomainSourceSiteRead, DossierName: "Source Company", SeedURL: "https://" + domain, ReadID: readID})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.CompanyID == nil {
		t.Fatal("company was not created")
	}
	if err := e.store.FinishSiteRead(ctx, readID, FinishSiteReadInput{Status: "done", ClaimedAt: &claim.ClaimedAt, Pages: []SiteReadPage{{Kind: "home", URL: "https://another-company.test/"}}}); err != nil {
		t.Fatal(err)
	}
	candidates, _, err := e.store.ForwardedTriageBindings(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Domain != domain || candidates[0].CompanyID != *resolved.CompanyID || candidates[0].ReadID != readID {
		t.Fatalf("candidates = %+v", candidates)
	}
	after, _, err := e.store.ForwardedTriageBindings(ctx, domain, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("continuation repeated candidates: %+v", after)
	}
	if _, err := e.store.UpdateCompany(ctx, *resolved.CompanyID, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	hidden, cursor, err := e.store.ForwardedTriageBindings(e.asOther(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 0 || cursor != "" {
		t.Fatalf("private binding leaked through rows or cursor: rows=%+v cursor=%q", hidden, cursor)
	}

}

func TestForwardedIdentityRequiresExplicitMatchAndKeepsProvenance(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	target := e.resolveSiteVerdict(ctx, t, "target@destination.test", "destination.test", "Destination Company")
	domain := "alias.test"
	e.openTriageFirst(ctx, t, "sender@"+domain, "Sender", domain)
	readID := e.startTriageRead(ctx, t, domain)
	proof := DomainForwardEvidence{ReadID: readID, RequestedDomain: domain, EffectiveURL: "https://destination.test/", RedirectChain: []string{"https://alias.test/", "https://destination.test/"}}
	if err := e.store.WithholdForwardedDomain(ctx, proof); err != nil {
		t.Fatal(err)
	}
	input := ResolveDomainTriageInput{Domain: domain, Status: DomainCompany, Source: DomainSourceHuman}
	if _, err := e.store.ResolveDomainTriage(ctx, input); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("unmatched keep = %v", err)
	}
	input.CompanyID = target.CompanyID
	matched, err := e.store.ResolveDomainTriage(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if matched.CompanyCreated || matched.CompanyID == nil || *matched.CompanyID != *target.CompanyID {
		t.Fatalf("explicit match = %+v", matched)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		row, known, err := readDispositionTx(ctx, tx, domain)
		if err != nil {
			return err
		}
		if !known || row.SiteReadID == nil || *row.SiteReadID != readID {
			t.Fatalf("read provenance = %+v", row)
		}
		var stored DomainForwardEvidence
		if err := json.Unmarshal([]byte(row.Evidence), &stored); err != nil {
			return err
		}
		if stored.EffectiveURL != proof.EffectiveURL || len(stored.RedirectChain) != 2 {
			t.Fatalf("redirect provenance = %+v", stored)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
