// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// controllerParticularsObject gates who may read and write the installation's
// own identity. The same object the authorization posture uses: both are facts
// about the installation rather than about any record in it, and an operator
// trusted with one is trusted with the other.
const controllerParticularsObject = "installation_settings"

// ControllerParticulars holds bounded disclosure text for the applicable jurisdiction.
// DisclosuresFor selects the fields required by each send category.
type ControllerParticulars struct {
	// LegalName is who is writing, as the law would name them. Art. 13(1)(a)
	// GDPR's "identity of the controller" and Decree 91/2020's advertiser.
	LegalName string `yaml:"LegalName"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// PostalAddress is where they are, as a block of lines. Art. 13(1)(a)
	// again, and the German business-letter particulars.
	PostalAddress string `yaml:"PostalAddress"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// PrivacyContact is how to reach whoever answers about the data —
	// Art. 13(1)(b), which asks for the data protection officer where one
	// exists and the controller's own contact where one does not.
	PrivacyContact string `yaml:"PrivacyContact"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// ObjectionRoute is how to say stop, free and without a barrier. §7(3) UWG
	// requires it on every advertising message rather than only the first, and
	// the one-click unsubscribe this product already mints is usually the
	// honest answer — but an installation whose recipients answer by phone
	// says so here.
	ObjectionRoute string `yaml:"ObjectionRoute"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// AdvertiserContact is how to reach the advertiser directly — a phone
	// number, a website, whatever the jurisdiction asks for. Vietnamese Decree
	// 91/2020 requires it on advertising beyond naming who is advertising, and
	// it is a SEPARATE field from the postal address because that requirement
	// is about reachability rather than about where somebody is registered.
	AdvertiserContact string `yaml:"AdvertiserContact"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// PrivacyPage is where the installation's own privacy information is
	// published, the link a subscription question points a reader to.
	PrivacyPage string `yaml:"PrivacyPage"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// ArchivePurpose is why captured correspondence is kept in the archive
	// after its retention window. The privacy page names it; empty prints the
	// page's default purpose.
	ArchivePurpose string `yaml:"ArchivePurpose"` //nolint:tagliatelle // the saved row spells each field by its Go name
	// Translations state the same particulars in other languages. A mail in
	// one of them prints its version where one is stated.
	Translations []ParticularsTranslation `yaml:"Translations"` //nolint:tagliatelle // the saved row spells each field by its Go name
}

// ParticularsTranslation is the particulars in one more language. An empty
// field prints the default text.
type ParticularsTranslation struct {
	// Language is a lower-case ISO 639-1 code the mail catalog carries.
	Language          string `yaml:"Language"`          //nolint:tagliatelle // the saved row spells each field by its Go name
	LegalName         string `yaml:"LegalName"`         //nolint:tagliatelle // the saved row spells each field by its Go name
	PostalAddress     string `yaml:"PostalAddress"`     //nolint:tagliatelle // the saved row spells each field by its Go name
	PrivacyContact    string `yaml:"PrivacyContact"`    //nolint:tagliatelle // the saved row spells each field by its Go name
	ObjectionRoute    string `yaml:"ObjectionRoute"`    //nolint:tagliatelle // the saved row spells each field by its Go name
	AdvertiserContact string `yaml:"AdvertiserContact"` //nolint:tagliatelle // the saved row spells each field by its Go name
	ArchivePurpose    string `yaml:"ArchivePurpose"`    //nolint:tagliatelle // the saved row spells each field by its Go name
}

// asParticulars is the translation's fields alone, with no default text under them.
func (t ParticularsTranslation) asParticulars() ControllerParticulars {
	return ControllerParticulars{
		LegalName: t.LegalName, PostalAddress: t.PostalAddress, PrivacyContact: t.PrivacyContact,
		ObjectionRoute: t.ObjectionRoute, AdvertiserContact: t.AdvertiserContact, ArchivePurpose: t.ArchivePurpose,
	}
}

// translation answers the stated version in language, if there is one.
func (p ControllerParticulars) translation(language string) (ParticularsTranslation, bool) {
	for _, t := range p.Translations {
		if t.Language == language {
			return t, true
		}
	}
	return ParticularsTranslation{}, false
}

// In answers the particulars a mail in language prints: each translated field
// that is stated, else the default text.
func (p ControllerParticulars) In(language string) ControllerParticulars {
	t, ok := p.translation(language)
	if !ok {
		return p
	}
	out := p
	for _, f := range []struct {
		into *string
		from string
	}{
		{&out.LegalName, t.LegalName},
		{&out.PostalAddress, t.PostalAddress},
		{&out.PrivacyContact, t.PrivacyContact},
		{&out.ObjectionRoute, t.ObjectionRoute},
		{&out.AdvertiserContact, t.AdvertiserContact},
		{&out.ArchivePurpose, t.ArchivePurpose},
	} {
		if strings.TrimSpace(f.from) != "" {
			*f.into = f.from
		}
	}
	return out
}

