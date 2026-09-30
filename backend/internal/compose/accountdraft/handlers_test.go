// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

// The wire mapping's own obligations: refuse an id the caller did not send,
// rather than letting the zero UUID reach a lookup and come back as "that
// contact is not a contact on this account" — a refusal about a record the
// caller never named and cannot connect to anything they did.

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredBodyIDIsNamedWhenAbsent(t *testing.T) {
	_, err := requestFrom(crmcontracts.DraftCompanyEmailJSONRequestBody{})
	if err == nil {
		t.Fatal("an omitted contact_id was accepted; the zero UUID would reach the contact lookup")
	}
	assertNamesField(t, err, "contact_id")
}

// A null deal_id means "the account in general" and is an ordinary case. A
// present-but-zero one is a client bug, and answering "that deal is not open"
// about the nil UUID would hide it behind a plausible-sounding refusal.
func TestAPresentButZeroDealIDIsRefusedWhileAnAbsentOneIsFine(t *testing.T) {
	contact := openapi_types.UUID(ids.NewV7())
	zero := openapi_types.UUID(ids.UUID{})

	_, err := requestFrom(crmcontracts.DraftCompanyEmailJSONRequestBody{
		ContactId: contact, DealId: &zero,
	})
	if err == nil {
		t.Fatal("a zero deal_id was accepted")
	}
	assertNamesField(t, err, "deal_id")

	req, err := requestFrom(crmcontracts.DraftCompanyEmailJSONRequestBody{ContactId: contact})
	if err != nil {
		t.Fatalf("an absent deal_id must be accepted as the whole account: %v", err)
	}
	if req.DealID != "" {
		t.Fatalf("an absent deal_id became %q, want the empty string", req.DealID)
	}
}

// The draft on screen reaches the request, and its absence is a first draft.
// Both, because a mapping that dropped the field would look exactly like a
// caller who sent none — and the whole point of the field is that a rewrite
// stops generating a different email.
func TestTheDraftOnScreenReachesTheRequest(t *testing.T) {
	contact := openapi_types.UUID(ids.NewV7())
	shown := "Guten Tag Frau Malherbe,\n\nwir liefern am Montag."

	req, err := requestFrom(crmcontracts.DraftCompanyEmailJSONRequestBody{
		ContactId: contact, RewriteOf: &shown,
	})
	if err != nil {
		t.Fatalf("a body carrying the shown draft was refused: %v", err)
	}
	if req.RewriteOf != shown {
		t.Fatalf("RewriteOf = %q, want the draft the composer is showing", req.RewriteOf)
	}

	first, err := requestFrom(crmcontracts.DraftCompanyEmailJSONRequestBody{ContactId: contact})
	if err != nil {
		t.Fatalf("a body with no rewrite_of was refused: %v", err)
	}
	if first.RewriteOf != "" {
		t.Fatalf("an absent rewrite_of became %q — a first draft has nothing to rewrite", first.RewriteOf)
	}
}

func assertNamesField(t *testing.T, err error, field string) {
	t.Helper()
	var validation *httperr.DetailedError
	if !errors.As(err, &validation) {
		t.Fatalf("refusal is %T, want a validation error naming %s", err, field)
	}
	for _, each := range validation.Fields {
		if each.Field == field {
			return
		}
	}
	t.Fatalf("refusal names %+v, want the field %s", validation.Fields, field)
}
