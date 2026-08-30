// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The transport for the VAT standing: the handler wraps the gated store read
// onto the wire shape, tells "never consulted" apart from "consulted and told
// no", and existence-hides a foreign org as 404.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestGetOrganizationVatCheckHandler(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	h := Handlers{store: e.store}
	req := httptest.NewRequest(http.MethodGet, "/organizations/x/vat-check", nil).WithContext(ctx)

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Belegpflicht GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the organization to check: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))

	// A company nobody has consulted is a 404, not an empty body. The client
	// draws "we have not asked" from the absence, and an empty 200 would let it
	// draw "we asked and learned nothing" instead.
	rec := httptest.NewRecorder()
	h.GetOrganizationVatCheck(rec, req, org.Id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unconsulted company status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}

	// Seeded through the real writer, because a hand-inserted row proves
	// nothing about what production stores.
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID:     orgID,
		Number:             "DE123456789",
		Status:             VatCheckValid,
		ConsultationNumber: "WAPIAAAAXk3-stand-in",
		RegisteredName:     "Belegpflicht GmbH",
		RegisteredAddress:  "Musterstr. 1, Berlin",
		CheckedAt:          consultedAt,
	}); err != nil {
		t.Fatalf("recording the consultation: %v", err)
	}

	rec = httptest.NewRecorder()
	h.GetOrganizationVatCheck(rec, req, org.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("consulted company status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got crmcontracts.OrganizationVatCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the VAT check response: %v", err)
	}
	if got.Status != crmcontracts.OrganizationVatCheckStatusValid {
		t.Fatalf("status = %q, want valid", got.Status)
	}
	if got.VatNumber != "DE123456789" {
		t.Fatalf("vat_number = %q, want the number as consulted", got.VatNumber)
	}
	// The receipt is the half a tax authority accepts, so it has to survive the
	// wire — a verdict that arrives without it proves nothing.
	if got.ConsultationNumber == nil || *got.ConsultationNumber != "WAPIAAAAXk3-stand-in" {
		t.Fatalf("consultation_number = %v, want the register's receipt", got.ConsultationNumber)
	}
	if got.RegisteredName == nil || *got.RegisteredName != "Belegpflicht GmbH" {
		t.Fatalf("registered_name = %v, want who the register says the number belongs to", got.RegisteredName)
	}
	if !got.CheckedAt.Equal(consultedAt) {
		t.Fatalf("checked_at = %v, want the day the register reported (%v)", got.CheckedAt, consultedAt)
	}

	// A foreign or absent org is existence-hidden as 404, same as every other
	// row-scoped read.
	rec = httptest.NewRecorder()
	h.GetOrganizationVatCheck(rec, req, crmcontracts.Id(ids.NewV7()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign org status = %d, want 404", rec.Code)
	}
}

func TestAVatCheckWithNoReceiptOmitsItRatherThanSendingEmpty(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	h := Handlers{store: e.store}
	req := httptest.NewRequest(http.MethodGet, "/organizations/x/vat-check", nil).WithContext(ctx)

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Ohne Beleg GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the organization to check: %v", err)
	}

	// An installation with no VAT ID of its own still gets an answer; the
	// register issues it no receipt. Absent and empty-string are different
	// things on the wire: "" reads to a client as a receipt it should show.
	if err := e.store.RecordVatCheck(ctx, VatCheck{
		OrganizationID: ids.From[ids.OrganizationKind](ids.UUID(org.Id)),
		Number:         "DE987654321",
		Status:         VatCheckInvalid,
		CheckedAt:      consultedAt,
	}); err != nil {
		t.Fatalf("recording a consultation that issued no receipt: %v", err)
	}

	rec := httptest.NewRecorder()
	h.GetOrganizationVatCheck(rec, req, org.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode the VAT check response: %v", err)
	}
	for _, absent := range []string{"consultation_number", "registered_name", "registered_address"} {
		if _, present := raw[absent]; present {
			t.Fatalf("%s is on the wire as %v, want the key omitted when the register gave none", absent, raw[absent])
		}
	}
	if raw["status"] != string(VatCheckInvalid) {
		t.Fatalf("status = %v, want invalid — the verdict travels without a receipt", raw["status"])
	}
}
