// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// What a unit asks the core to lend it. A need is a capability the core owns,
// its AI models or its object store. The unit names it with a reason in
// Extension.Needs. The core grants what is named and answers ErrNotDeclared to
// the rest. The settings page lists each need to the admin who installs the unit.

import (
	"errors"
	"fmt"
	"slices"
)

// ErrNotDeclared is a call to a core capability the unit's Extension.Needs does
// not list.
var ErrNotDeclared = errors.New("extension: this unit does not declare the capability in Extension.Needs")

// Capability names one thing of the core's a unit can ask to use.
type Capability string

const (
	// CapabilityModels admits Runtime.Models.
	CapabilityModels Capability = "models"
	// CapabilityObjects admits Runtime.Objects.
	CapabilityObjects Capability = "objects"
)

// Need is one capability a unit declares, with the one sentence that tells the
// admin who installs it what the unit does with it.
type Need struct {
	Capability Capability
	Reason     string
}

// Validate refuses a capability the core does not offer, and a reason that is
// not one printable sentence.
func (n Need) Validate() error {
	switch n.Capability {
	case CapabilityModels, CapabilityObjects:
	default:
		return fmt.Errorf("extension: %q is not a capability; declare %q or %q", string(n.Capability), CapabilityModels, CapabilityObjects)
	}
	if err := Description(n.Reason).Validate(); err != nil {
		return fmt.Errorf("extension: the reason for needing %q: %w", string(n.Capability), err)
	}
	return nil
}

// Declares reports whether the unit lists capability in Needs.
func (e Extension) Declares(capability Capability) bool {
	return slices.ContainsFunc(e.Needs, func(n Need) bool { return n.Capability == capability })
}
