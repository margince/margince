// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "reflect"

func rawCapturePurgerMissing(purge RawCapturePurger) bool {
	if purge == nil {
		return true
	}
	value := reflect.ValueOf(purge)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return true
	}
	return false
}
