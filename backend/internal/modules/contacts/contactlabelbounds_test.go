// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestTypedContactNamesUseTheLabelBoundWithoutCuttingImportedNames(t *testing.T) {
	name := strings.Repeat("界", values.TextLabelCharacters)
	if _, err := ContactCreationInput(crmcontracts.CreateContactRequest{FullName: name, Source: "manual"}); err != nil {
		t.Fatalf("name at the character bound: %v", err)
	}
	name += "界"
	if _, err := ContactCreationInput(crmcontracts.CreateContactRequest{FullName: name, Source: "manual"}); err == nil {
		t.Fatal("typed name past the label bound was accepted")
	}
	imported, err := contactCreateInputFromImporter(crmcontracts.CreateContactRequest{FullName: name, Source: "manual"})
	if err != nil || imported.FullName != name {
		t.Fatalf("imported identity = %q, %v", imported.FullName, err)
	}
	if !imported.sourceNames {
		t.Fatal("the trusted importer lost its source-name admission")
	}
}

func TestUnchangedLongContactNamesDoNotBlockOtherEdits(t *testing.T) {
	name := strings.Repeat("界", values.TextLabelCharacters+1)
	current := crmcontracts.Contact{FullName: name, FirstName: &name}
	for _, patch := range []UpdateContactInput{{}, {FullName: &name, FirstName: &name}} {
		if err := checkChangedContactNames(current, patch); err != nil {
			t.Fatalf("unchanged source name: %v", err)
		}
	}
	changed := name + "界"
	if err := checkChangedContactNames(current, UpdateContactInput{FullName: &changed}); err == nil {
		t.Fatal("a changed oversized name was admitted")
	}
}
