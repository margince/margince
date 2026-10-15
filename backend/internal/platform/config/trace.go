// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import "strings"

// TraceLines are the sources read, then one line per setting. A secret shows
// its source and never its value.
func (c *Config) TraceLines() []string {
	lines := append([]string(nil), c.trace...)
	for _, e := range c.Entries() {
		value := "<secret>"
		switch {
		case !e.Secret && e.Value != nil:
			value = *e.Value
		case !e.Secret:
			value = "<unset>"
		}
		lines = append(lines, e.Key+" = "+value+" ("+originText(e.Origin)+")")
	}
	return lines
}

func originText(o Origin) string {
	if o.Source == SourceUnset {
		return "unset"
	}
	parts := []string{string(o.Source), o.Name, o.File}
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " ")
}
