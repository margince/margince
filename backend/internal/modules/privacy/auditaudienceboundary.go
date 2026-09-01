// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Which audit rows answer to an activity's audience, and what survives when one
// is withheld.
//
// An audit image describing an activity carries that activity's content — the
// subject verbatim, an attachment's user-supplied filename, a scheduled send's
// subject line. The compliance log is admin-only and deliberately crosses row
// scope, so the audience is the only rule left standing over those images, and
// it is applied here rather than in the read: auditlog.go asks the question,
// this file defines what it is asking about.
//
// Three lists have to agree — the governed types, the route that resolves each
// one's activity, and the per-type key sets — and auditgovernedtypes_test.go is
// what fails when they drift.

import (
	"bytes"
	"encoding/json"
)

// auditGovernedTypes are the entity types whose audit image answers to an
// activity's audience: the activity itself, plus the four collateral records
// that hang off one and carry its content into their own images.
//
// It is the corpus auditActivityRouteSQL resolves and the key set
// auditImageGovernanceKeys is defined over, so a type present in one and absent
// from the other is what auditgovernedtypes_test.go fails on.
var auditGovernedTypes = []string{
	entityTypeActivity, "attachment", "attachment_extraction", "transcript_read", "scheduled_send",
}

// auditActivityRouteSQL resolves the activity an audit row's image describes,
// for every type in auditGovernedTypes, as ONE expression.
//
// One expression rather than one join per type because the audience arm is a
// correlated subquery over workspace membership: rendering it five times would
// evaluate it five times per row and register its parameters five times over.
//
// The four collateral routes, each verified against the baseline schema:
//
//   - attachment is polymorphic AND carries activity_id. The capture path writes
//     both (capturedfiles.go); a manual upload writes only the pair
//     (attachmentupload.go). Reading either column is what covers both writers.
//     Reading one alone leaves the other writer's rows unresolved, and an
//     unresolved row is withheld — no disclosure, so no disclosure test can see
//     it, while every manually uploaded file's image collapses to the withheld
//     marker even for a reader the thread is open to.
//     TestAnUploadedFileOnAnOpenThreadKeepsItsName is the case that fails.
//   - attachment_extraction reaches its activity through attachment, two hops.
//   - transcript_read carries activity_id NOT NULL.
//   - scheduled_send carries activity_id once released and anchor_activity_id for
//     a reply; an origin_kind='account' send that has not been released has
//     NEITHER, by its own CHECK constraint. Those rows resolve to NULL and are
//     withheld, which is the correct answer rather than a gap: no activity's
//     audience can admit an image whose activity does not exist.
const auditActivityRouteSQL = `
		  SELECT CASE a.entity_type
		           WHEN 'activity' THEN a.entity_id
		           WHEN 'attachment' THEN (
		             SELECT coalesce(att.activity_id,
		                             CASE WHEN att.entity_type = 'activity' THEN att.entity_id END)
		               FROM attachment att WHERE att.id = a.entity_id)
		           WHEN 'attachment_extraction' THEN (
		             SELECT coalesce(att.activity_id,
		                             CASE WHEN att.entity_type = 'activity' THEN att.entity_id END)
		               FROM attachment_extraction ext
		               JOIN attachment att ON att.id = ext.attachment_id
		              WHERE ext.id = a.entity_id)
		           WHEN 'transcript_read' THEN (
		             SELECT tr.activity_id FROM transcript_read tr WHERE tr.id = a.entity_id)
		           WHEN 'scheduled_send' THEN (
		             SELECT coalesce(ss.activity_id, ss.anchor_activity_id)
		               FROM scheduled_send ss WHERE ss.id = a.entity_id)
		         END AS activity_id`

// auditActivityJoin reaches the activity an audit row's image describes, and
// only for rows that describe one.
//
// The route resolves first and the activity is joined on its result, so
// entity_id is never matched against activity.id directly: entity_id is a bare
// uuid across every entity type, and a direct match is what would let a person's
// id collide with an activity's and withhold an image that has no audience to
// answer to.
const auditActivityJoin = `
		LEFT JOIN LATERAL (` + auditActivityRouteSQL + `) aud_route ON true
		LEFT JOIN activity ` + auditActivityAlias + `
		  ON ` + auditActivityAlias + `.id = aud_route.activity_id`

