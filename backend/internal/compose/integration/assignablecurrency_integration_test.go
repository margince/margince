// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A currency newly set or changed must be an assigned ISO-4217 code. A value
// already stored outside the list stays editable, because the schema only ever
// checked the three-letter shape.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func requireUnassigned(t *testing.T, what string, err error) {
	t.Helper()
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Field != "currency" || parse.Code != "currency_unassigned" {
		t.Errorf("%s = %v, want a currency_unassigned refusal", what, err)
	}
}

func TestADealStoredInAnUnassignedCurrencyStaysEditable(t *testing.T) {
	e := Setup(t)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Currency fixture', false, 93)`, pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, stage, pipeline)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Legacy currency", ids.PipelineID{UUID: pipeline}, ids.StageID{UUID: stage}, nil))
	e.WsExec(t, `UPDATE deal SET amount_minor = 100, currency = 'ZZZ' WHERE id = $1`, deal)

	renamed := "Renamed"
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatalf("an unrelated edit on a deal stored in ZZZ: %v", err)
	}
	same := "ZZZ"
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{Currency: &same}); err != nil {
		t.Fatalf("re-sending the stored currency: %v", err)
	}

	eur, qqq := "EUR", "QQQ"
	amount := int64(100)
	if _, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{Currency: &eur, AmountMinor: &amount}); err != nil {
		t.Fatalf("moving the deal to EUR: %v", err)
	}
	_, err := e.Deals.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{Currency: &qqq, AmountMinor: &amount})
	requireUnassigned(t, "changing the currency to QQQ", err)
}

func TestADealIsNotCreatedInAnUnassignedCurrency(t *testing.T) {
	e := Setup(t)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Currency fixture', false, 93)`, pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, stage, pipeline)
	zzz, amount := "ZZZ", int64(100)
	_, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Typo", PipelineID: ids.PipelineID{UUID: pipeline}, StageID: ids.StageID{UUID: stage},
		AmountMinor: &amount, Currency: &zzz,
	})
	requireUnassigned(t, "creating a deal in ZZZ", err)
}

func TestAContractTakesOnlyAnAssignedCurrencyWhenItIsSet(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", nil)
	zzz, value := "ZZZ", int64(1000)

	_, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		CompanyID: ids.From[ids.CompanyKind](company), Title: "Typo", ValueMinor: &value,
		Currency: &zzz, ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	requireUnassigned(t, "creating a contract in ZZZ", err)

	predecessor := anActiveContract(t, e, company)
	_, err = e.Contracts.Renew(e.Admin(), predecessor, contracts.CreateContractInput{
		Title: "Renewal", ValueMinor: &value, Currency: &zzz,
		ValueBasis: contracts.BasisAnnualized, Source: "renewal",
	}, nil)
	requireUnassigned(t, "renewing into ZZZ", err)

	draft := draftInCurrency(t, e, company, "USD")
	_, err = e.Contracts.UpdateContract(e.Admin(), draft, crmcontracts.UpdateContractRequest{Currency: &zzz}, nil, nil)
	requireUnassigned(t, "changing a draft's currency to ZZZ", err)
}

func TestAContractStoredInAnUnassignedCurrencyStaysEditable(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", nil)
	draft := draftInCurrency(t, e, company, "USD")
	e.WsExec(t, `UPDATE contract SET currency = 'ZZZ' WHERE id = $1`, draft)

	title := "Retitled"
	if _, err := e.Contracts.UpdateContract(e.Admin(), draft, crmcontracts.UpdateContractRequest{Title: &title}, nil, nil); err != nil {
		t.Fatalf("an unrelated edit on a contract stored in ZZZ: %v", err)
	}
	zzz := "ZZZ"
	if _, err := e.Contracts.UpdateContract(e.Admin(), draft, crmcontracts.UpdateContractRequest{Currency: &zzz}, nil, nil); err != nil {
		t.Fatalf("re-sending the stored currency: %v", err)
	}
}
