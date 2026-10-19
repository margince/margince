// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
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
	LegalName string
	// PostalAddress is where they are, as a block of lines. Art. 13(1)(a)
	// again, and the German business-letter particulars.
	PostalAddress string
	// PrivacyContact is how to reach whoever answers about the data —
	// Art. 13(1)(b), which asks for the data protection officer where one
	// exists and the controller's own contact where one does not.
	PrivacyContact string
	// ObjectionRoute is how to say stop, free and without a barrier. §7(3) UWG
	// requires it on every advertising message rather than only the first, and
	// the one-click unsubscribe this product already mints is usually the
	// honest answer — but an installation whose recipients answer by phone
	// says so here.
	ObjectionRoute string
	// AdvertiserContact is how to reach the advertiser directly — a phone
	// number, a website, whatever the jurisdiction asks for. Vietnamese Decree
	// 91/2020 requires it on advertising beyond naming who is advertising, and
	// it is a SEPARATE field from the postal address because that requirement
	// is about reachability rather than about where somebody is registered.
	AdvertiserContact string
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
		p.AdvertiserContact == ""
}

// ControllerIdentity is the installation's own particulars.
//
// MachineryApplied, for the same reason the authorization posture beside it is:
// the disclosures a message carries must apply whoever composes it. A rep
// writing a reply is the case that decides it — a system worker would pass the
// gate anyway, since auth.Require admits PrincipalSystem, but a rep holds
// contact grants and no settings grant at all. Gating the read would mean a
// message went out WITHOUT its legally required disclosure because the sender
// lacked an operator permission, which is the failure this whole slice exists
// to end.
//
// The disclosure through behaviour is the feature, which is the bar
// MachineryApplied sets. These particulars are the installation's own public
// identity — the name, address and contact it prints at the bottom of its mail
// — so there is nothing here a reader could learn that any recipient of that
// mail does not already have in their inbox. WRITING them stays gated on
// installation_settings at update, which is where the trust actually sits.
var ControllerIdentity = settings.Define[ControllerParticulars](
	"consent.controller_particulars",
	controllerParticularsObject,
	"update",
	ControllerParticulars{},
	validateControllerParticulars,
).MachineryApplied()

// validateControllerParticulars refuses what a message could not carry.
//
// It does NOT require the fields to be filled. An installation that has not
// configured itself is a real state with a real answer — the disclosure is
// recorded as missing rather than invented — and refusing the empty value here
// would mean the setting could never be saved partially, which is how somebody
// filling in a form works.
func validateControllerParticulars(p ControllerParticulars) error {
	for _, f := range []struct {
		name, value string
	}{
		{"legal_name", p.LegalName},
		{"postal_address", p.PostalAddress},
		{"privacy_contact", p.PrivacyContact},
		{"objection_route", p.ObjectionRoute},
		{"advertiser_contact", p.AdvertiserContact},
	} {
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
