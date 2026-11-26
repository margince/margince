// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// The page names the controller by the rule a mail's disclosure line uses. So
// a name with no address is shown as an identity on neither. The name alone is
// still the one "This message is from" names.
func TestThePageAndTheMailAgreeOnWhatNamesTheController(t *testing.T) {
	t.Parallel()
	alone := wireController(ControllerParticulars{LegalName: "Beispiel GmbH"})
	if alone == nil || alone.Identity != nil || alone.LegalName == nil || *alone.LegalName != "Beispiel GmbH" {
		t.Errorf("a name with no address is shown as %+v, want the name alone and no identity, "+
			"which the mail would report missing", alone)
	}
	stated := ControllerParticulars{LegalName: "Beispiel GmbH", PostalAddress: "Hauptstr. 1", PrivacyContact: "privacy@beispiel.test"}
	got := wireController(stated)
	if got == nil || got.Identity == nil || *got.Identity != particularFor(messagingrules.ControllerIdentity, stated) {
		t.Fatalf("the page names %+v, want the mail's identity line", got)
	}
	if got.PrivacyContact == nil || *got.PrivacyContact != "privacy@beispiel.test" || got.ObjectionRoute != nil {
		t.Errorf("the page names contact %v and route %v, want only what was stated", got.PrivacyContact, got.ObjectionRoute)
	}
}

// The privacy page names the archive's purpose in the language of its link.
// It takes the stated translation, else the installation's own words. With
// neither, it names none and the page prints its default purpose.
func TestThePrivacyPageNamesTheArchivePurposeInTheLinksLanguage(t *testing.T) {
	t.Parallel()
	stated := ControllerParticulars{
		ArchivePurpose: "answer claims under our supply contracts",
		Translations: []ParticularsTranslation{
			{Language: "vi", ArchivePurpose: "giải quyết khiếu nại theo hợp đồng cung cấp"},
		},
	}
	for language, want := range map[string]string{
		"vi": "giải quyết khiếu nại theo hợp đồng cung cấp",
		"de": "answer claims under our supply contracts",
		"en": "answer claims under our supply contracts",
	} {
		got := wireController(stated.In(language))
		if got == nil || got.ArchivePurpose == nil || *got.ArchivePurpose != want {
			t.Errorf("a %s page names the archive purpose as %+v, want %q", language, got, want)
		}
	}
	if got := wireController(ControllerParticulars{LegalName: "Beispiel GmbH"}); got == nil || got.ArchivePurpose != nil {
		t.Errorf("with no purpose stated the page names %+v, want none so the default prints", got)
	}
}

// The purpose is bounded and refused when it is only whitespace, as every
// stated particular is, in each language.
func TestAnArchivePurposeIsValidatedLikeEveryParticular(t *testing.T) {
	t.Parallel()
	for name, p := range map[string]ControllerParticulars{
		"blank":            {ArchivePurpose: "   "},
		"too long":         {ArchivePurpose: strings.Repeat("a", particularsFieldCap+1)},
		"blank translated": {Translations: []ParticularsTranslation{{Language: "vi", ArchivePurpose: " "}}},
	} {
		err := validateControllerParticulars(p)
		if err == nil || !strings.Contains(err.Error(), "archive_purpose") {
			t.Errorf("%s purpose answered %v, want a refusal naming archive_purpose", name, err)
		}
	}
	if err := validateControllerParticulars(ControllerParticulars{ArchivePurpose: "keep business records"}); err != nil {
		t.Errorf("a stated purpose was refused: %v", err)
	}
}
