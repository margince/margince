// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strconv"
	"strings"
)

// Placeholders binds a complete value list, including for INSERT … SELECT.
// Its length follows the arguments, so extending a writer cannot reuse a slot.
func Placeholders[T any](arguments []T) string {
	holders := make([]string, len(arguments))
	for index := range arguments {
		holders[index] = "$" + strconv.Itoa(index+1)
	}
	return strings.Join(holders, ",")
}
