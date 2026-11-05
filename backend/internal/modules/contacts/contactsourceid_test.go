// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheContactDoorsCarryTheOutsideIDAndRefuseOneWithNoSystem(t *testing.T) {
	system, id := "acme-erp", "P-1"
	for name, door := range map[string]func(crmcontracts.CreateContactRequest) (CreateContactInput, error){
		"client":   ContactCreationInput,
		"importer": contactCreateInputFromImporter,
	} {
		in, err := door(crmcontracts.CreateContactRequest{Source: "manual", FullName: "X", SourceSystem: &system, SourceId: &id})
		if err != nil || in.SourceID == nil || *in.SourceID != id {
			t.Errorf("%s door: SourceID = %v, err %v; want %q carried", name, in.SourceID, err, id)
		}
		_, err = door(crmcontracts.CreateContactRequest{Source: "manual", FullName: "X", SourceId: &id})
		if _, refused := err.(*provenance.SourceIDError); !refused {
			t.Errorf("%s door: an id with no system = %v, want a *provenance.SourceIDError", name, err)
		}
	}
}
