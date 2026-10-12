// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import "fmt"

// Between is the validator of a whole-number setting held to lowest..highest.
// The unit names the number in the refusal an admin reads.
func Between(unit string, lowest, highest int) func(int) error {
	return func(n int) error {
		if n < lowest || n > highest {
			return fmt.Errorf("choose %d..%d %s, not %d", lowest, highest, unit, n)
		}
		return nil
	}
}
