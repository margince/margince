// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The importer's namespace is a security boundary, not a convention: a
// caller who can write it can pre-plant a row under an incumbent record
// id and have a later import treat the real record as already landed.
func TestReservedSourceSystem(t *testing.T) {
	for _, reserved := range []string{
		"mirror:legacy_crm", "mirror:salesforce", "mirror:",
		provenance.EmailRequestSource,
		// The quiet-account reminders key their idempotent replay on this
		// name, and the replay tests nothing else about the row it finds —
		// so a client able to spell one could plant a row under a
		// reminder's key and have the scan read it back as already asked.
		provenance.NoActivityReminderSource,
		provenance.CheckInCadenceSource,
	} {
		if !provenance.ReservedSourceSystem(reserved) {
			t.Errorf("%q must be refused from a client write", reserved)
		}
	}
	// renewal_reminder carries no natural key, so it is deliberately NOT
	// reserved: reserving a name nothing stamps would refuse a client write
	// to buy nothing.
	for _, allowed := range []string{"legacy_crm", "gmail", "", "notmirror:legacy_crm", "MIRROR:legacy_crm", "renewal_reminder"} {
		if provenance.ReservedSourceSystem(allowed) {
			t.Errorf("%q is an ordinary source system and must stay writable", allowed)
		}
	}
}

func TestRefuseNamesTheFieldAndLetsOrdinaryValuesThrough(t *testing.T) {
	err := provenance.Refuse("source", provenance.ReservedSourceSystemPrefix+"legacy_crm:contact:p-1")
	var reserved *provenance.ReservedError
	if !errors.As(err, &reserved) {
		t.Fatalf("err = %v, want ReservedError — a client write into the import namespace must be refused", err)
	}
	if reserved.Field != "source" {
		t.Errorf("refusal names field %q, want the one it arrived on; the caller has to know which to change", reserved.Field)
	}
	if !strings.Contains(reserved.Error(), "source") || !strings.Contains(reserved.Error(), "reserved") {
		t.Errorf("message %q says neither the field nor why", reserved.Error())
	}
	// The guard is a prefix rule, not a ban: ordinary provenance — and an
	// empty one — stay writable, or every create wire would break.
	for _, ordinary := range []string{"", "legacy_crm", "legacy_crm:contact:p-1", "mirrorless"} {
		if err := provenance.Refuse("source", ordinary); err != nil {
			t.Errorf("Refuse(%q) = %v, want nil", ordinary, err)
		}
	}
}

// The refusal has to name every identity it refuses, or a caller reads a
// message listing two names, picks the third, and is refused again with the
// same sentence.
//
// The corpus is DERIVED, not a list repeated here: a hard-coded three would go
// on passing when a fourth identity is added and the message does not follow
// it, which is the one failure this test exists to catch. ReservedSourceSystem
// is the only authority on membership, so the candidates are probed against it
// rather than against a copy of the set.
func TestTheRefusalNamesEveryReservedIdentity(t *testing.T) {
	message := (&provenance.ReservedError{
		Field: "source_system", Value: "ordinary_value",
	}).Error()
	// The corpus comes from the reserved set itself, so a FOURTH identity added
	// without the refusal following it fails here. A hand-written list of three
	// would go on passing, which is the whole defect this guards.
	reserved := provenance.InternalSourceSystems()
	if len(reserved) < 3 {
		t.Fatalf("the reserved set lists %d identities, want at least the three that exist", len(reserved))
	}
	for _, candidate := range reserved {
		if !strings.Contains(message, candidate) {
			t.Errorf("refusal %q does not name the reserved identity %q", message, candidate)
		}
	}
}

func TestReservedErrorStatesItselfAsCallerFixable(t *testing.T) {
	// Implementing apperrors.FieldFault is what carries this refusal to
	// the caller as a 422 naming the field — on the HTTP surface AND on
	// the MCP tool surface, neither of which knows this type. Without it
	// the refusal degrades to an opaque internal fault telling the caller
	// to retry something that will never succeed.
	var fault apperrors.FieldFault = &provenance.ReservedError{
		Field: "source", Value: "mirror:legacy_crm:contact:p-1",
	}
	field, code, message := fault.FieldFault()
	if field != "source" {
		t.Errorf("field = %q, want the one the value arrived on", field)
	}
	if code != "reserved_source_system" {
		t.Errorf("code = %q, want the contract's machine code", code)
	}
	if !strings.Contains(message, "reserved") || !strings.Contains(message, provenance.ReservedSourceSystemPrefix) {
		t.Errorf("message %q must say what is wrong and which namespace to avoid", message)
	}
}