// auditImageGovernanceKeys are the keys an activity's audit image may carry
// that describe the MUTATION rather than the activity's content — who the
// audience became, which record a relink pointed at, which fields moved.
//
// The audience governs an activity's CONTENT. It has no claim over the record
// of an administrative act performed on that activity, and withholding one
// destroys governance data rather than protecting anything: the row recording
// "this conversation was limited to its participants, naming 3 members" carries
// nothing a reader outside the audience is not entitled to, and is precisely
// what a compliance reader is looking for.
//
// The map is an ALLOWLIST and the redaction is fail-closed: a key that is not
// here is dropped, so a new writer that starts recording content into an audit
// image cannot leak it by default.
//
// SCOPE: this map is the ACTIVITY's key set. The four collateral types have
// their own, in auditCollateralGovernanceKeys, because a shared map would have
// to be the union of five vocabularies and would then admit each type's keys on
// every other type — the direction that quietly re-widens an allowlist.
//
// Each entry carries a predicate on the VALUE, because a key alone is not a
// safety property. `body` is the case that forced it: the writer reduces it to a
// presence flag and says so in a comment, but nothing bound the READER to that —
// one plausible edit turning `delta["body"] = true` into the body itself would
// have handed an out-of-audience admin the confidential text of a limited
// conversation through this endpoint, passing every gate in the tree.
//
// `body` is the only entry actually constrained. The rest are anyValue, so the
// "a writer starts nesting content under this key" scenario applies unchanged
// to `kind`, `source_system` or a timestamp: those are judged safe by what the
// activity read surface answers for them, which is a claim about today's
// writers rather than a guard. Tightening one is a one-line predicate.
//
// TestRedactionKeepsGovernanceAndDropsContent pins both directions, including
// a `body` carrying something other than a boolean.
var auditImageGovernanceKeys = map[string]func(json.RawMessage) bool{
	"audience": anyValue, "member_count": anyValue,
	"entity_type": anyValue, "entity_id": anyValue, "replaced": anyValue,
	"merged_into_id": anyValue,
	// Mirrors what the activity READ surface keeps on a withheld row
	// (activityread.go): kind, direction, occurred_at and source_system are
	// markers it answers; subject, body and SOURCE_ID are what it nils. source_id
	// is deliberately absent here for that reason — it identifies the message at
	// the provider, the capture sink writes it onto the audit image, and admitting
	// it would have this endpoint answer what the record's own read refuses.
	"kind": anyValue, "direction": anyValue, "source_system": anyValue,
	"occurred_at": anyValue, "due_at": anyValue, "remind_at": anyValue,
	"assignee_id": anyValue, "is_done": anyValue,
	// Presence only, and enforced HERE rather than trusted from the writer.
	"body": isJSONBool,
}

// auditCollateralGovernanceKeys are the keys each collateral type's image may
// carry to a reader outside the activity's audience.
//
// Per type rather than one shared set, and derived from what each writer
// actually records rather than from what sounded safe:
//
//   - attachment: capturedfiles.go writes {entity_type, entity_id, category,
//     byte_size, source_system}; documents.go writes a metadata patch over
//     {category, title, doc_state, pinned, supersedes_id}. FILENAME is never
//     admitted — it is user-supplied and is the leak this PR closes — and
//     neither is TITLE, which is a human-authored name for a document on a
//     limited thread and is content by the same argument.
//   - attachment_extraction: extractionread.go writes {attachment_id,
//     requested_by}, both of which are the governance record itself. Withholding
//     them wholly would destroy "who asked to read this file", which is exactly
//     what a compliance reader came for.
//   - transcript_read: transcriptread.go writes {activity_id, requested_by,
//     line_count}. line_count measures the transcript rather than quoting it.
//   - scheduled_send: scheduledsend.go writes {scheduled_at, scheduled_tz,
//     SUBJECT}; scheduledsendfire.go writes {activity_id, delivery_id} on
//     release and {reason} on a hold. Subject is the message's own subject line,
//     verbatim — content, and absent here for the same reason the activity map
//     drops it. `reason` is a fixed hold vocabulary the fire path chooses, not
//     free text from a user.
//
// Fail-closed like the activity map: a key absent here is dropped, so a writer
// that starts recording content into one of these images cannot leak it by
// default. A type absent from this map has NO governance keys and is withheld
// whole, which auditgovernedtypes_test.go turns into a failing test rather than
// a silent loss of governance data.
var auditCollateralGovernanceKeys = map[string]map[string]func(json.RawMessage) bool{
	"attachment": {
		"entity_type": anyValue, "entity_id": anyValue,
		"category": anyValue, "byte_size": anyValue, "source_system": anyValue,
		"doc_state": anyValue, "pinned": anyValue, "supersedes_id": anyValue,
	},
	"attachment_extraction": {
		"attachment_id": anyValue, "requested_by": anyValue,
	},
	"transcript_read": {
		"activity_id": anyValue, "requested_by": anyValue, "line_count": anyValue,
	},
	// Spelled as literals, not as activities.FieldScheduledAt/TZ: privacy is a
	// sibling module and the layout forbids the import. The two spellings are
	// held together by auditgovernedtypes_test.go rather than by the compiler.
	"scheduled_send": {
		"scheduled_at": anyValue, "scheduled_tz": anyValue,
		"activity_id": anyValue, "delivery_id": anyValue, "reason": anyValue,
	},
}

// governanceKeysFor answers which keys survive redaction for one entity type.
//
// The activity's own set and the collateral sets are looked up through one
// function so a caller cannot reach for the wrong map, and a type in neither
// returns nil — every key dropped — rather than falling back to a permissive
// default.
func governanceKeysFor(entityType string) map[string]func(json.RawMessage) bool {
	if entityType == entityTypeActivity {
		return auditImageGovernanceKeys
	}
	return auditCollateralGovernanceKeys[entityType]
}

// anyValue admits a key whose every possible value is metadata about the
// mutation. Named rather than written as an inline closure so the exceptions —
// today just `body` — stand out at a glance in the map above.
func anyValue(json.RawMessage) bool { return true }

// isJSONBool admits only a literal JSON `true` or `false`.
//
// The null check is not redundant: json.Unmarshal into a bool accepts JSON null
// and leaves the bool at its zero value, so a bare Unmarshal admits
// `"body": null` — the one shape where a non-boolean body would survive
// redaction, and survive it with no marker to say anything was examined.
func isJSONBool(raw json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	var flag bool
	return json.Unmarshal(raw, &flag) == nil
}
