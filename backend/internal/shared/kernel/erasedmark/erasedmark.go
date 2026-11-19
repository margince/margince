// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package erasedmark is the value an Art. 17 erasure writes into a unique key
// column to say the row is erased, and that the pass destroying its link secret
// reads back. Privacy writes it and activities reads it, and neither imports the
// other.
package erasedmark

// Prefix starts the tombstone of an erased row's key; the row's activity id
// follows, which keeps the column unique.
const Prefix = "erased:"
