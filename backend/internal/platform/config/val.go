// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import "fmt"

// Val is the boot value of a setting that is not stored.
func Val[T any](c *Config, s *Setting[T]) T {
	v, _ := ValWithSource(c, s)
	return v
}

// ValWithSource panics on a stored or unregistered setting: both are wiring
// mistakes a boot test catches, and a zero value returned instead would run.
func ValWithSource[T any](c *Config, s *Setting[T]) (T, Origin) {
	if s.Stored() {
		panic(fmt.Sprintf("config: %s is stored; read it with StoredVal", s.Key()))
	}
	r, ok := c.values[s.Key()]
	if !ok {
		panic(fmt.Sprintf("config: %s is not in this program's registry", s.Key()))
	}
	v, ok := r.value.(T)
	if !ok {
		var zero T
		return zero, r.origin
	}
	return v, r.origin
}
