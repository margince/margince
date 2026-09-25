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

// ImporterNamespace is the prefix ALONE. If it ever widened to the whole
// reserved set, the importer door would also hand a caller the three internal
// reminder identities, and a planted row under a reminder's replay key makes
// the scan read it back as already asked.
func TestImporterNamespaceIsThePrefixAlone(t *testing.T) {
	for _, inside := range []string{"mirror:hubspot", "mirror:legacy_crm", "mirror:"} {
		if !provenance.ImporterNamespace(inside) {
			t.Errorf("ImporterNamespace(%q) = false, want true — the importer writes this", inside)
		}
	}
	for _, outside := range []string{
		"hubspot", "", "mirrorless", "MIRROR:hubspot",
		// Reserved, but never an import's to spell.
		provenance.EmailRequestSource,
		provenance.NoActivityReminderSource,
		provenance.CheckInCadenceSource,
	} {
		if provenance.ImporterNamespace(outside) {
			t.Errorf("ImporterNamespace(%q) = true, want false", outside)
		}
	}
}

// author.via is copied verbatim from the column, so without stripping, an
// imported row reads "Logged in mirror:hubspot by …" on the timeline.
func TestDisplaySourceSystemStripsOnlyThePrefix(t *testing.T) {
	for value, want := range map[string]string{
		"mirror:hubspot":              "hubspot",
		"mirror:legacy_crm":           "legacy_crm",
		"mirror:":                     "",
		"hubspot":                     "hubspot",
		"":                            "",
		provenance.EmailRequestSource: provenance.EmailRequestSource,
		// Only a leading occurrence is machinery; one inside the name is part
		// of the name.
		"legacy_mirror:crm": "legacy_mirror:crm",
	} {
		if got := provenance.DisplaySourceSystem(value); got != want {
			t.Errorf("DisplaySourceSystem(%q) = %q, want %q", value, got, want)
		}
	}
}

// Both provenance fields are guarded, and source_system is answered first:
// it is the field the namespace is keyed on, so a caller sending two reserved
// values is told about the one that matters.
func TestRefuseWireGuardsBothFieldsSourceSystemFirst(t *testing.T) {
	reservedValue := provenance.ReservedSourceSystemPrefix + "hubspot"

	var refusedSystem *provenance.ReservedError
	if err := provenance.RefuseWire("", &reservedValue); !errors.As(err, &refusedSystem) {
		t.Fatalf("err = %v, want ReservedError for source_system", err)
	} else if refusedSystem.Field != "source_system" {
		t.Errorf("field = %q, want source_system", refusedSystem.Field)
	}

	var refusedSource *provenance.ReservedError
	if err := provenance.RefuseWire(reservedValue, nil); !errors.As(err, &refusedSource) {
		t.Fatalf("err = %v, want ReservedError for source", err)
	} else if refusedSource.Field != "source" {
		t.Errorf("field = %q, want source", refusedSource.Field)
	}

	// Both reserved: source_system is named, or a caller fixes source and is
	// refused again by the field nothing told them about.
	var refusedBoth *provenance.ReservedError
	if err := provenance.RefuseWire(reservedValue, &reservedValue); !errors.As(err, &refusedBoth) {
		t.Fatalf("err = %v, want ReservedError", err)
	} else if refusedBoth.Field != "source_system" {
		t.Errorf("field = %q, want source_system answered first", refusedBoth.Field)
	}

	// Ordinary provenance — and an absent source_system — stay writable, or
	// every create wire breaks.
	ordinary := "legacy_crm"
	for _, err := range []error{
		provenance.RefuseWire("", nil),
		provenance.RefuseWire("legacy_crm", nil),
		provenance.RefuseWire("legacy_crm", &ordinary),
	} {
		if err != nil {
			t.Errorf("RefuseWire on ordinary provenance = %v, want nil", err)
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
