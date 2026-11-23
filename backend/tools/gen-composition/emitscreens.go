// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The SPA's composed SCREEN registry, split out of emitfrontend.go so that file
// stays under the length cap.

import (
	"fmt"
	"strings"
)

// extScreensGen emits the SPA's composed screen registry: which unit package
// renders which unit. The empty set reproduces the committed vanilla stub
// byte-for-byte (held by stubMatchesVanilla).
//
// It is apart from frontendGen because it exists only for a unit that ships a
// frontend package. A unit with none gets the published-operations card.
//
// Import order follows the sorted unit order, so the file is a function of the
// enabled set. The same import brings in each panel and the app a unit
// declares. A declared export the package lacks fails the composed typecheck.
func extScreensGen(units []extensionUnit, panels map[string][]declaredPanel, apps map[string]declaredApp) []byte {
	var imports, entries, panelEntries, appEntries strings.Builder
	for _, u := range units {
		if u.Frontend == nil {
			continue
		}
		ident := screenIdent(u.Name)
		named := make([]string, 0, len(panels[u.Name])+1)
		slots := make([]string, 0, len(panels[u.Name]))
		for _, p := range panels[u.Name] {
			local := panelIdent(u.Name, p.Export)
			named = append(named, p.Export+" as "+local)
			slots = append(slots, fmt.Sprintf("    %s: %s,\n", tsString(p.Slot), local))
		}
		if app, ok := apps[u.Name]; ok {
			local := unitCamel(u.Name) + "__app__" + app.Export
			named = append(named, app.Export+" as "+local)
			fmt.Fprintf(&appEntries, "  %s: %s,\n", tsString(u.Name), local)
		}
		if len(named) == 0 {
			fmt.Fprintf(&imports, "import %s from %s;\n", ident, tsString(u.Frontend.Export))
		} else {
			fmt.Fprintf(&imports, "import %s, { %s } from %s;\n", ident, strings.Join(named, ", "), tsString(u.Frontend.Export))
			if len(slots) > 0 {
				fmt.Fprintf(&panelEntries, "  %s: {\n%s  },\n", tsString(u.Name), strings.Join(slots, ""))
			}
		}
		fmt.Fprintf(&entries, "  %s: %s,\n", tsString(u.Name), ident)
	}
	var b strings.Builder
	b.WriteString(extScreensGenHeader)
	b.WriteString(imports.String())
	writeTSRecord(&b, "extensionScreens", entries.String())
	writeTSRecord(&b, "extensionPanels", panelEntries.String())
	writeTSRecord(&b, "extensionApps", appEntries.String())
	return []byte(b.String())
}
