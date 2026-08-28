// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// partner_org_id and partner_attribution are one fact in two columns, and the
// deal_partner_attribution_pairing CHECK is what makes that true of the stored
// row rather than only of the code that writes it. These tests go through the
// real writer against a real database, because the store's refusals and the
// schema's are two different guarantees: the store one can be bypassed by a
// future caller, the CHECK cannot, and only Postgres can tell us the second is
// really there.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// execDirect runs one statement against the deal table outside the store, to
// ask the DATABASE what it refuses rather than what the store refuses.
func execDirect(t *testing.T, e *Env, sql string, args ...any) error {
	t.Helper()
	return e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	})
}

// seedDealWithPartner links a live deal to a partner organization through the
// real writer and hands back both ids.
func seedDealWithPartner(t *testing.T, e *Env) (ids.DealID, ids.OrganizationID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	partnerOrg := orgIDOf(e.SeedPartnerOrg(t, "Northgate Partners", nil, nil))
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1))
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		PartnerOrganizationID: &partnerOrg,
	}); err != nil {
		t.Fatalf("linking the deal to its partner: %v", err)
	}
	return deal, partnerOrg
}

func TestNamingAPartnerStoresTheSourcedClaimWithIt(t *testing.T) {
	e := Setup(t)
	deal, partner := seedDealWithPartner(t, e)

	got, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil {
		t.Fatalf("reading the deal back: %v", err)
	}
	if got.PartnerOrgId == nil || ids.UUID(*got.PartnerOrgId) != partner.UUID {
		t.Fatalf("partner_org_id = %v, want the partner just linked", got.PartnerOrgId)
	}
	if got.PartnerAttribution == nil || *got.PartnerAttribution != "sourced" {
		t.Errorf("partner_attribution = %v, want \"sourced\" — a bare partner link is the sourced motion", got.PartnerAttribution)
	}
}

func TestADealCanBeReAttributedToInfluencedWithoutMovingThePartner(t *testing.T) {
	e := Setup(t)
	deal, partner := seedDealWithPartner(t, e)
	influenced := crmcontracts.DealPartnerAttribution("influenced")

	claim := string(influenced)
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		PartnerAttribution: &claim,
	}); err != nil {
		t.Fatalf("re-attributing the deal: %v", err)
	}

	got, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil {
		t.Fatalf("reading the deal back: %v", err)
	}
	if got.PartnerAttribution == nil || *got.PartnerAttribution != influenced {
		t.Errorf("partner_attribution = %v, want %q", got.PartnerAttribution, influenced)
	}
	if got.PartnerOrgId == nil || ids.UUID(*got.PartnerOrgId) != partner.UUID {
		t.Errorf("partner_org_id = %v, want the partner to have stayed put", got.PartnerOrgId)
	}
}

func TestAttributingADealThatNamesNoPartnerIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Direct renewal", pipeline, open, &e.Rep1))
	sourced := "sourced"

	_, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		PartnerAttribution: &sourced,
	})

	if err == nil {
		t.Fatal("attributing a deal with no partner succeeded; the stored row would breach the pairing CHECK")
	}
	var unpaired *deals.PartnerAttributionUnpairedError
	if !errors.As(err, &unpaired) {
		t.Fatalf("error = %v, want PartnerAttributionUnpairedError", err)
	}
}

// The pairing CHECK is the guarantee the store's own refusal cannot provide.
// Writing the half-set row directly proves the constraint is on the table, so a
// future writer that forgets the rule is stopped by the database.
func TestTheDatabaseItselfRefusesAHalfSetPartnerPair(t *testing.T) {
	e := Setup(t)
	deal, _ := seedDealWithPartner(t, e)

	err := execDirect(t, e,
		`UPDATE deal SET partner_attribution = NULL WHERE id = $1`, deal)

	if err == nil {
		t.Fatal("clearing the attribution while the partner stayed succeeded; the pairing CHECK is not on the table")
	}
}

// Deleting a partner organization must detach its deals rather than fail: the
// FK clears partner_org_id, and without the trigger that clears the attribution
// with it the delete would breach the pairing CHECK instead of succeeding.
func TestDeletingAPartnerOrganizationDetachesItsDealsIntact(t *testing.T) {
	e := Setup(t)
	deal, partner := seedDealWithPartner(t, e)

	if err := execDirect(t, e, `DELETE FROM organization WHERE id = $1`, partner); err != nil {
		t.Fatalf("deleting the partner organization: %v", err)
	}

	got, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil {
		t.Fatalf("reading the orphaned deal back: %v", err)
	}
	if got.PartnerOrgId != nil || got.PartnerAttribution != nil {
		t.Errorf("deal kept partner %v / attribution %v after its partner was deleted; both halves leave together",
			got.PartnerOrgId, got.PartnerAttribution)
	}
}

// The clear surface names the pair ONCE, as `partner_org_id`, and forgetting the
// partner forgets what they did. Routed through the single-column clear path
// instead, this write would set one half and earn the constraint violation the
// test above proves is really there.
func TestForgettingADealsPartnerForgetsItsClaimToo(t *testing.T) {
	e := Setup(t)
	deal, _ := seedDealWithPartner(t, e)

	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		Clear: []string{"partner_org_id"},
	}); err != nil {
		t.Fatalf("clearing the deal's partner: %v", err)
	}

	got, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil {
		t.Fatalf("reading the deal back: %v", err)
	}
	if got.PartnerOrgId != nil || got.PartnerAttribution != nil {
		t.Errorf("deal kept partner %v / attribution %v; both halves leave together",
			got.PartnerOrgId, got.PartnerAttribution)
	}
}

