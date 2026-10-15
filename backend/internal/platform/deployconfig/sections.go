// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"reflect"
	"strings"
)

// Sections are the top-level keys of margince.yaml this package decodes. The
// settings loader reads the same file and must leave these keys to Load. The
// set comes from Config's own tags, so no list beside it can drift.
func Sections() map[string]bool {
	out := map[string]bool{}
	for field := range reflect.TypeFor[Config]().Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}
