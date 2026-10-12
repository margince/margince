// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The deal mapper hands the author pair to provenance.AdmitSourceAuthor with
// its door's importer flag and the body's source_system; the rules are the
// kernel's, the wiring is this.
func TestTheDealDoorsAdmitAnAuthorOnlyFromTheImporter(t *testing.T) {
	seat := openapi_types.UUID(ids.NewV7())
	hubspot := "mirror:hubspot"
	body := func(sourceSystem *string) crmcontracts.CreateDealRequest {
		return crmcontracts.CreateDealRequest{
			Source: "manual",
			Name:   "Imported", SourceSystem: sourceSystem, SourceAuthorId: &seat,
			PipelineId: openapi_types.UUID(ids.NewV7()), StageId: openapi_types.UUID(ids.NewV7()),
		}
	}

	in, err := dealCreateInputFromImporter(body(&hubspot))
	if err != nil || in.Author.AuthorID == nil || *in.Author.AuthorID != ids.UUID(seat) {
		t.Errorf("importer door = %v / %v, want the seat carried", in.Author.AuthorID, err)
	}
	legacy := "legacy_crm"
	for door, err := range map[string]error{
		"client door":           second(dealCreateInput(body(&legacy))),
		"importer from nowhere": second(dealCreateInputFromImporter(body(nil))),
	} {
		if _, ok := errors.AsType[*provenance.AuthorError](err); !ok {
			t.Errorf("%s admitted an author: %v", door, err)
		}
	}
}

func second(_ CreateDealInput, err error) error { return err }
