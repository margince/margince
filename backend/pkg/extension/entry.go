// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The entry surface: a unit's way into its own screen from a core screen. The
// unit names the slot, the words and the glyph. The core decides where the
// entry sits and how it looks, so a unit cannot restyle a core page.

import (
	"fmt"
	"slices"
	"strings"
)

// EntrySlot names a place on a core screen that offers a unit's entries.
type EntrySlot string

const (
	// SlotLogActivity is the Log activity form, on any record it is opened on.
	SlotLogActivity EntrySlot = "log_activity"
	// SlotContactHeader is the verb row in a contact page's header.
	SlotContactHeader EntrySlot = "record_header.contact"
	// SlotCompanyHeader is the verb row in a company page's header.
	SlotCompanyHeader EntrySlot = "record_header.company"
	// SlotDealHeader is the verb row in a deal page's header.
	SlotDealHeader EntrySlot = "record_header.deal"
	// SlotContactMark is the small note beside a contact page's header verbs.
	SlotContactMark EntrySlot = "record_mark.contact"
	// SlotCompanyMark is the small note beside a company page's header verbs.
	SlotCompanyMark EntrySlot = "record_mark.company"
	// SlotDealMark is the small note beside a deal page's header verbs.
	SlotDealMark EntrySlot = "record_mark.deal"
)

// EntrySlots is every slot a core screen renders. The frontend's list mirrors
// it, and the gate below fails when the two differ.
//
// Held by: TestTheFrontendEntrySlotsMatchTheGoOnes (backend/gates/frontendentryslots_test.go)
func EntrySlots() []EntrySlot {
	return []EntrySlot{
		SlotLogActivity, SlotContactHeader, SlotCompanyHeader, SlotDealHeader,
		SlotContactMark, SlotCompanyMark, SlotDealMark,
	}
}

// Valid reports whether a core screen renders this slot.
func (s EntrySlot) Valid() bool {
	return slices.Contains(EntrySlots(), s)
}

// EntryIcon names a design-system glyph an entry may lead with. The names are
// lucide's own, so a reader of either side can find the drawing.
type EntryIcon string

// The glyphs an entry may lead with, each spelled as lucide names it.
const (
	IconClipboardList EntryIcon = "clipboard-list"
	IconFileText      EntryIcon = "file-text"
	IconMapPin        EntryIcon = "map-pin"
	IconCamera        EntryIcon = "camera"
	IconCheckSquare   EntryIcon = "check-square"
	IconMessageSquare EntryIcon = "message-square"
	IconPlug          EntryIcon = "plug"
)

// EntryIcons is every glyph an entry may name. The frontend's icon map mirrors
// it.
//
// Held by: TestTheFrontendEntryIconsMatchTheGoOnes (backend/gates/frontendentryslots_test.go)
func EntryIcons() []EntryIcon {
	return []EntryIcon{IconClipboardList, IconFileText, IconMapPin, IconCamera, IconCheckSquare, IconMessageSquare, IconPlug}
}

// Valid reports whether the frontend can draw this glyph.
func (i EntryIcon) Valid() bool {
	return slices.Contains(EntryIcons(), i)
}

// Entry is one way into the unit's screen, offered in one slot. Pressing it
// opens `#/ext/<unit>[/<Target>]` with the record's type and id in the query.
type Entry struct {
	Slot EntrySlot
	// Label is a copy key from the unit's own frontend/i18n files. The
	// generator refuses one that is missing from any locale.
	Label string
	Icon  EntryIcon
	// Target is a path segment under the unit's screen, or empty for the
	// screen itself.
	Target string
	// Object is the unit's RBAC object a reader needs read on to see the
	// entry. The generator refuses one that none of the unit's operations
	// gates on.
	Object string
}

// Validate checks what one declaration can show on its own. The copy and the
// operation checks need the unit's files, so the generator runs them.
func (e Entry) Validate(unit Name) error {
	if !e.Slot.Valid() {
		return fmt.Errorf("%w: entry slot %q is not one a core screen renders (%s)", ErrInvalid, e.Slot, joinSlots())
	}
	if !e.Icon.Valid() {
		return fmt.Errorf("%w: entry icon %q is not one the design system offers an entry", ErrInvalid, e.Icon)
	}
	if !strings.HasPrefix(e.Label, "ext") || strings.ContainsAny(e.Label, " \t\n") {
		return fmt.Errorf("%w: entry label %q must be a copy key from the unit's own i18n files", ErrInvalid, e.Label)
	}
	if e.Target != "" && !nameGrammar.MatchString(e.Target) {
		return fmt.Errorf("%w: entry target %q must be one path segment of lower-case letters, digits and single hyphens", ErrInvalid, e.Target)
	}
	namespace, err := unit.Namespace()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(e.Object, namespace+"_") {
		return fmt.Errorf("%w: entry object %q must be one of the unit's own RBAC objects (%s_<object>)", ErrInvalid, e.Object, namespace)
	}
	return nil
}

func joinSlots() string {
	names := make([]string, 0, len(EntrySlots()))
	for _, slot := range EntrySlots() {
		names = append(names, string(slot))
	}
	return strings.Join(names, ", ")
}
