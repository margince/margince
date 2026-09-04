// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The boot checks over a unit's PASSIVE POLICY: its jurisdiction packs, its
// retention floors and its outbound-messaging rules.
//
// Their own file because they are one concept and extensions.go had reached
// the size cap. What joins them is what they protect against: none of these
// declarations is a governed operation an operator resolves, so none appears in
// the manifest — and a malformed one would therefore compose silently, look
// registered, and be read by an engine as the country's law. These checks are
// the only place that reads the VALUES before the registries take them.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/pkg/extension"
)

// preflightJurisdictions checks one unit's declared packs for grammar,
// duplicates within the composed set, collisions with core packs, and
// retention classes outside the closed vocabularies — an unknown class
// (or anchor, or a negative period) would be a statutory floor that
// looks registered while the engine misreads or ignores it.
func preflightJurisdictions(e extension.Extension, packCodes map[jurisdiction.Code]extension.Name) error {
	for _, p := range e.Jurisdictions {
		code := p.Code()
		if err := code.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if owner, dup := packCodes[code]; dup {
			return fmt.Errorf("compose: extensions %q and %q both declare jurisdiction %q", owner, e.Name, code)
		}
		if _, taken := jurisdiction.For(code); taken {
			return fmt.Errorf("compose: extension %q declares jurisdiction %q, which a core pack already registers", e.Name, code)
		}
		if err := preflightRetentionClasses(e.Name, code, p.Retention()); err != nil {
			return err
		}
		packCodes[code] = e.Name
	}
	return nil
}

// preflightMessagingRules checks one unit's declared messaging rules for
// grammar and for a second rule set naming the same jurisdiction.
//
// A duplicate is refused rather than merged or last-one-wins, because two rule
// sets for one country are two answers to "may we write to this person" — and
// the failure would be silent: whichever set the engine happened to read would
// look like the country's law.
func preflightMessagingRules(e extension.Extension, messagingCodes map[jurisdiction.Code]extension.Name) error {
	for _, r := range e.Messaging {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if owner, dup := messagingCodes[r.Jurisdiction]; dup {
			return fmt.Errorf("compose: extensions %q and %q both declare messaging rules for jurisdiction %q",
				owner, e.Name, r.Jurisdiction)
		}
		// The core-collision check its jurisdiction sibling carries. No core
		// caller registers a rule set today, and the day one does an extension
		// naming the same code must meet this composed error rather than the
		// registry's panic — a boot that dies in a registry tells an operator
		// far less than one that names both declarers.
		if _, taken := messagingrules.For(r.Jurisdiction); taken {
			return fmt.Errorf("compose: extension %q declares messaging rules for jurisdiction %q, which the core already registers",
				e.Name, r.Jurisdiction)
		}
		messagingCodes[r.Jurisdiction] = e.Name
	}
	return nil
}

// preflightRetentionClasses validates one pack's declared floors: class
// name, period, and anchor each carry their own published grammar, and a
// class may be declared once — two floors for the same class with
// different Keep/Anchor would leave the engine picking one silently.
func preflightRetentionClasses(unit extension.Name, code jurisdiction.Code, ret jurisdiction.Retention) error {
	if ret == nil {
		return nil
	}
	seen := make(map[jurisdiction.RetentionClassName]bool)
	for _, class := range ret.Classes() {
		if err := class.Name.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q, jurisdiction %q: %w", unit, code, err)
		}
		if seen[class.Name] {
			return fmt.Errorf("compose: extension %q, jurisdiction %q declares retention class %q twice", unit, code, class.Name)
		}
		seen[class.Name] = true
		if err := class.Keep.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q, jurisdiction %q, class %q: %w", unit, code, class.Name, err)
		}
		if err := class.Anchor.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q, jurisdiction %q, class %q: %w", unit, code, class.Name, err)
		}
	}
	return nil
}
