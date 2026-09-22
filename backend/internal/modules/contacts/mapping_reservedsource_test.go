// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The importer's provenance namespace is a security boundary, and these
// mappers are where it is enforced. Two distinct things depend on it:
// the lead store keys its idempotent replay on (source_system,
// source_id), so a client able to spell the reserved prefix could
// pre-plant a row under an incumbent record id and have a later import
// hand it back as already existing; and the flip's crash repair reads
// `source` back to recognize records it created but had not yet mapped,
// which is only safe while nothing else can write that prefix.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestLeadCreateInputRefusesTheImporterNamespace(t *testing.T) {
	reserved := "mirror:legacy_crm"
	_, err := leadCreateInput(crmcontracts.CreateLeadRequest{
		SourceSystem: &reserved, SourceId: ptr("501"),
	})
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want provenance.ReservedError — a client must not write the importer's namespace", err)
	}
	if refused.Value != reserved {
		t.Errorf("refusal names %q, want the offending value", refused.Value)
	}
}

func TestLeadCreateInputAcceptsAnOrdinarySourceSystem(t *testing.T) {
	ordinary := "legacy_crm"
	in, err := leadCreateInput(crmcontracts.CreateLeadRequest{
		SourceSystem: &ordinary, SourceId: ptr("501"),
	})
	if err != nil {
		t.Fatalf("an ordinary source system must stay writable: %v", err)
	}
	if in.SourceSystem == nil || *in.SourceSystem != ordinary {
		t.Errorf("SourceSystem = %v, want it carried through", in.SourceSystem)
	}
}

// EVERY create wire that accepts provenance refuses the namespace. The
// flip stamps `source` inside it on contacts, companies and deals —
// classes with no (source_system, source_id) replay key — so a gap in
// any one of these lets a planted row be adopted as the importer's own.
func TestEveryProvenanceWireRefusesTheImporterNamespace(t *testing.T) {
	const reserved = "mirror:legacy_crm:contact:p-1"
	var refused *provenance.ReservedError

	if _, err := contactCreateInput(crmcontracts.CreateContactRequest{
		FullName: "Planted", Source: reserved,
	}); !errors.As(err, &refused) {
		t.Errorf("contact: err = %v, want the namespace refused", err)
	}
	if _, err := companyCreateInput(crmcontracts.CreateCompanyRequest{
		DisplayName: "Planted", Source: reserved,
	}); !errors.As(err, &refused) {
		t.Errorf("company: err = %v, want the namespace refused", err)
	}
	if _, err := leadCreateInput(crmcontracts.CreateLeadRequest{
		FullName: ptr("Planted"), Source: reserved,
	}); !errors.As(err, &refused) {
		t.Errorf("lead source: err = %v, want the namespace refused", err)
	}

	// The refusal names the field it arrived on, because the two are
	// different wire fields and the caller has to know which to change.
	if refused != nil && refused.Field != "source" {
		t.Errorf("refusal names field %q, want source", refused.Field)
	}

	// An ordinary provenance string stays writable — the guard is a
	// prefix rule, not a ban on the field.
	if _, err := contactCreateInput(crmcontracts.CreateContactRequest{
		FullName: "Real", Source: "legacy_crm:contact:p-1",
	}); err != nil {
		t.Errorf("an ordinary source must stay writable: %v", err)
	}
}

// contact and company gained a source_system wire of their own, and it is
// guarded like every other provenance field. Separate from the test above on
// purpose: that one reuses ONE `refused` var across its probes and asserts the
// field is "source", so a source_system probe added there would flip that
// assertion and pass for the wrong reason.
func TestTheRecordWiresRefuseTheImporterNamespaceOnSourceSystem(t *testing.T) {
	reserved := "mirror:legacy_crm"

	var contactRefused *provenance.ReservedError
	if _, err := contactCreateInput(crmcontracts.CreateContactRequest{
		FullName: "Planted", Source: "legacy_crm", SourceSystem: &reserved,
	}); !errors.As(err, &contactRefused) {
		t.Fatalf("contact: err = %v, want the namespace refused", err)
	} else if contactRefused.Field != "source_system" {
		t.Errorf("contact: refusal names %q, want source_system — the field the caller has to change", contactRefused.Field)
	}

	var companyRefused *provenance.ReservedError
	if _, err := companyCreateInput(crmcontracts.CreateCompanyRequest{
		DisplayName: "Planted", Source: "legacy_crm", SourceSystem: &reserved,
	}); !errors.As(err, &companyRefused) {
		t.Fatalf("company: err = %v, want the namespace refused", err)
	} else if companyRefused.Field != "source_system" {
		t.Errorf("company: refusal names %q, want source_system", companyRefused.Field)
	}

	// The positive control. Without it this test would still pass if the
	// mapper refused every source_system, which would break every ordinary
	// import rather than only the forged one.
	ordinary := "legacy_crm"
	contactIn, err := contactCreateInput(crmcontracts.CreateContactRequest{
		FullName: "Real", Source: "legacy_crm", SourceSystem: &ordinary,
	})
	if err != nil {
		t.Fatalf("contact: an ordinary source system must stay writable: %v", err)
	}
	if contactIn.SourceSystem == nil || *contactIn.SourceSystem != ordinary {
		t.Errorf("contact: SourceSystem = %v, want it carried to the store", contactIn.SourceSystem)
	}
	companyIn, err := companyCreateInput(crmcontracts.CreateCompanyRequest{
		DisplayName: "Real", Source: "legacy_crm", SourceSystem: &ordinary,
	})
	if err != nil {
		t.Fatalf("company: an ordinary source system must stay writable: %v", err)
	}
	if companyIn.SourceSystem == nil || *companyIn.SourceSystem != ordinary {
		t.Errorf("company: SourceSystem = %v, want it carried to the store", companyIn.SourceSystem)
	}
}

func ptr(s string) *string { return &s }
