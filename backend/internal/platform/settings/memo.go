// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

// ValueMemoKey is the key a transaction remembers an applied setting under.
// Every setting writer invalidates it before its change hooks read again.
func ValueMemoKey(key string) string {
	return "setting:" + key
}
