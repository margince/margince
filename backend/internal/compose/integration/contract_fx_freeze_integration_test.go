// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contract freezes its conversion at activation, which is what makes its
// base-currency value the one the parties agreed to rather than the one a
// report happens to be run on.
//
// The schema documented the freeze and nothing wrote it, so every activated
// foreign-currency contract carried NULL — and the base-currency freeze guard
// counts a contract by its frozen rate, so an installation holding only
// contract rows could still change its base currency and silently restate them.

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedRate loads a published rate the freeze can find.
func seedRate(t *testing.T, e *Env, from, to string, rate string, on time.Time) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
			 VALUES ($1, $2, $3::numeric, $4)
			 ON CONFLICT (from_currency, to_currency, rate_date) DO UPDATE SET rate = EXCLUDED.rate`,
			from, to, rate, on.UTC().Truncate(24*time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// draftInCurrency stages a contract carrying a value in the named currency.
func draftInCurrency(t *testing.T, e *Env, org ids.UUID, currency string) ids.ContractID {
	t.Helper()
	value := int64(250_000)
	created, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		OrganizationID: ids.From[ids.OrganizationKind](org),
		Title:          "A foreign-currency agreement",
		ValueMinor:     &value,
		Currency:       &currency,
		ValueBasis:     contracts.BasisTotal,
		Source:         "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ContractKind](ids.UUID(created.Id))
}

func TestActivationFreezesTheContractsConversion(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", nil)
	seedRate(t, e, "USD", "EUR", "0.9", time.Now())

	id := draftInCurrency(t, e, org, "USD")
	activated, err := e.Contracts.ChangeStatus(e.Admin(), id, contracts.StatusActive, nil)
	if err != nil {
		t.Fatalf("activating a contract whose rate is published: %v", err)
	}

	if activated.FxRateToBase == nil {
		t.Fatal("an activated foreign-currency contract carries no frozen rate — the base-currency " +
			"freeze guard counts a contract by that rate, so this one is invisible to it")
	}
	// Compared as a NUMBER, not as text: the column is numeric with a scale, so
	// the rate reads back as 0.9000000000 — the same form a closing deal
	// freezes, and a text comparison here would be asserting the scale rather
	// than the rate.
	frozen, err := strconv.ParseFloat(*activated.FxRateToBase, 64)
	if err != nil {
		t.Fatalf("the frozen rate %q does not parse as a number: %v", *activated.FxRateToBase, err)
	}
	if frozen != 0.9 {
		t.Errorf("the frozen rate is %v, want the published 0.9", frozen)
	}
	// Both, always together: a rate without its date is not a frozen
	// conversion, which is what contract_fx_pair holds in the schema.
	if activated.FxRateDate == nil {
		t.Error("the rate was frozen without the day it is the rate FOR")
	}
}

// The freeze is at ACTIVATION and never re-read: a contract that already
// carries a rate keeps it, so re-activating after a cancellation does not
// re-price an agreement nobody renegotiated.
func TestReActivationDoesNotRePriceTheAgreement(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", nil)
	seedRate(t, e, "USD", "EUR", "0.9", time.Now())

	id := draftInCurrency(t, e, org, "USD")
	first, err := e.Contracts.ChangeStatus(e.Admin(), id, contracts.StatusActive, nil)
	if err != nil {
		t.Fatalf("activating: %v", err)
	}

	// The market moves.
	seedRate(t, e, "USD", "EUR", "0.5", time.Now())

	again, err := e.Contracts.ChangeStatus(e.Admin(), id, contracts.StatusActive, nil)
	if err != nil {
		t.Fatalf("re-asserting the status: %v", err)
	}
	if again.FxRateToBase == nil || first.FxRateToBase == nil || *again.FxRateToBase != *first.FxRateToBase {
		t.Errorf("the frozen rate moved to %v — it is what the agreement was worth when it was "+
			"made, not what it is worth when somebody re-asserts its status", again.FxRateToBase)
	}
}

// No rate to freeze is a refusal, not a NULL. The alternative is an activated
// contract the freeze guard cannot count, which is the state this exists to
// end.
func TestActivationRefusesWhenNoRateIsPublished(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", nil)

	id := draftInCurrency(t, e, org, "JPY")
	_, err := e.Contracts.ChangeStatus(e.Admin(), id, contracts.StatusActive, nil)

	var missing *deals.MissingFxRateError
	if !errors.As(err, &missing) {
		t.Fatalf("activating with no published rate answered %v, want the missing-rate refusal", err)
	}

	// And nothing moved: a refused activation leaves the contract a draft.
	read, err := e.Contracts.GetContract(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if read.Status == nil || string(*read.Status) != contracts.StatusDraft {
		t.Errorf("the contract is %v after a refused activation, want it left as a draft", read.Status)
	}
}

// A contract with no currency has nothing to convert, and freezing nothing is
// the right answer rather than a refusal.
func TestAContractWithNoCurrencyActivatesWithoutARate(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Acme", nil)

	created, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		OrganizationID: ids.From[ids.OrganizationKind](org),
		Title:          "An agreement with no money in it",
		ValueBasis:     contracts.BasisTotal,
		Source:         "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContractKind](ids.UUID(created.Id))

	activated, err := e.Contracts.ChangeStatus(e.Admin(), id, contracts.StatusActive, nil)
	if err != nil {
		t.Fatalf("activating a contract with no currency: %v", err)
	}
	if activated.FxRateToBase != nil {
		t.Errorf("a contract with nothing to convert froze %q", *activated.FxRateToBase)
	}
}
