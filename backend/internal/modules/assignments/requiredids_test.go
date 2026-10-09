// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

// The create mapping's own obligation: refuse an id the caller did not send,
// rather than letting the zero UUID reach the role or subject lookup and come
// back as "no such role" — a refusal about a role the caller never named.

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()
	present := openapi_types.UUID(ids.NewV7())

	if err := checkAssignmentIDs(crmcontracts.CreateRecordAssignmentRequest{
		SubjectId: present,
	}); err == nil {
		t.Fatal("an omitted role_id was accepted; the zero UUID would reach the role lookup")
	} else {
		assertNamesField(t, err, "role_id")
	}

	if err := checkAssignmentIDs(crmcontracts.CreateRecordAssignmentRequest{
		RoleId: present,
	}); err == nil {
		t.Fatal("an omitted subject_id was accepted; the zero UUID would reach the subject lookup")
	} else {
		assertNamesField(t, err, "subject_id")
	}

	// The admit case, without which the two refusals above would pass against a
	// guard that refused every request.
	if err := checkAssignmentIDs(crmcontracts.CreateRecordAssignmentRequest{
		RoleId: present, SubjectId: present,
	}); err != nil {
		t.Fatalf("a complete request was refused: %v", err)
	}
}

// assertNamesField fails unless the error names the field the caller has to
// fix. An error that refuses without saying which id was missing leaves the
// caller guessing between two.
func assertNamesField(t *testing.T, err error, field string) {
	t.Helper()
	if got := err.Error(); !strings.Contains(got, field) {
		t.Fatalf("error %q does not name %q", got, field)
	}
}

// The kind the caller sent is checked before the role is read, so a kind the
// contract does not declare names its own field.
//
// The role's check answers whether a declared kind may hold that role.
//
// An absent or misspelled kind fell through to it and blamed `role_id`.
//
// A caller who forgot the kind was told the role cannot be held by a blank.
func TestAnUndeclaredSubjectKindNamesItsOwnField(t *testing.T) {
	t.Parallel()
	for name, kind := range map[string]crmcontracts.AssignmentSubjectKind{
		"absent":     "",
		"unknown":    "group",
		"wrong case": "USER",
		"a record":   "company",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := admitSubjectKind(kind)
			if err == nil {
				t.Fatalf("kind %q was accepted, and the contract declares user and team", kind)
			}
			assertNamesField(t, err, "subject_kind")
			if strings.Contains(err.Error(), "role_id") {
				t.Errorf("error %q blames the role for a kind fault", err)
			}
		})
	}
}

// Both declared kinds pass, so the guard refuses the undeclared value rather
// than the field.
func TestBothDeclaredSubjectKindsAreAdmitted(t *testing.T) {
	t.Parallel()
	for _, kind := range []crmcontracts.AssignmentSubjectKind{
		crmcontracts.AssignmentSubjectKindUser, crmcontracts.AssignmentSubjectKindTeam,
	} {
		if err := admitSubjectKind(kind); err != nil {
			t.Errorf("kind %q refused with %v, and the contract declares it", kind, err)
		}
	}
}
