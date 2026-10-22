// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheDealDoorsCarryTheOutsideIDAndRefuseOneWithNoSystem(t *testing.T) {
	system, id := "acme-erp", "D-1"
	request := func() crmcontracts.CreateDealRequest {
		return crmcontracts.CreateDealRequest{
			Name: "X", Source: "manual",
			PipelineId: openapi_types.UUID(ids.NewV7()), StageId: openapi_types.UUID(ids.NewV7()),
		}
	}
	for name, door := range map[string]func(crmcontracts.CreateDealRequest) (CreateDealInput, error){
		"client":   DealCreationInput,
		"importer": dealCreateInputFromImporter,
	} {
		withPair := request()
		withPair.SourceSystem, withPair.SourceId = &system, &id
		in, err := door(withPair)
		if err != nil || in.SourceID == nil || *in.SourceID != id {
			t.Errorf("%s door: SourceID = %v, err %v; want %q carried", name, in.SourceID, err, id)
		}
		idOnly := request()
		idOnly.SourceId = &id
		if _, err := door(idOnly); err == nil {
			t.Errorf("%s door: an id with no system was admitted", name)
		} else if _, refused := err.(*provenance.SourceIDError); !refused {
			t.Errorf("%s door: an id with no system = %v, want a *provenance.SourceIDError", name, err)
		}
	}
}
