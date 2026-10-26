// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The selector table: which records each (object_type, category) policy scope
// governs. Split out of retention.go because it is also the AUTHORABLE
// VOCABULARY — retentionscope.go derives what an admin may author from exactly
// these keys (GCS-PARAM-8), so the one list serves the evaluator and the
// authoring surface, and neither can drift from the other.
//
// Adding a key here widens what an admin may author AND moves
// MaxPassDuration's bound, which is derived from the count.

// selectors name the records a (object_type, category) policy governs.
//
// Every record selector asks TWO ages: the date the record carries (created_at,
// closed_at, occurred_at) and how long it has been in this installation
// (entered_at, or an activity's created_at, which no import restates). An import
// states the source system's dates, and a record must not be swept on the day
// it arrives because it was old somewhere else.
// The closed map is deliberate: a policy row with a scope the engine
// does not understand is skipped LOUDLY (logged every pass), never
// half-applied. Every query filters the hold column — and for
// activities, the holds of every linked record plus the statutory floor.
var retentionSelectors = map[string]string{
	"report_edition/": reportingRetentionSelector(),
	// An ARCHIVE policy passes over leads already archived, or it would pick the
	// same batch every night and never reach the rest. An ANONYMIZE policy does
	// not: an archived lead still holds its name and address, so a policy
	// switched from archive to anonymize must reach every lead the archive
	// already took. $3 is the policy's action (dueRecords).
	"lead/unconverted": `SELECT id FROM lead
		WHERE status IN ('new','contacted','engaged') AND (archived_at IS NULL OR $3 = 'anonymize') AND NOT legal_hold
		  AND full_name IS DISTINCT FROM 'Anonymized Lead'
		  AND created_at < now() - make_interval(days => $1)
		  AND entered_at < now() - make_interval(days => $1) LIMIT $2`,
	"activity/": `SELECT a.id FROM activity a
		WHERE a.archived_at IS NULL
		  AND a.occurred_at < now() - make_interval(days => $1) -- occurred_at raw: a retention age in days
		  AND a.created_at < now() - make_interval(days => $1)
		  ` + correspondenceFloorPredicate(3, 4) + `
		  AND NOT EXISTS (SELECT 1 FROM activity_link l
		        LEFT JOIN contact p ON p.id = l.contact_id
		        LEFT JOIN company o ON o.id = l.company_id
		        LEFT JOIN deal d ON d.id = l.deal_id
		        LEFT JOIN lead ld ON ld.id = l.lead_id
		        LEFT JOIN project pj ON pj.id = l.project_id
		        WHERE l.activity_id = a.id
		          AND (coalesce(p.legal_hold, false) OR coalesce(o.legal_hold, false) OR coalesce(d.legal_hold, false)
		               OR coalesce(ld.legal_hold, false) OR coalesce(pj.legal_hold, false)))
		LIMIT $2`,
	"activity/transcript": `SELECT a.id FROM activity a
		WHERE a.source_system = 'transcript' AND a.body IS NOT NULL
		  AND a.occurred_at < now() - make_interval(days => $1) -- occurred_at raw: a retention age in days
		  AND a.created_at < now() - make_interval(days => $1)
		  ` + correspondenceFloorPredicate(3, 4) + `
		  AND NOT EXISTS (SELECT 1 FROM activity_link l
		        LEFT JOIN contact p ON p.id = l.contact_id
		        LEFT JOIN company o ON o.id = l.company_id
		        LEFT JOIN deal d ON d.id = l.deal_id
		        LEFT JOIN lead ld ON ld.id = l.lead_id
		        LEFT JOIN project pj ON pj.id = l.project_id
		        WHERE l.activity_id = a.id
		          AND (coalesce(p.legal_hold, false) OR coalesce(o.legal_hold, false) OR coalesce(d.legal_hold, false)
		               OR coalesce(ld.legal_hold, false) OR coalesce(pj.legal_hold, false)))
		LIMIT $2`,
	"contact/no_consent_no_deal": `SELECT p.id FROM contact p
		WHERE p.archived_at IS NULL AND NOT p.legal_hold
		  AND p.full_name IS DISTINCT FROM 'Erased Subject'
		  AND p.created_at < now() - make_interval(days => $1)
		  AND p.entered_at < now() - make_interval(days => $1)
		  AND NOT EXISTS (SELECT 1 FROM contact_consent pc WHERE pc.contact_id = p.id AND pc.state = 'granted')
		  AND NOT EXISTS (SELECT 1 FROM relationship r
		        WHERE r.kind = 'deal_stakeholder' AND r.contact_id = p.id AND r.archived_at IS NULL)
		LIMIT $2`,
	"deal/lost": `SELECT id FROM deal
		WHERE status = 'lost' AND archived_at IS NULL AND NOT legal_hold
		  AND closed_at < now() - make_interval(days => $1)
		  AND entered_at < now() - make_interval(days => $1) LIMIT $2`,
	// deal/won is authorable and unseeded. A won deal is the commercial record
	// of the relationship, and the product takes no view on when a workspace
	// stops keeping it. UC-GDPR-09's main success scenario authors this policy
	// ("archive deal/won after a shorter period than the default"). Without a
	// selector that worked example could not run.
	"deal/won": `SELECT id FROM deal
		WHERE status = 'won' AND archived_at IS NULL AND NOT legal_hold
		  AND closed_at < now() - make_interval(days => $1)
		  AND entered_at < now() - make_interval(days => $1) LIMIT $2`,
	"ai_call_payload/content": `SELECT id FROM ai_call_payload
		WHERE occurred_at < now() - make_interval(days => $1) LIMIT $2`,
	// The verbatim provider original, aged on its own clock rather than on the
	// activity's. Without this stage only the activity sweep and Art. 17 erasure
	// reach raw_capture, so it grows for as long as the activity policy keeps.
	//
	// JOINED to the activity, and that join is the whole safety of the stage.
	// It carries the statutory floor and the legal-hold test, so an original is
	// never destroyed while the correspondence it is the original OF is
	// shielded; and it leaves the activity row standing, which is what keeps
	// the `(source_system, source_id)` tombstone against a replay — the same
	// reasoning capture's own purge states.
	//
	// It does not reach an original no activity was read from, such as a lead's
	// or a channel update that never became a message. With no activity there is
	// no floor or hold to ask, so the stage leaves the row.
	"raw_capture/": `SELECT rc.id FROM raw_capture rc
		JOIN activity a ON ` + rawCaptureLink + `
		WHERE rc.received_at < now() - make_interval(days => $1)
		  AND a.archived_at IS NULL
		  ` + correspondenceFloorPredicate(3, 4) + `
		  AND NOT EXISTS (SELECT 1 FROM activity_link l
		        LEFT JOIN contact p ON p.id = l.contact_id
		        LEFT JOIN company o ON o.id = l.company_id
		        LEFT JOIN deal d ON d.id = l.deal_id
		        LEFT JOIN lead ld ON ld.id = l.lead_id
		        LEFT JOIN project pj ON pj.id = l.project_id
		        WHERE l.activity_id = a.id
		          AND (coalesce(p.legal_hold, false) OR coalesce(o.legal_hold, false) OR coalesce(d.legal_hold, false)
		               OR coalesce(ld.legal_hold, false) OR coalesce(pj.legal_hold, false)))
		LIMIT $2`,
	// The queue's recorded risk days, aged on the day they describe. A day and
	// its verdicts exist to answer the same-day next-step figure over its
	// window, which counts only days starting inside it; a day starting
	// before the widest window answers nothing. No legal-hold test: the rows
	// carry nothing a hold protects, and the deals they name are untouched.
	"deal_risk_day/": `SELECT id FROM deal_risk_day
		WHERE day_start < now() - make_interval(days => $1) LIMIT $2`,
}
