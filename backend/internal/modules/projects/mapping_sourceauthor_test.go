// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The project mapper hands the author pair to provenance.AdmitSourceAuthor with
// its door's importer flag and the body's source_system; the rules are the
// kernel's, the wiring is this.
func TestTheProjectDoorsAdmitAnAuthorOnlyFromTheImporter(t *testing.T) {
	name := "Anna Müller"
	hubspot := "mirror:hubspot"
	body := func(sourceSystem *string) crmcontracts.CreateProjectRequest {
		return crmcontracts.CreateProjectRequest{
			Name: "Imported", Source: "manual", SourceSystem: sourceSystem, SourceAuthorName: &name,
			CompanyId: openapi_types.UUID(ids.NewV7()),
		}
	}

	in, err := projectCreateInputFromImporter(body(&hubspot))
	if err != nil || in.Author.AuthorName == nil || *in.Author.AuthorName != name {
		t.Errorf("importer door = %v / %v, want the name carried", in.Author.AuthorName, err)
	}
	legacy := "legacy_crm"
	for door, err := range map[string]error{
		"client door":           errOf(projectCreateInput(body(&legacy))),
		"importer from nowhere": errOf(projectCreateInputFromImporter(body(nil))),
	} {
		if _, ok := errors.AsType[*provenance.AuthorError](err); !ok {
			t.Errorf("%s admitted an author: %v", door, err)
		}
	}
}

func errOf(_ CreateProjectInput, err error) error { return err }
