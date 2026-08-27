// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import "strings"

// ReservedSourceSystemPrefix namespaces a source_system only an IMPORT
// may write.
//
// The lead and activity stores key their idempotent replay on
// (source_system, source_id), and both columns arrive from the client on
// their create wire. Without a reserved namespace a caller could
// pre-plant a row under a guessed incumbent record id and have the store
// hand it back to a later import as already existing — silently
// suppressing the real record, and (because activities resolve their
// links through the same identity) attaching the incumbent's timeline to
// the planted row. The importer writes inside this namespace; every
// client-facing create path refuses it.
const ReservedSourceSystemPrefix = "mirror:"

// ReservedSourceSystem reports whether a client-supplied source system
// trespasses on the importer's namespace.
func ReservedSourceSystem(sourceSystem string) bool {
	return strings.HasPrefix(sourceSystem, ReservedSourceSystemPrefix)
}

// ReservedSystemSource is the source value the automation engine's own
// writes carry (automation's create executor stamps it on every record a
// workflow mints). A client-facing create must not spell it: the follow-up
// auto-resolver and the duplicate-fold data repair select system-minted
// rows by this value, so a caller who could write it would hand their own
// row — or a colleague's lead's row — to the system's completion and
// archival paths, and claim system provenance in every reader that trusts
// the column. Both of those paths ALSO predicate on captured_by (which no
// client can write), so this reservation is the loud front door rather
// than the only lock.
const ReservedSystemSource = "system"

// ReservedError refuses a client write into a reserved provenance value,
// naming the field it arrived on and why that value is not a client's to
// spell. One type rather than one per module: the rule is a single
// invariant — no client-facing path writes reserved provenance — and
// three copies of it would be three places for the next provenance field
// to be forgotten.
type ReservedError struct{ Field, Value, Reason string }

func (e *ReservedError) Error() string {
	return e.Field + " " + e.Value + " is reserved " + e.Reason
}

// FieldFault states the refusal as caller-fixable, which is how it
// reaches every surface — the HTTP mapper and the MCP tool surface both
// read this rather than each module restating it.
func (e *ReservedError) FieldFault() (field, code, message string) {
	return e.Field, "reserved_source_system", e.Error()
}

// Refuse guards ONE provenance field on a create wire. The flip stamps
// its own writes inside this namespace and reads them back to recognize
// records a crashed attempt landed, which is safe only while nothing
// else can spell the prefix.
func Refuse(field, value string) error {
	if ReservedSourceSystem(value) {
		return &ReservedError{
			Field: field, Value: value,
			Reason: "for imports; omit it or use a value outside the " + ReservedSourceSystemPrefix + " namespace",
		}
	}
	if value == ReservedSystemSource {
		return &ReservedError{
			Field: field, Value: value,
			Reason: "for the system's own writes; use a value naming the real capture source",
		}
	}
	return nil
}
