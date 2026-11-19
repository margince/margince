// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

// For is the guide of one object's target: its "object.target" entry where one
// object's column means something else, or else the target's own.
func For(object, target string) (Column, bool) {
	if c, ok := Columns[object+"."+target]; ok {
		return c, true
	}
	c, ok := Columns[target]
	return c, ok
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
		guide, ok := For(object, target)
		if !ok {
			continue
		}
		for _, name := range append([]string{guide.Label.En, guide.Label.De, guide.Label.Vi}, guide.Aliases...) {
			claim(name, target)
		}
	}
	return out
}
