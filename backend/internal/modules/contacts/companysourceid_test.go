// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheCompanyDoorsCarryTheOutsideIDAndRefuseOneWithNoSystem(t *testing.T) {
	system, id := "acme-erp", "C-1"
	for name, door := range map[string]func(crmcontracts.CreateCompanyRequest) (CreateCompanyInput, error){
		"client":   CompanyCreationInput,
		"importer": companyCreateInputFromImporter,
	} {
		in, err := door(crmcontracts.CreateCompanyRequest{Source: "manual", DisplayName: "X", SourceSystem: &system, SourceId: &id})
		if err != nil || in.SourceID == nil || *in.SourceID != id {
			t.Errorf("%s door: SourceID = %v, err %v; want %q carried", name, in.SourceID, err, id)
		}
		_, err = door(crmcontracts.CreateCompanyRequest{Source: "manual", DisplayName: "X", SourceId: &id})
		if _, refused := err.(*provenance.SourceIDError); !refused {
			t.Errorf("%s door: an id with no system = %v, want a *provenance.SourceIDError", name, err)
		}
	}
}
