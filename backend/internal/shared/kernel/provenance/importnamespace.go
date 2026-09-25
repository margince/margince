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

// EngineReminderSource reports whether a source system is one the automation
// engine stamps on its own quiet-account reminders.
//
// It exists so the caller allowed to write these names — the engine, acting
// under the system principal — can be told apart from every other caller
// without that caller restating which names those are. It is deliberately
// narrower than ReservedSourceSystem: the importer's namespace has its own
// writer and is never admitted here.
//
// Held by: TestAnOrdinaryCallerMayNotStampAReminderIdentity and
// TestTheSystemPrincipalDoesNotUnlockTheImporterNamespace
// (backend/internal/modules/activities/provider_reminderidentity_test.go)
func EngineReminderSource(sourceSystem string) bool {
	return sourceSystem == NoActivityReminderSource || sourceSystem == CheckInCadenceSource
}

// InternalSourceSystems lists the exact reserved identities, sorted. It is the
// one place that knows the membership, so the refusal message and any census
// over it read the same set rather than two copies that can drift.
func InternalSourceSystems() []string {
	names := make([]string, 0, len(internalSourceSystems))
	for name := range internalSourceSystems {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// internalSourceSystemList names the reserved identities for a refusal. Derived
// rather than written out again: a fourth identity must not be addable without
// the refusal naming it.
func internalSourceSystemList() string {
	return strings.Join(InternalSourceSystems(), ", ")
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

// ImporterNamespace reports whether a source system sits in the importer's
// prefix, and nothing else.
//
// Narrower than ReservedSourceSystem on purpose: that one also holds the three
// exact internal identities, which no import may spell. The door that admits a
// declared importer asks this, so admitting an importer never admits
// email_request, no_activity_reminder or check_in_cadence.
func ImporterNamespace(sourceSystem string) bool {
	return strings.HasPrefix(sourceSystem, ReservedSourceSystemPrefix)
}

// DisplaySourceSystem renders a source system for a reader.
//
// author.via is set verbatim from the column, so an imported row would
// otherwise read "Logged in mirror:hubspot by …". The prefix is machinery for
// the replay key, not something a reader should ever see.
func DisplaySourceSystem(sourceSystem string) string {
	return strings.TrimPrefix(sourceSystem, ReservedSourceSystemPrefix)
}

// DisplayVia is DisplaySourceSystem over the nullable column, for the four
// record reads that project author.via straight out of it.
//
// Here rather than four identical locals in four modules: the stripping rule
// belongs to the namespace, and a fifth read added later gets it by calling
// this rather than by remembering that it exists.
func DisplayVia(sourceSystem *string) *string {
	if sourceSystem == nil {
		return nil
	}
	shown := DisplaySourceSystem(*sourceSystem)
	return &shown
}

// RefuseWire guards BOTH provenance fields a create wire can carry, in one
// statement so a mapper spends one line rather than two `if err != nil` blocks.
//
// source_system is checked first because it is the field the reserved
// namespace is actually keyed on; a caller sending both reserved values hears
// about that one.
func RefuseWire(source string, sourceSystem *string) error {
	if sourceSystem != nil {
		if err := Refuse("source_system", *sourceSystem); err != nil {
			return err
		}
	}
	return Refuse("source", source)
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
