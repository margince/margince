// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "slices"

// DedupeEvidenceFields is every field name this module can write into a
// dedupe candidate's evidence snapshot.
//
// It exists because the snapshot is stored as free JSON and read by a client
// that must print each field in the reader's own language. A client translates
// what it recognises; a field it does not recognise is either printed raw — the
// database's own column name, at a person — or dropped, which loses the row
// that explains why the pair was raised.
//
// Held by: TestEveryDedupeEvidenceFieldIsNameableOnTheWire
// (backend/dedupeevidencefields_test.go), which fails in BOTH directions
// against the contract's own enum.
func DedupeEvidenceFields() []string {
	fields := []string{
		fieldDisplayName,
		fieldLegalName,
		fieldFullName,
		fieldEmail,
		fieldPhone,
		fieldMatchedLane,
		fieldChannelIdentity,
	}
	slices.Sort(fields)
	return fields
}

// DedupeEvidenceSignals is every verdict this module can write into a dedupe
// candidate's evidence snapshot.
//
// Same obligation as the field names, and the same failure when it drifts: a
// signal the client cannot name costs its row the whole comparison, so a pair
// arrives named but with nothing said about WHY it was raised. The field list
// alone is not enough to prevent that — an identity-conflict pair's field is
// recognised and its signal is not, and the row is dropped either way.
//
// Held by: TestEveryDedupeEvidenceSignalIsNameableOnTheWire
// (backend/dedupeevidencefields_test.go), which fails in BOTH directions
// against the contract's own enum.
func DedupeEvidenceSignals() []string {
	signals := []string{
		evidenceSignalCollide,
		evidenceSignalOneSided,
		evidenceSignalExactConflict,
	}
	slices.Sort(signals)
	return signals
}
