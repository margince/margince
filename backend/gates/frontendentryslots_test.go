// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A unit's entry names a slot and a glyph, and a panel names a slot, and two
// sides must agree on each.
// The boot and the generator accept what pkg/extension lists; the SPA renders
// what its own lists hold. A slot only Go knows is an entry that passes every
// check and never shows. A glyph only Go knows is an entry without its icon.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

const (
	frontendEntrySlots = "../frontend/src/lib/extensionentries.ts"
	frontendEntryIcons = "../frontend/src/components/extensionentries.tsx"
	frontendPanelSlots = "../frontend/src/components/extensionpanels.slots.ts"
)

// tsKey reads object keys in either spelling an object literal admits, bare or
// quoted. The slot list is read with tsQuoted.
var tsKey = regexp.MustCompile(`(?m)^\s*"?([a-z][a-z0-9-]*)"?\s*:`)

func TestTheFrontendEntrySlotsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	literal := tsLiteral(t, frontendEntrySlots, "export const EXTENSION_ENTRY_SLOTS = [", "] as const;")
	var inTS []string
	for _, m := range tsQuoted.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	var inGo []string
	for _, slot := range extension.EntrySlots() {
		inGo = append(inGo, string(slot))
	}
	compareSets(t, "entry slot", inGo, inTS)
}

// The list above is a type; a slot shows only where a screen mounts it. So the
// mounted set is read from every `<ExtensionEntries` element under src/.
func TestEverySlotGoAcceptsIsMountedOnAScreen(t *testing.T) {
	t.Parallel()
	mounted, err := mountedSlots("../frontend/src", entriesElement)
	if err != nil {
		t.Fatal(err)
	}
	var inGo []string
	for _, slot := range extension.EntrySlots() {
		inGo = append(inGo, string(slot))
	}
	compareSets(t, "mounted entry slot", inGo, mounted)
}

func TestTheFrontendPanelSlotsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	literal := tsLiteral(t, frontendPanelSlots, "export const EXTENSION_PANEL_SLOTS = [", "] as const;")
	var inTS []string
	for _, m := range tsQuoted.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	compareSets(t, "panel slot", goPanelSlots(), inTS)
}

// A panel slot shows only where a page mounts it, read from every
// `<ExtensionPanels` element under src/.
func TestEveryPanelSlotGoAcceptsIsMountedOnAPage(t *testing.T) {
	t.Parallel()
	mounted, err := mountedSlots("../frontend/src", panelsElement)
	if err != nil {
		t.Fatal(err)
	}
	compareSets(t, "mounted panel slot", goPanelSlots(), mounted)
}

func goPanelSlots() []string {
	var inGo []string
	for _, slot := range extension.PanelSlots() {
		inGo = append(inGo, string(slot))
	}
	return inGo
}

func TestTheFrontendEntryIconsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	literal := tsLiteral(t, frontendEntryIcons, "export const ENTRY_ICONS: Readonly<Record<string, LucideIcon>> = {", "};")
	var inTS []string
	for _, m := range tsKey.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	var inGo []string
	for _, icon := range extension.EntryIcons() {
		inGo = append(inGo, string(icon))
	}
	compareSets(t, "entry icon", inGo, inTS)
}

// tsLiteral returns the text between open and close in path, comments removed
// so a name a comment mentions cannot stand in for a deleted entry.
func tsLiteral(t *testing.T, path, open, closing string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	_, rest, found := strings.Cut(string(source), open)
	if !found {
		t.Fatalf("%s no longer declares %q — this gate is reading a shape that is gone", path, open)
	}
	body, _, found := strings.Cut(rest, closing)
	if !found {
		t.Fatalf("%s: the literal after %q is unterminated", path, open)
	}
	return tsComment.ReplaceAllString(body, " ")
}

func compareSets(t *testing.T, what string, inGo, inTS []string) {
	t.Helper()
	if len(inTS) == 0 {
		t.Fatalf("no %s parsed out of the frontend list — a gate that reads nothing agrees with everything", what)
	}
	for _, name := range inGo {
		if !slices.Contains(inTS, name) {
			t.Errorf("%s %q is accepted by pkg/extension and absent from the frontend, so an entry naming it never renders as declared", what, name)
		}
	}
	for _, name := range inTS {
		if !slices.Contains(inGo, name) {
			t.Errorf("%s %q is in the frontend and absent from pkg/extension, so no unit can declare it", what, name)
		}
	}
}
