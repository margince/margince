// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

// headerAliases are the other names exports most often give a core field,
// keyed like Columns. A field's label in any language needs no entry here.
var headerAliases = map[string][]string{
	"company.display_name": {"Name"},
	"full_name":            {"Name"},
	"company_name":         {"Company"},
	"contact.company_name": {"Company"},
	"domain":               {"Website", "Webseite", "Web"},
	"phone":                {"Phone number", "Telephone", "Telefonnummer"},
	"address.line1":        {"Address", "Adresse", "Street", "Straße"},
	"size_band":            {"Number of employees", "Anzahl Mitarbeiter"},
}

// For is the guide of one object's target: its "object.target" entry where one
// object's column means something else, or else the target's own.
func For(object, target string) (Column, bool) {
	if c, ok := Columns[object+"."+target]; ok {
		return c, true
	}
	c, ok := Columns[target]
	return c, ok
}

// aliasesOf are the other names of one object's target, read like For.
func aliasesOf(object, target string) []string {
	if names, ok := headerAliases[object+"."+target]; ok {
		return names
	}
	return headerAliases[target]
}

// HeaderNames maps each folded name a header may give a target to that target.
// The names are its label in each language and its aliases. A name two targets
// share maps to "", so neither is suggested.
func HeaderNames(object string, targets []string, fold func(string) string) map[string]string {
	out := map[string]string{}
	claim := func(name, target string) {
		folded := fold(name)
		if folded == "" {
			return
		}
		if held, seen := out[folded]; seen && held != target {
			out[folded] = ""
			return
		}
		out[folded] = target
	}
	for _, target := range targets {
		if guide, ok := For(object, target); ok {
			claim(guide.Label.En, target)
			claim(guide.Label.De, target)
			claim(guide.Label.Vi, target)
		}
		for _, alias := range aliasesOf(object, target) {
			claim(alias, target)
		}
	}
	return out
}
