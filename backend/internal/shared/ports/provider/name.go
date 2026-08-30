// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provider

// What a provider may be called.
//
// The name is not decoration: it is the discriminator on every row a run
// touches, the `connector:<name>` provenance on the values it buys, and a value
// the API publishes as a pattern-constrained string. It is a pattern rather
// than an enum because which providers exist is a deployment fact — what this
// binary composed — and an enum would assert that the legal set is identical in
// every installation.
//
// A name the pattern refuses reaches a client as a response the schema refuses,
// so the check belongs where a name ENTERS the system: an adapter with an
// illegal name is one whose author is told now, rather than through a client
// that validates and reports a breach far from the cause.
//
// Held by: TestEveryRegisteredNameRuleIsTheContractsOwn
// (backend/gates/providername_test.go)

import "regexp"

const (
	// NamePattern is the shape a provider name must have, spelled exactly as
	// the contract spells it so a gate can compare the two as text.
	NamePattern = `^[a-z][a-z0-9_]*$`
	// NameMaxLength is the ceiling the contract publishes.
	NameMaxLength = 32
)

// nameRe is NamePattern compiled once.
var nameRe = regexp.MustCompile(NamePattern)

// ValidName reports whether a name is one the contract can carry.
func ValidName(name string) bool {
	return len(name) <= NameMaxLength && nameRe.MatchString(name)
}
