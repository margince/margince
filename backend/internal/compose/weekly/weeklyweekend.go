// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The deal as it stood when the week closed, not as it stands today.
//
// Four of the deal block's counts describe a POPULATION rather than an event:
// how many deals were open, how many carried a next step, how many were
// multi-threaded, how many had a sound close date. The others count things that
// happened inside the window and are bounded by it, so they answer the same way
// whenever they are asked. These four read the deal row, and the deal row is
// current.
//
// That is invisible when the review runs the moment the week closes and wrong
// every other time. The dispatcher retries all week for a rep whose review
// landed un-narrated, so a review written on Thursday counted Thursday's
// pipeline as Sunday's — and the review is write-once, so the wrong number is
// the permanent record of that week.
//
// The fix is to walk the audit spine backwards from the closing instant and
// undo every change made since, which the spine can answer for status, the
// close date and its provisional flag.
//
// OWNERSHIP IS NOT REWOUND, and that is a stated limit rather than an oversight.
// The rep's deals are selected by CURRENT owner_id, so a deal reassigned after
// the week closed lands on the new owner's week and leaves the old owner's. The
// image is there to read — deallinks.go sets owner_id through the patch — but
// the selection feeds four counts (open_deals, moves, dwell, cat), and each
// needs its own answer to whose week a reassigned deal's event belongs in.
// Those are product questions, and guessing them would put somebody else's work
// on a rep's scorecard, which is the defect this file exists to fix. Issue 5086
// carries it. Archival is the exception in shape: the
// archive verb writes no field images (dealarchive.go audits the lifecycle, not
// the column), so the VERB is the signal and the reverse-apply reads actions
// rather than images for that one field.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/privacy"
)

// weekEndDealsSQL reconstructs each of the rep's deals as of the closing
// instant.
//
// One pass over the spine per deal rather than a snapshot table: the audit rows
// already exist and already carry the images, and a projection written beside
// them would be a second answer to a question the spine can answer, drifting the
// first time somebody wrote one and not the other.
//
// THE ERASURE BOUNDARY IS THE POINT, not a detail. The spine is append-only, so
// a scrub cannot rewrite the images captured before it — the images of an erased
// deal are still sitting there, and a reverse-apply that read them would put
// what was certified destroyed back onto a rep's weekly review. privacy's own
// boundary is what decides which rows are readable, taken through
// UnscrubbedImageSQL rather than respelled here: that helper's comment says
// plainly that almost-the-same is how the boundary comes to mean two moments,
// and this is a fifth reader of it, not a second copy.
//
// A deal whose reconstruction would cross its own tombstone is not quietly
// reported at its current values and not quietly dropped. It is counted as
// unreconstructible, and a block carrying any such deal says so rather than
// publishing a number it cannot stand behind.
//
// NO WRITER TOMBSTONES A DEAL TODAY, so this excludes nothing in practice: the
// scrub verbs are written against person, lead, activity, attachment,
// deal_room_participant, scheduled_send and the AI records. It is here because
// it must be here BEFORE deal erasure lands rather than after — a boundary added
// afterwards has already served the images it was meant to withhold. The day the
// set widens, gates/scrubbedentitytypes_test.go fails and asks what a frozen
// week should report.
//
// The verbs are the field-image ones. `update` and `restore` carry the column
// patches (the two the update door may write, per auditverb), `advance_stage`
// carries the deal's stage patch and can move status with it, and `archive`
// carries the lifecycle. Every other verb's payload is
// evidence ABOUT an operation rather than an image of the row, and reverse-
// applying one would fabricate a change that never happened — the same closed
// set privacy's field history projects, for the same reason.
func weekEndDealsSQL(endPos, verbsPos string) string {
	// Each fragment already closes its own CTE with a comma; the pieces are
	// concatenated, not punctuated.
	return fmt.Sprintf(rewindSQL+foldSQL+weekEndRowSQL,
		endPos, privacy.UnscrubbedImageSQL("a", verbsPos))
}

// rewindSQL reads every post-cutoff change to one of the rep's deals, with the
// erasure boundary applied per row, and names the deals it may not rebuild.
const rewindSQL = `
		-- Every post-cutoff change to one of the rep's deals, with the erasure
		-- boundary applied per row.
		rewind AS (
		  SELECT a.entity_id, a.action, a.before, a.occurred_at, a.id,
		         %[2]s AS readable
		    FROM audit_log a
		    JOIN mine ON mine.id = a.entity_id
		   WHERE a.entity_type = 'deal'
		     AND a.occurred_at >= %[1]s
		     AND a.action IN ('update', 'advance_stage', 'archive', 'restore')),
		-- A deal is unreconstructible when ANY post-cutoff row of it is behind a
		-- scrub. Asked over the whole rewind rather than one row: a readable
		-- change followed by an erased one still means the spine cannot be
		-- trusted to describe this deal's week.
		scrubbed AS (
		  SELECT DISTINCT entity_id FROM rewind WHERE NOT readable),`

