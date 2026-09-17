// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"sort"
	"strings"
)

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

// EmailRequestSource names reminders created by the internal request worker.
// Their source identity links task state to the original email obligation.
const EmailRequestSource = "email_request"

// NoActivityReminderSource and CheckInCadenceSource name the quiet-account
// reminders the automation engine mints, and they are reserved for the same
// reason EmailRequestSource is: the activity store keys its idempotent replay
// on (source_system, source_id) and tests nothing else about the row it finds
// — not the source, not captured_by, not the kind. A caller able to spell one
// of these could plant a row under a reminder's key and have the scan read it
// back as already asked, so the reminder is never written and the silence
// looks like success.
//
// renewal_reminder is deliberately absent: it carries no natural key, because
// its due date stays on the anchor rather than a fixed horizon.
const (
	NoActivityReminderSource = "no_activity_reminder"
	CheckInCadenceSource     = "check_in_cadence"
)

// internalSourceSystems are the exact identities only an internal writer may
// spell. Exact names rather than a second prefix: these are already written
// into rows, and renaming them to fit a namespace would strand every row that
// carries the old spelling.
var internalSourceSystems = map[string]bool{
	EmailRequestSource:       true,
	NoActivityReminderSource: true,
	CheckInCadenceSource:     true,
}

// ReservedSourceSystem reports whether a client-supplied source system
// trespasses on the importer's namespace.
func ReservedSourceSystem(sourceSystem string) bool {
	return internalSourceSystems[sourceSystem] || strings.HasPrefix(sourceSystem, ReservedSourceSystemPrefix)
}

// internalSourceSystemList names the reserved identities for a refusal, sorted
// so the message is stable. Derived from the map rather than written out again:
// a fourth identity must not be addable without the refusal naming it.
func internalSourceSystemList() string {
	names := make([]string, 0, len(internalSourceSystems))
	for name := range internalSourceSystems {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ReservedError refuses a client write into the importer's namespace,
// naming the field it arrived on. One type rather than one per module:
// the rule is a single invariant — no client-facing path may write this
// namespace — and three copies of it would be three places for the next
// provenance field to be forgotten.
type ReservedError struct{ Field, Value string }

func (e *ReservedError) Error() string {
	return e.Field + " " + e.Value + " is reserved for internal writes; omit it or choose ordinary provenance outside the " + ReservedSourceSystemPrefix + " namespace and " + internalSourceSystemList()
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
		return &ReservedError{Field: field, Value: value}
	}
	return nil
}