func TestForgettingThePartnerWhileClaimingSomethingOfThemIsRefused(t *testing.T) {
	e := Setup(t)
	deal, _ := seedDealWithPartner(t, e)
	influenced := "influenced"

	_, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		Clear:              []string{"partner_org_id"},
		PartnerAttribution: &influenced,
	})

	var unpaired *deals.PartnerAttributionUnpairedError
	if !errors.As(err, &unpaired) {
		t.Fatalf("error = %v, want PartnerAttributionUnpairedError — the request forgets the partner the claim describes", err)
	}
}

// Naming the claim forgets the whole pair, because there is no state where a
// deal carries a partner it claims nothing about. A restore reverting a
// partner-add names both halves as null, and refusing either would leave that
// reversal impossible to express.
func TestNamingTheClaimForgetsTheWholePair(t *testing.T) {
	e := Setup(t)
	deal, _ := seedDealWithPartner(t, e)

	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		Clear: []string{"partner_attribution"},
	}); err != nil {
		t.Fatalf("clearing the deal's claim about its partner: %v", err)
	}

	got, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil {
		t.Fatalf("reading the deal back: %v", err)
	}
	if got.PartnerOrgId != nil || got.PartnerAttribution != nil {
		t.Errorf("deal kept partner %v / attribution %v; both halves leave together",
			got.PartnerOrgId, got.PartnerAttribution)
	}
}

// Forgetting a link is a write ABOUT the record it points at, so it carries the
// permission naming that record would. Without this a rep who was not shown the
// deal's partner — masked_fields says so — could still destroy the attribution a
// commission accrues on.
func TestForgettingAPartnerTheReaderCannotSeeIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	partnerOrg := e.SeedPartnerOrg(t, "Northgate Partners", nil, &e.Rep3)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1))
	partnerID := orgIDOf(partnerOrg)
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		PartnerOrganizationID: &partnerID,
	}); err != nil {
		t.Fatalf("linking the deal to its partner: %v", err)
	}
	e.MakeCapturePrivate(t, "organization", partnerOrg, e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	_, err := e.Deals.UpdateDeal(rep, deal, deals.UpdateDealInput{
		Clear: []string{"partner_org_id"},
	})

	if err == nil {
		t.Fatal("a reader who cannot open the partner cleared it; that is a write about an organization they may not name")
	}
	// Not-found rather than forbidden: existence stays hidden, which is what
	// EnsureLinkTarget answers on the set path too.
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	// Read off the ROW, not through the store: the same masking that withholds
	// this partner from the rep withholds it from any reader whose scope cannot
	// reach the capture-private organization, so a store read here cannot tell a
	// surviving link from a cleared one.
	partner, claim := partnerPairOnTheRow(t, e, deal)
	if partner == nil || claim == nil {
		t.Errorf("refused clear still moved the pair: partner %v / attribution %v", partner, claim)
	}
}

// partnerPairOnTheRow reads both halves straight off the deal row.
func partnerPairOnTheRow(t *testing.T, e *Env, deal ids.DealID) (partner, claim *string) {
	t.Helper()
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT partner_org_id::text, partner_attribution FROM deal WHERE id = $1`,
			deal).Scan(&partner, &claim)
	}); err != nil {
		t.Fatalf("reading the deal's partner pair off the row: %v", err)
	}
	return partner, claim
}

// The company arm of the same rule.
func TestForgettingACompanyTheReaderCannotSeeIsRefused(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	org := e.SeedOrg(t, "Meridian Labs", &e.Rep3)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Meridian renewal", pipeline, open, &e.Rep1))
	orgID := orgIDOf(org)
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		OrganizationID: &orgID,
	}); err != nil {
		t.Fatalf("linking the deal to its company: %v", err)
	}
	e.MakeCapturePrivate(t, "organization", org, e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	_, err := e.Deals.UpdateDeal(rep, deal, deals.UpdateDealInput{
		Clear: []string{"organization_id"},
	})

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound — the company is one this reader was never shown", err)
	}
}

// A deal without a company is an ordinary deal — the column is nullable and a
// deal is created without one — so the nullability crm.yaml declares has to be
// reachable. It was not: the edit form offers unsetting the company, and the
// store refused the null it sent.
func TestADealCanBeUnlinkedFromItsCompany(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Kestrel renewal", pipeline, open, &e.Rep1))
	org := orgIDOf(e.SeedOrg(t, "Kestrel Foods", nil))
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		OrganizationID: &org,
	}); err != nil {
		t.Fatalf("linking the deal to its company: %v", err)
	}

	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{
		Clear: []string{"organization_id"},
	}); err != nil {
		t.Fatalf("unlinking the deal from its company: %v", err)
	}

	got, err := e.Deals.GetDeal(e.Admin(), deal, 0)
	if err != nil {
		t.Fatalf("reading the deal back: %v", err)
	}
	if got.OrganizationId != nil {
		t.Errorf("organization_id = %v, want nil", got.OrganizationId)
	}
}
