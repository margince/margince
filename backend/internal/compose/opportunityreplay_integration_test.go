// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBuyerReattributionDoesNotDuplicateTheSourceFinding(t *testing.T) {
	e := setupScout(t)
	partner := e.SeedCompany(t, "Cedar Partner", nil)
	buyerCompany := e.SeedCompany(t, "Maple Studio", nil)
	source, buyer := e.buyingEmail(t, buyerCompany, e.daysAgo(2))
	ref := "contact:" + buyer.String()
	quote := "We need a platform migration."
	thread := settledThread{CompanyID: partner, Messages: []threadMessage{{
		ID: source, Body: quote, Parties: []commitmentParty{{Ref: ref, Contact: &buyer, Role: "cc"}},
	}}}
	event := extractedEvent{
		Kind: extractKindOpportunity, MessageID: source.String(),
		Summary: "Maple needs a migration.", Quote: quote, BuyerRef: ref, CommercialRole: commercialRoleBuyer,
	}
	for pass := range 2 {
		if pass == 1 {
			contactID, companyID := ids.From[ids.ContactKind](buyer), ids.From[ids.CompanyKind](partner)
			if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
				Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual",
			}); err != nil {
				t.Fatal(err)
			}
		}
		err := database.WithWorkspaceTx(e.system(), e.Pool, func(tx pgx.Tx) error {
			raised, err := recordEventSignal(e.system(), tx, thread, event, source, e.now, CommitmentRemembered)
			if err == nil && raised != (pass == 0) {
				t.Fatalf("pass %d raised=%v", pass, raised)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if count := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind = 'new_opportunity'`); count != 1 {
		t.Fatalf("source findings=%d, want one", count)
	}
}

func TestCommercialIntermediariesDoNotCreateBuyingReviewMatters(t *testing.T) {
	e := setupScout(t)
	for _, role := range []string{commercialRoleSupplier, commercialRolePartner, commercialRoleProvider} {
		t.Run(role, func(t *testing.T) {
			company := e.SeedCompany(t, role+" company", nil)
			source, contact := e.buyingEmail(t, company, e.daysAgo(2))
			ref := "contact:" + contact.String()
			thread := settledThread{CompanyID: company, Messages: []threadMessage{{
				ID: source, Body: "We need a platform migration.",
				Parties: []commitmentParty{{Ref: ref, Contact: &contact, Role: "to"}},
			}}}
			event := extractedEvent{
				Kind: extractKindOpportunity, MessageID: source.String(),
				Summary: "A commercial intermediary described work.", Quote: thread.Messages[0].Body,
				BuyerRef: ref, CommercialRole: role,
			}
			err := database.WithWorkspaceTx(e.system(), e.Pool, func(tx pgx.Tx) error {
				_, err := recordEventSignal(e.system(), tx, thread, event, source, e.now, CommitmentRemembered)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
			if pass := e.pass(t); pass.Raised != 0 {
				t.Fatalf("%s raised %d deal proposals", role, pass.Raised)
			}
		})
	}
	store := assurance.NewStore(InstallationDB(e.Pool))
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig()).WithSignalSubjects(AssuranceSignalSubjects)
	if _, err := scanner.Scan(e.system(), e.now, nil); err != nil {
		t.Fatal(err)
	}
	if found := readSignalFindings(e.signalReader(), t, e); len(found) != 0 {
		t.Fatalf("intermediaries created buying findings: %+v", found)
	}
}
