// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importkeys answers whether a unique key of an import row is already
// held. An earlier row of the same run or a stored record may hold it. The
// import's test run and its commit ask the same questions here, so they reach
// the same verdict for every row.
package importkeys

// Kind is one unique key a record holds.
type Kind int

// The kinds of key a record holds.
const (
	Email Kind = iota
	LinkedIn
	Domain
)

// Key is one key value and the record type holding it.
type Key struct {
	Object string
	Kind   Kind
	Value  string
}

// Ledger holds the keys of every row a run has landed, or predicts will land,
// across every part of a ZIP. The test run writes nothing, so a later row
// meets an earlier one only here.
type Ledger map[Key]bool

// Claim records a landing row's keys. An empty value holds nothing.
func (l Ledger) Claim(object string, values map[Kind]string) {
	for kind, value := range values {
		if value != "" {
			l[Key{Object: object, Kind: kind, Value: value}] = true
		}
	}
}

// Held reports whether an earlier row of one of these record types holds the
// value as a key of this kind.
func (l Ledger) Held(kind Kind, value string, holders ...string) bool {
	for _, holder := range holders {
		if value != "" && l[Key{Object: holder, Kind: kind, Value: value}] {
			return true
		}
	}
	return false
}
