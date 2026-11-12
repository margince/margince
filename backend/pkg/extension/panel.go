// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The panel surface: a unit's own component, drawn inline on a core page. The
// core decides where the slot sits and fences each panel off, so one unit's
// failure stays in its own panel.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// PanelSlot names a place on a core page that draws units' panels.
type PanelSlot string

const (
	// SlotContactPanel is the end of a contact page's research drawer, after its lists.
	SlotContactPanel PanelSlot = "record_panel.contact"
	// SlotCompanyPanel is the end of a company page's Contacts tab, after the reach card.
	SlotCompanyPanel PanelSlot = "record_panel.company"
	// SlotDealPanel is the end of a deal page's Edit details drawer, after its lists.
	SlotDealPanel PanelSlot = "record_panel.deal"
)

// PanelSlots is every slot a core page draws. The frontend's list mirrors it.
//
// Held by: TestTheFrontendPanelSlotsMatchTheGoOnes (backend/gates/frontendentryslots_test.go)
func PanelSlots() []PanelSlot {
	return []PanelSlot{SlotContactPanel, SlotCompanyPanel, SlotDealPanel}
}

// Valid reports whether a core page draws this slot.
func (s PanelSlot) Valid() bool {
	return slices.Contains(PanelSlots(), s)
}

// panelExport is a named export a bundler can import and React can mount.
var panelExport = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)

// Panel is one of the unit's components, drawn in one slot with the record the
// page shows. The unit's frontend package exports it by name; the component
// renders its own card and draws nothing when it has nothing to say.
type Panel struct {
	Slot PanelSlot
	// Export is the component's named export in the unit's frontend package.
	Export string
	// Object is the unit's RBAC object a reader needs read on to be shown the
	// panel. The generator refuses one that none of the unit's operations
	// gates on.
	Object string
}

// Validate checks what one declaration can show on its own. The export and
// the operation checks need the unit's files, so the generator runs them.
func (p Panel) Validate(unit Name) error {
	if !p.Slot.Valid() {
		return fmt.Errorf("%w: panel slot %q is not one a core page draws (%s)", ErrInvalid, p.Slot, joinPanelSlots())
	}
	if !panelExport.MatchString(p.Export) {
		return fmt.Errorf("%w: panel export %q must be a component name: a capital letter, then letters and digits", ErrInvalid, p.Export)
	}
	namespace, err := unit.Namespace()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(p.Object, namespace+"_") {
		return fmt.Errorf("%w: panel object %q must be one of the unit's own RBAC objects (%s_<object>)", ErrInvalid, p.Object, namespace)
	}
	return nil
}

func joinPanelSlots() string {
	names := make([]string, 0, len(PanelSlots()))
	for _, slot := range PanelSlots() {
		names = append(names, string(slot))
	}
	return strings.Join(names, ", ")
}
