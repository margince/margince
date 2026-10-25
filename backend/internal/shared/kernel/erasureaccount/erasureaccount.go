// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package erasureaccount is what one Art. 17 erasure removed, as the erasure
// engine counts it and as the proof of a privacy request keeps it.
package erasureaccount

// Account is the erasure's own count of its work.
type Account struct {
	// Rows maps "<table>.<deleted|updated|inserted>" to the rows the erasure's
	// statements touched there. A row two statements touch counts twice.
	Rows map[string]int64 `json:"rows"`
	// Uncounted names the arms that destroyed something outside the database,
	// where no row count shows how much.
	Uncounted []string `json:"uncounted"`
}

// Empty is the account of a request that erased nothing.
func Empty() Account { return Account{Rows: map[string]int64{}, Uncounted: []string{}} }
