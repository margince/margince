// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The project create wire refused NOTHING until the four record wires gained
// source_system: contact, company, deal and lead have guarded the importer's
// namespace since it existed, and this one was the gap. A caller who could
// spell the prefix here would have the retrieval trust ladder read a row they
// typed as captured external history.

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestProjectCreateInputRefusesTheImporterNamespace(t *testing.T) {
	company := openapi_types.UUID(ids.NewV7())
	reserved := "mirror:legacy_crm"

	var onSystem *provenance.ReservedError
	if _, err := projectCreateInput(crmcontracts.CreateProjectRequest{
		Name: "Planted", CompanyId: company, Source: "ui", SourceSystem: &reserved,
	}); !errors.As(err, &onSystem) {
		t.Fatalf("source_system: err = %v, want the namespace refused", err)
	} else if onSystem.Field != "source_system" {
		t.Errorf("refusal names %q, want source_system", onSystem.Field)
	}

	var onSource *provenance.ReservedError
	if _, err := projectCreateInput(crmcontracts.CreateProjectRequest{
		Name: "Planted", CompanyId: company, Source: reserved,
	}); !errors.As(err, &onSource) {
		t.Fatalf("source: err = %v, want the namespace refused", err)
	} else if onSource.Field != "source" {
		t.Errorf("refusal names %q, want source", onSource.Field)
	}
}

// The positive control. A guard that refused every source_system would refuse
// every real import, and this test is the only thing that tells the two apart.
func TestProjectCreateInputCarriesAnOrdinarySourceSystem(t *testing.T) {
	ordinary := "legacy_crm"
	in, err := projectCreateInput(crmcontracts.CreateProjectRequest{
		Name:         "Real",
		CompanyId:    openapi_types.UUID(ids.NewV7()),
		Source:       "ui",
		SourceSystem: &ordinary,
	})
	if err != nil {
		t.Fatalf("an ordinary source system must stay writable: %v", err)
	}
	if in.SourceSystem == nil || *in.SourceSystem != ordinary {
		t.Errorf("SourceSystem = %v, want it carried to the store", in.SourceSystem)
	}
}
