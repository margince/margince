// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import "fmt"

// secretFileOf is the path a secret was read from when it was declared as
// ${file:path}, and empty for any other declaration.
func secretFileOf(s Any, declared string, origin Origin) string {
	if !s.Secret() {
		return ""
	}
	if kind, _, ok := parseRef(declared); !ok || kind != refFile {
		return ""
	}
	return origin.File
}

// RereadSecret answers a secret setting as it stands now. A ${file:path} value
// is read from that file again, with the resolver and checks Load used. Any
// other declaration is answered as Load resolved it, because a process's
// environment cannot change while it runs.
func RereadSecret[T any](c *Config, s *Setting[T]) (T, Origin, error) {
	value, origin := ValWithSource(c, s)
	path := c.values[s.Key()].secretFile
	if path == "" {
		return value, origin, nil
	}
	var zero T
	text, _, _, err := resolveRef(refFilePrefix+path+"}", c.readFile, nil)
	if err != nil {
		return zero, origin, fmt.Errorf("%s: %w", origin.Name, err)
	}
	v, err := parseChecked(s, text)
	if err != nil {
		return zero, origin, fmt.Errorf("%s: %w", origin.Name, err)
	}
	typed, ok := v.(T)
	if !ok {
		return zero, origin, fmt.Errorf("%s: re-read value is %T, not the setting's type", origin.Name, v)
	}
	return typed, origin, nil
}
