// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// The activity mapper reads the importer half of its admission only. The
// engine's reminder door may stamp its own source identity, and that must not
// make it an author's door too.
func TestOnlyTheImporterAdmissionAdmitsAnAuthor(t *testing.T) {
	name := "Anna Müller"
	body := func(sourceSystem string) crmcontracts.CreateActivityRequest {
		return crmcontracts.CreateActivityRequest{Kind: "note", SourceSystem: &sourceSystem, SourceAuthorName: &name}
	}

	in, err := LogActivityInputFromImporter(body("mirror:hubspot"))
	if err != nil || in.Author.AuthorName == nil || *in.Author.AuthorName != name {
		t.Errorf("importer door = %v / %v, want the name carried", in.Author.AuthorName, err)
	}
	for door, err := range map[string]error{
		"client door":   errOf(LogActivityInputFrom(body("legacy_crm"))),
		"reminder door": errOf(logActivityInputAllowingReminderIdentity(body(provenance.NoActivityReminderSource))),
	} {
		var refused *provenance.AuthorError
		if !errors.As(err, &refused) || refused.Code != "reserved_source_author" {
			t.Errorf("%s: err = %v, want the author refused as reserved", door, err)
		}
	}
}

func errOf(_ LogActivityInput, err error) error { return err }