// particularsFieldCap bounds one field. Long enough for a postal address with a
// register court and number on separate lines; short enough that a footer stays
// a footer.
const particularsFieldCap = 500

// IsZero reports whether nothing has been stated.
//
// Used to tell "this installation has not configured itself" from "it stated a
// name and nothing else", which are different answers: the first is a setup
// step nobody did, and the second is a deliberate choice by somebody who read
// the form.
func (p ControllerParticulars) IsZero() bool {
	return p.LegalName == "" && p.PostalAddress == "" &&
		p.PrivacyContact == "" && p.ObjectionRoute == "" &&
		p.AdvertiserContact == "" && p.PrivacyPage == "" && p.ArchivePurpose == "" &&
		len(p.Translations) == 0
}

// ControllerIdentity is the installation's own particulars. MachineryApplied: a
// rep writing a reply holds no settings grant, and a gated read would send the
// message without its legally required disclosure. The particulars are public,
// printed on every mail; writing them stays gated on installation_settings.
// The saved row spells each field by its Go name, and so do a boot value and
// the yaml tags.
var ControllerIdentity = config.DefineStored(config.Spec[ControllerParticulars]{
	Key:      "consent.controller_particulars",
	Env:      "MARGINCE_CONSENT_CONTROLLER_PARTICULARS",
	Type:     config.Struct[ControllerParticulars](),
	Validate: validateControllerParticulars,
	Doc:      "Who this installation is, as every message it sends must say: legal name, address, privacy contact, how to object, and advertiser contact.",
}, config.Store{RBACObject: controllerParticularsObject, AuditVerb: settingsAuditVerb, MachineryApplied: true})

// validateControllerParticulars refuses what a message could not carry.
//
// It does NOT require the fields to be filled. An installation that has not
// configured itself is a real state with a real answer — the disclosure is
// recorded as missing rather than invented — and refusing the empty value here
// would mean the setting could never be saved partially, which is how somebody
// filling in a form works.
func validateControllerParticulars(p ControllerParticulars) error {
	if err := validateParticularFields(particularFields(p, "")); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, t := range p.Translations {
		if !mailcopy.Known(t.Language) || t.Language != strings.ToLower(t.Language) {
			return fmt.Errorf("translation language %q is not one this installation can send mail in", t.Language)
		}
		if seen[t.Language] {
			return fmt.Errorf("translation %q is stated twice, and a mail can print only one", t.Language)
		}
		seen[t.Language] = true
		if err := validateParticularFields(particularFields(t.asParticulars(), t.Language)); err != nil {
			return err
		}
	}
	return nil
}

// particularField is one stated text and the name a field error gives it.
type particularField struct {
	name, value string
	kind        messagingrules.DisclosureKind
}

// particularFields names each text of p. A translation's names carry its
// language, so an error points at the version that is wrong.
func particularFields(p ControllerParticulars, language string) []particularField {
	name := func(field string) string {
		if language == "" {
			return field
		}
		return "translations." + language + "." + field
	}
	return []particularField{
		{name("legal_name"), p.LegalName, messagingrules.ControllerIdentity},
		{name("postal_address"), p.PostalAddress, messagingrules.ControllerIdentity},
		{name("privacy_contact"), p.PrivacyContact, messagingrules.PrivacyContact},
		{name("objection_route"), p.ObjectionRoute, messagingrules.ObjectionRoute},
		{name("advertiser_contact"), p.AdvertiserContact, messagingrules.AdvertiserContact},
		// No disclosure kind: the page is linked, never printed as a particular.
		{name("privacy_page"), p.PrivacyPage, ""},
		// No disclosure kind: only the privacy page prints it.
		{name("archive_purpose"), p.ArchivePurpose, ""},
	}
}

func validateParticularFields(fields []particularField) error {
	for _, f := range fields {
		if len([]rune(f.value)) > particularsFieldCap {
			return fmt.Errorf(
				"%s is longer than %d characters, and it goes into every message this "+
					"installation sends", f.name, particularsFieldCap)
		}
		// A field of only whitespace reads as filled in and discloses nothing.
		// Refused rather than trimmed, because trimming would silently turn
		// somebody's mistake into an empty field they think they filled.
		if f.value != "" && strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s is only whitespace, which discloses nothing", f.name)
		}
	}
	return nil
}
