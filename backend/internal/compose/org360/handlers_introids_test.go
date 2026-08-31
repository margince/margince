// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The intro-request mapping's own obligation: refuse an id the caller did not
// send, rather than letting the zero UUID reach a lookup and come back as
// "that contact is not on this account" — a refusal about a record the caller
// never named and cannot connect to anything they did.

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	present := openapi_types.UUID(ids.NewV7())
	for field, body := range map[string]crmcontracts.DraftIntroRequestJSONRequestBody{
		"person_id":   {ViaUserId: present},
		"via_user_id": {PersonId: present},
	} {
		_, err := introRequestFrom(body)
		if err == nil {
			t.Fatalf("an omitted %s was accepted; the zero UUID would reach a lookup", field)
		}
		assertNamesIntroField(t, err, field)
	}
}

// A null deal_id means "the account in general" and is an ordinary case. A
// present-but-zero one is a client bug, and answering "that deal is not open"
// about the nil UUID would hide it behind a plausible-sounding refusal.
func TestAPresentButZeroDealIDIsRefusedWhileAnAbsentOneIsFine(t *testing.T) {
	t.Parallel()
	present := openapi_types.UUID(ids.NewV7())
	zero := openapi_types.UUID(ids.UUID{})

	_, err := introRequestFrom(crmcontracts.DraftIntroRequestJSONRequestBody{
		PersonId: present, ViaUserId: present, DealId: &zero,
	})
	if err == nil {
		t.Fatal("a zero deal_id was accepted")
	}
	assertNamesIntroField(t, err, "deal_id")

	req, err := introRequestFrom(crmcontracts.DraftIntroRequestJSONRequestBody{
		PersonId: present, ViaUserId: present,
	})
	if err != nil {
		t.Fatalf("an absent deal_id must be accepted as the whole account: %v", err)
	}
	if req.DealID != nil {
		t.Fatalf("an absent deal_id became %v, want none", req.DealID)
	}
}

func assertNamesIntroField(t *testing.T, err error, field string) {
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
