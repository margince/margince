// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Each record mapper hands the author pair to provenance.AdmitSourceAuthor with
// its own door's importer flag and the body's source_system. The rules
// themselves are held in the kernel; what these hold is the wiring.

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheRecordImporterDoorsCarryTheAuthor(t *testing.T) {
	seat := openapi_types.UUID(ids.NewV7())
	hubspot := "mirror:hubspot"
	name := " Anna Müller "

	contactIn, err := contactCreateInputFromImporter(crmcontracts.CreateContactRequest{
		Source:   "manual",
		FullName: "Imported", SourceSystem: &hubspot, SourceAuthorId: &seat, SourceAuthorName: &name,
	})
	assertAuthor(t, "contact", contactIn.Author, err, seat)
	companyIn, err := companyCreateInputFromImporter(crmcontracts.CreateCompanyRequest{
		Source:      "manual",
		DisplayName: "Imported", SourceSystem: &hubspot, SourceAuthorId: &seat, SourceAuthorName: &name,
	})
	assertAuthor(t, "company", companyIn.Author, err, seat)
	leadIn, err := leadCreateInputFromImporter(crmcontracts.CreateLeadRequest{
		FullName: ptr("Imported"), SourceSystem: &hubspot, SourceAuthorId: &seat, SourceAuthorName: &name,
	})
	assertAuthor(t, "lead", leadIn.Author, err, seat)
}

func TestTheRecordClientDoorsRefuseAnAuthor(t *testing.T) {
	legacy := "legacy_crm"
	name := "Anna Müller"
	_, contactErr := contactCreateInput(crmcontracts.CreateContactRequest{Source: "manual", FullName: "X", SourceSystem: &legacy, SourceAuthorName: &name})
	_, companyErr := companyCreateInput(crmcontracts.CreateCompanyRequest{Source: "manual", DisplayName: "X", SourceSystem: &legacy, SourceAuthorName: &name})
	_, leadErr := leadCreateInput(crmcontracts.CreateLeadRequest{FullName: ptr("X"), SourceSystem: &legacy, SourceAuthorName: &name})
	for record, err := range map[string]error{"contact": contactErr, "company": companyErr, "lead": leadErr} {
		assertAuthorRefused(t, record, err, "reserved_source_author")
	}
}

// The importer door passes the body's source_system, not a constant: an author
// on a record naming no source is refused there too.
func TestTheRecordImporterDoorsRefuseAnAuthorFromNowhere(t *testing.T) {
	name := "Anna Müller"
	_, contactErr := contactCreateInputFromImporter(crmcontracts.CreateContactRequest{Source: "manual", FullName: "X", SourceAuthorName: &name})
	_, companyErr := companyCreateInputFromImporter(crmcontracts.CreateCompanyRequest{Source: "manual", DisplayName: "X", SourceAuthorName: &name})
	_, leadErr := leadCreateInputFromImporter(crmcontracts.CreateLeadRequest{FullName: ptr("X"), SourceAuthorName: &name})
	for record, err := range map[string]error{"contact": contactErr, "company": companyErr, "lead": leadErr} {
		assertAuthorRefused(t, record, err, "source_author_needs_a_source")
	}
}

func assertAuthor(t *testing.T, record string, got storekit.SourceAuthorInput, err error, seat openapi_types.UUID) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s importer door refused an author: %v", record, err)
	}
	if got.AuthorID == nil || *got.AuthorID != ids.UUID(seat) {
		t.Errorf("%s author id = %v, want the seat sent", record, got.AuthorID)
	}
	if got.AuthorName == nil || *got.AuthorName != "Anna Müller" {
		t.Errorf("%s author name = %v, want it trimmed and kept beside the seat", record, got.AuthorName)
	}
}

func assertAuthorRefused(t *testing.T, record string, err error, code string) {
	t.Helper()
	var refused *provenance.AuthorError
	if !errors.As(err, &refused) || refused.Code != code {
		t.Errorf("%s: err = %v, want an AuthorError coded %s", record, err, code)
	}
}
