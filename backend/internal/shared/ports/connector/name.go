// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// What a messaging transport may be called.
//
// The name is the discriminator on every activity a transport captures, the
// `connector:<name>` provenance on what it writes, the key a reply path
// resolves a recipient through, and a value the API publishes as
// `ProviderRef` — a pattern-constrained string rather than an enum, because
// which transports exist is a deployment fact including any extension unit
// present.
//
// A name the pattern refuses reaches a client as a response the schema refuses.
// So the check belongs where a transport ENTERS the registry: an extension
// author with an illegal name is told at composition time, rather than through
// a client that validates and reports a breach far from the cause.
//
// SEPARATE from provider.ValidName, which governs licensed DATA providers under
// the `Provider` schema. The two rules are identical today and are two schemas
// on purpose: a transport and a data vendor are different things, and one of
// them can gain a character the other must not.
//
// Held by: TestEveryRegisteredNameRuleIsTheContractsOwn
// (backend/gates/providername_test.go)

import "regexp"

const (
	// NamePattern is the shape a transport name must have, spelled exactly as
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