// foldSQL rewinds each column by its own oldest post-cutoff change and reads
// the archival lifecycle from the verb.
const foldSQL = `
		-- EACH COLUMN IS REWOUND BY ITS OWN OLDEST CHANGE, not by one shared row.
		-- A patch carries only the columns it moved, so the first post-cutoff
		-- edit is authoritative for what IT touched and silent about the rest.
		-- Reading a single row and falling back to current values everywhere else
		-- would undo the first edit and keep the second, producing a row that
		-- existed at no instant: last week's status beside this week's close date.
		first_image AS (
		  SELECT r.entity_id, col.name AS column_name,
		         (array_agg(r.before -> col.name
		           ORDER BY r.occurred_at, r.id))[1] AS value
		    FROM rewind r
		    CROSS JOIN LATERAL (VALUES
		      ('expected_close_date'), ('close_date_provisional'), ('status')
		    ) AS col(name)
		   WHERE r.before ? col.name
		   GROUP BY r.entity_id, col.name),
		-- One row per deal, pivoted. jsonb has no max(), and none is wanted: each
		-- (deal, column) group holds exactly one value already, so the aggregate
		-- is only collapsing three rows into three columns.
		rewound AS (
		  SELECT entity_id,
		         (array_agg(value) FILTER (WHERE column_name = 'expected_close_date'))[1]
		           AS expected_close_date,
		         (array_agg(value) FILTER (WHERE column_name = 'close_date_provisional'))[1]
		           AS close_date_provisional,
		         (array_agg(value) FILTER (WHERE column_name = 'status'))[1] AS status
		    FROM first_image GROUP BY entity_id),
		-- Archival is read from the VERB, because the archive writer records the
		-- lifecycle and passes no field images (dealarchive.go audits with nil
		-- images either side). A post-cutoff 'archive' therefore means the deal
		-- was live when the week closed.
		--
		-- ONLY 'archive'. There is no un-archive writer for a deal — dealarchive.go
		-- has ArchiveDeal and no counterpart — so 'archive' is the only lifecycle
		-- verb a deal ever carries, and a second one says the same thing as the
		-- first. 'restore' is NOT the opposite of it: auditverb calls that verb
		-- "an ordinary update in every respect except what the trail calls it",
		-- written by the reversal path over any field. Reading it as an un-archive
		-- would mark a deal archived because somebody undid a name correction.
		-- The deal was live at the cutoff only if it was archived after it AND
		-- carries no archive row from BEFORE it. Both halves are needed:
		-- retention's archiveDeal writes archived_at unconditionally, so a deal
		-- already archived can take a SECOND archive row later, and reading the
		-- post-cutoff verb alone would report that deal as live through a week it
		-- spent archived. The current archived_at cannot answer this — it holds
		-- the LATEST stamp, which is the post-cutoff one.
		lifecycle AS (
		  SELECT DISTINCT r.entity_id
		    FROM rewind r
		   WHERE r.action = 'archive'
		     AND NOT EXISTS (
		       SELECT 1 FROM audit_log prior
		        WHERE prior.entity_type = 'deal' AND prior.entity_id = r.entity_id
		          AND prior.action = 'archive' AND prior.occurred_at < %[1]s)),`

// weekEndRowSQL is the deal as it stood when the week closed: a rewound value
// per column where one moved, the current value where none did.
const weekEndRowSQL = `
		week_end AS (
		  SELECT m.id, m.stage_id, m.pipeline_id,
		         -- A column with no post-cutoff change never moved, so the current
		         -- value IS the week-end value. PRESENCE decides that, never the
		         -- value: a patch clearing a column records JSON null, which says
		         -- "it was empty" — and COALESCE on the extracted text would read
		         -- that as "no image" and fall through to today's value, silently
		         -- giving the week a date it did not have.
		         CASE WHEN r.expected_close_date IS NOT NULL
		              THEN (r.expected_close_date #>> '{}')::date
		              ELSE m.expected_close_date END AS expected_close_date,
		         CASE WHEN r.close_date_provisional IS NOT NULL
		              THEN (r.close_date_provisional #>> '{}')::boolean
		              ELSE m.close_date_provisional END AS close_date_provisional,
		         CASE WHEN r.status IS NOT NULL
		              THEN r.status #>> '{}'
		              ELSE m.status END AS status,
		         -- Archived after the cutoff means live at it; otherwise the
		         -- current archival state stands, never having moved.
		         CASE WHEN l.entity_id IS NOT NULL THEN false
		              ELSE m.archived_at IS NOT NULL END AS was_archived,
		         (s.entity_id IS NOT NULL) AS unreconstructible
		    FROM mine m
		    LEFT JOIN rewound r ON r.entity_id = m.id
		    LEFT JOIN lifecycle l ON l.entity_id = m.id
		    LEFT JOIN scrubbed s ON s.entity_id = m.id)`
