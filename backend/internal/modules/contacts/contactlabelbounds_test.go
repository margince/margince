// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The label bound holds for every caller of the contact endpoint. A role that
// may import does not make a request an import. Only the import and capture
// factories set sourceNames in code and keep a source's full name.
func TestTypedContactNamesUseTheLabelBoundForEveryCaller(t *testing.T) {
	name := strings.Repeat("界", values.TextLabelCharacters)
	if _, err := ContactCreationInput(crmcontracts.CreateContactRequest{FullName: name, Source: "manual"}); err != nil {
		t.Fatalf("name at the character bound: %v", err)
	}
	name += "界"
	for caller, mapInput := range map[string]func(crmcontracts.CreateContactRequest) (CreateContactInput, error){
		"a caller":            ContactCreationInput,
		"a declared importer": contactCreateInputFromImporter,
	} {
		in, err := mapInput(crmcontracts.CreateContactRequest{FullName: name, Source: "manual"})
		if err == nil || in.sourceNames {
			t.Errorf("%s: a typed name past the label bound = %+v, %v; want a refusal", caller, in, err)
		}
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
