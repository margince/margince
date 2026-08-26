// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package auditverb names what the audit trail calls an update-shaped write.
// It sits in the kernel because the provider seam (shared/ports/datasource)
// carries the verb on its update input and the audit door
// (platform/database/storekit) writes it, and the layer DAG forbids the first
// from importing the second. storekit aliases these names, so there is one
// type and two spellings of the same thing rather than two types.
package auditverb

import "fmt"

// Verb is the action an update-shaped write records. The zero value is
// Update, so a caller that does not care never names one and the update door
// keeps exactly the meaning it had.
//
// Only the reversal path sets it. A restore is an ordinary update in every
// respect except what the trail calls it and the row it names as reversed,
// which is why it travels here rather than in a second write engine.
type Verb string

const (
	Update  Verb = "update"
	Restore Verb = "restore"
)

// OrUpdate resolves the zero value.
func (v Verb) OrUpdate() Verb {
	if v == "" {
		return Update
	}
	return v
}

// Valid reports whether the verb is one the update door may write. The
// audit_log CHECK admits many more; this door admits two, because an update
// path that could write any verb is a way to launder a write into a shape it
// did not take.
func (v Verb) Valid() bool {
	resolved := v.OrUpdate()
	return resolved == Update || resolved == Restore
}

// Trail is the audit context an update-shaped write carries: what the trail
// calls the write, and what it records ABOUT the write. It travels as one value
// from the provider seam down to the audit door so that the six record types
// spell the reversal path once rather than six times, and so that a third piece
// of audit context later needs no change at the seam.
type Trail struct {
	// Verb names the action. The zero value is Update.
	Verb Verb
	// Evidence lands in audit_log.evidence — context ABOUT the mutation, never
	// a field image. A restore names the row it reverses here. Nil is none.
	Evidence map[string]any
}

// Resolve returns the action string the audit doors take and the evidence to
// ride with it, refusing a verb this door may not write. An update path that
// could write any verb is a way to launder a write into a shape it did not
// take, so an unknown verb is stopped here rather than at the audit_log CHECK,
// where the failure would name a constraint instead of the mistake.
//
// Because the action reaches the door as a value and not a literal,
// auditbeforeimage_test.go cannot reduce these call sites to a verb and
// ratifies them instead. What holds them is the chokepoint in
// storekit.AuditWithEvidence, which binds BOTH update-shaped verbs: it refuses
// a restore with no before-image exactly as it refuses an update with none.
func (t Trail) Resolve() (string, map[string]any, error) {
	if !t.Verb.Valid() {
		return "", nil, fmt.Errorf("audit: %q is not a verb the update door may write; "+
			"use auditverb.Update or auditverb.Restore", string(t.Verb))
	}
	return string(t.Verb.OrUpdate()), t.Evidence, nil
}
