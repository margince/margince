// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// An existing-customer claim settles on the sibling the record bears out. The
// rep claims a service message; the named invoice to its billing contact makes
// it an invoice notice under the same contract ground.
//
// Mutation: answer the claim's own refusal in validateFamily and this fails.
func TestAnExistingCustomerClaimSettlesOnTheSiblingTheRecordBearsOut(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryCustomerService,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if !got.Supported || got.Category != commsauthz.CategoryInvoiceOrPayment {
		t.Fatalf("resolved %q supported=%v (%s), want a supported invoice_or_payment", got.Category, got.Supported, got.Reason)
	}
	if got.Basis != commsauthz.BasisContract {
		t.Errorf("basis = %q, want contract", got.Basis)
	}
}

// A sibling never reaches outside its family: a marketing claim with an
// invoice behind it stays a marketing claim.
func TestAMarketingClaimHasNoSiblings(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryMarketing,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if got.Category != commsauthz.CategoryMarketing {
		t.Errorf("resolved %q, want the marketing claim kept", got.Category)
	}
}
