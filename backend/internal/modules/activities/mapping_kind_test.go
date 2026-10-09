// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// activity.kind is a foreign key to the activity_kind catalog, so an unknown
// kind reached the database and came back as a foreign-key violation.
//
// The transport's generic net reads one as a missing record.
//
// So a caller who mistyped the kind was told to send an id of the right kind.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAnUnknownActivityKindIsRefusedAsAPicklistValue(t *testing.T) {
	t.Parallel()
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "bogus"})

	invalid, ok := errors.AsType[*InvalidKindError](err)
	if !ok {
		t.Fatalf("err = %v, want InvalidKindError — an unknown kind is a picklist mistake", err)
	}
	field, code, _ := invalid.FieldFault()
	if field != "kind" {
		t.Errorf("field = %q, want kind — the caller can only correct a field they are told about", field)
	}
	if code != codeInvalidEnum {
		t.Errorf("code = %q, want %q, not the missing-reference code", code, codeInvalidEnum)
	}
}

// An absent kind is a missing field, and stays one: the picklist guard sits
// beside that check and must not swallow it.
func TestAnAbsentActivityKindIsStillAMissingField(t *testing.T) {
	t.Parallel()
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{})

	required, ok := errors.AsType[*RequiredFieldError](err)
	if !ok {
		t.Fatalf("err = %v, want RequiredFieldError — no kind is a missing field", err)
	}
	if field, _, _ := required.FieldFault(); field != fieldKind {
		t.Errorf("field = %q, want %q", field, fieldKind)
	}
}

// Every door onto the create mapping shares the guard, because each reaches
// the same column.
func TestEveryCreateDoorRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()
	doors := map[string]func(crmcontracts.CreateActivityRequest) (LogActivityInput, error){
		"client":          LogActivityInputFrom,
		"importer":        LogActivityInputFromImporter,
		"engine reminder": logActivityInputAllowingReminderIdentity,
	}
	for name, door := range doors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := door(crmcontracts.CreateActivityRequest{Kind: "bogus"}); !errorIsInvalidKind(err) {
				t.Errorf("%s door: err = %v, want InvalidKindError", name, err)
			}
		})
	}
}

// A kind the contract declares stays writable, so the guard refuses the
// unknown value rather than the field.
func TestADeclaredActivityKindStaysWritable(t *testing.T) {
	t.Parallel()
	for _, kind := range []crmcontracts.CreateActivityRequestKind{"call", "email", "meeting", "message", "note", "task"} {
		if _, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: kind}); errorIsInvalidKind(err) {
			t.Errorf("kind %q was refused, and the contract declares it", kind)
		}
	}
}

func errorIsInvalidKind(err error) bool {
	_, ok := errors.AsType[*InvalidKindError](err)
	return ok
}
