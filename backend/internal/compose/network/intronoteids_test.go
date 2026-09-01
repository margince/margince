// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// The note mapping's own obligation: refuse an id the caller did not send,
// rather than letting the zero UUID reach the route lookup and come back as
// "that colleague has no recorded route to this contact" — a refusal about a
// colleague the caller never named and cannot connect to anything they did.

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
	if err := checkNoteIDs(crmcontracts.DraftIntroNoteJSONRequestBody{}); err == nil {
		t.Fatal("an omitted via_user_id was accepted; the zero UUID would reach the route lookup")
	} else {
		assertNamesNoteField(t, err, "via_user_id")
	}
}

// A null through_person_id means a direct route and is the ordinary case. A
// present-but-zero one is a client bug, and answering "that colleague has no
// route to this contact" about the nil UUID would hide it behind a
// plausible-sounding refusal.
func TestAPresentButZeroIntermediaryIsRefusedWhileAnAbsentOneIsFine(t *testing.T) {
	t.Parallel()
	present := openapi_types.UUID(ids.NewV7())
	zero := openapi_types.UUID(ids.UUID{})

	err := checkNoteIDs(crmcontracts.DraftIntroNoteJSONRequestBody{
		ViaUserId: present, ThroughPersonId: &zero,
	})
	if err == nil {
		t.Fatal("a zero through_person_id was accepted")
	}
	assertNamesNoteField(t, err, "through_person_id")

	// The admit case, without which the refusals above would pass against a
	// guard that refused every request.
	if err := checkNoteIDs(crmcontracts.DraftIntroNoteJSONRequestBody{
		ViaUserId: present,
	}); err != nil {
		t.Fatalf("an absent through_person_id must be accepted as a direct route: %v", err)
	}
}

func assertNamesNoteField(t *testing.T, err error, field string) {
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
