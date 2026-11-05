// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a unit's feed does to a field a user last edited. The user edits
// through the product's own stores, so the audit trail the rule reads is the
// one a user's edit leaves.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func (e *crmEnv) allowOverwrite(t *testing.T, on bool) {
	t.Helper()
	if err := identity.SetExtensionOverwritesEditedFields(e.Admin(), NewSettingsStore(e.Pool), ingressUnit, on); err != nil {
		t.Fatalf("saving the overwrite setting: %v", err)
	}
}

func (e *crmEnv) companyEditedByAUser(t *testing.T) extension.Result {
	t.Helper()
	first := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.MustParse(first.Ref.ID)),
		contacts.UpdateCompanyInput{Industry: new("automotive")}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}
	return first
}

func TestAFeedWithholdsAFieldAUserEditedAndSaysSo(t *testing.T) {
	e := setupCRMIngress(t)
	first := e.companyEditedByAUser(t)

	second := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{
		DisplayName: "Acme", Industry: "education", Description: "from the feed",
	}))
	if second.Ref.ID != first.Ref.ID {
		t.Fatalf("second ingest made %s, want the same company %s", second.Ref.ID, first.Ref.ID)
	}
	if !slices.Equal(second.Withheld, []string{"industry"}) {
		t.Errorf("withheld = %v, want [industry]", second.Withheld)
	}
	industry, description, _, _ := e.companyColumns(t, first.Ref.ID)
	if industry == nil || *industry != "automotive" || description == nil || *description != "from the feed" {
		t.Errorf("industry/description = %v/%v, want the user's value kept and the other field written", industry, description)
	}
}

func TestAFeedAllowedToOverwriteReplacesAFieldAUserEdited(t *testing.T) {
	e := setupCRMIngress(t)
	first := e.companyEditedByAUser(t)
	e.allowOverwrite(t, true)

	res := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Industry: "education"}))
	if len(res.Withheld) != 0 {
		t.Errorf("withheld = %v, want none once the unit may overwrite", res.Withheld)
	}
	if industry, _, _, _ := e.companyColumns(t, first.Ref.ID); industry == nil || *industry != "education" {
		t.Errorf("industry = %v, want the feed's value", industry)
	}

	e.allowOverwrite(t, false)
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.MustParse(first.Ref.ID)),
		contacts.UpdateCompanyInput{Industry: new("agriculture")}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}
	res = e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme", Industry: "construction"}))
	if !slices.Equal(res.Withheld, []string{"industry"}) {
		t.Errorf("withheld = %v after switching the setting off, want [industry]", res.Withheld)
	}
}

func TestAFeedWithholdsADealAmountAndItsCurrencyTogether(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, e.dealRecord("opp-1", nil))
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](ids.MustParse(made.Ref.ID)),
		deals.UpdateDealInput{AmountMinor: new(int64(250)), Currency: new("EUR")}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}

	res := e.mustIngest(t, e.dealRecord("opp-1", func(d *extension.DealFields) {
		d.AmountMinor, d.Currency = new(int64(999)), "USD"
	}))
	if !slices.Equal(res.Withheld, []string{"amount_minor", "currency"}) {
		t.Errorf("withheld = %v, want the amount and its currency together", res.Withheld)
	}
	var amount int64
	var currency string
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT amount_minor, currency FROM deal WHERE id = $1`, ids.MustParse(made.Ref.ID)).
			Scan(&amount, &currency)
	})
	if amount != 250 || currency != "EUR" {
		t.Errorf("amount/currency = %d/%s, want the user's 250 EUR kept", amount, currency)
	}
}

// The feed overwrote the amount after a user set the figure. The amount's
// last writer is the feed, and the currency's is still the user.
func TestAFeedWithholdsTheAmountWhenTheUserLastWroteOnlyTheCurrency(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, e.dealRecord("opp-1", nil))
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](ids.MustParse(made.Ref.ID)),
		deals.UpdateDealInput{AmountMinor: new(int64(250)), Currency: new("EUR")}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}
	e.allowOverwrite(t, true)
	e.mustIngest(t, e.dealRecord("opp-1", func(d *extension.DealFields) { d.AmountMinor = new(int64(700)) }))
	e.allowOverwrite(t, false)

	res := e.mustIngest(t, e.dealRecord("opp-1", func(d *extension.DealFields) { d.AmountMinor = new(int64(999)) }))
	if !slices.Equal(res.Withheld, []string{"amount_minor", "currency"}) {
		t.Errorf("withheld = %v, want both halves of the figure", res.Withheld)
	}
	var amount int64
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT amount_minor FROM deal WHERE id = $1`, ids.MustParse(made.Ref.ID)).Scan(&amount)
	})
	if amount != 700 {
		t.Errorf("amount = %d, want the earlier 700 kept beside the user's currency", amount)
	}
}

func TestChangingTheOverwriteSettingIsAudited(t *testing.T) {
	e := setupCRMIngress(t)
	e.allowOverwrite(t, true)
	got := e.countAsWorkspace(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'installation_settings'
		   AND after ? 'identity.extension_overwrites_edited_fields'`)
	if got != 1 {
		t.Errorf("audit rows for the overwrite setting = %d, want 1", got)
	}
}
