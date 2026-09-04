// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The one read the automation module's clock scan needs (Task 14a,
// automation/seams.go's ActivityScan): which linked entities have gone
// quiet. Sourced from this module's OWN tables (activity + activity_link)
// rather than the schema-maintained last_activity_at columns (deal, person,
// organization; migration 1787032690's triggers), because this scan asks a
// narrower question those columns do not — it excludes automation-engine
// writes and wants live-work eligibility — and a module
// reaches records only through seams (ADR-0054 §9), and this file is the
// seam's implementation, adapted onto automation.ActivityScan in
// compose/timescan.go.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// LastTouchCandidate is one REMINDER-ELIGIBLE linked entity whose most
// recent GENUINE engagement (across every kind and every link) landed
// before the caller's cutoff. "Genuine" excludes the automation engine's
// own output — source AND captured_by together (systemSource,
// systemCapturedBy, followupresolve.go), because source alone rides the
// client create wire and a planted value must not let a caller hide real
// engagement or move a record's anchor; "eligible" is the live-work test
// each entity type gets in the query — see LastTouchBefore.
type LastTouchCandidate struct {
	EntityType string
	EntityID   ids.UUID
	LastTouch  time.Time
}

// lastTouchCandidateQuery is LastTouchBefore's read: every linked entity's most
// recent genuine engagement, narrowed to the ones carrying live work. It is a
// function rather than a constant because two of its fragments are built —
// the link-id coalesce, and the organization walk — and it sits apart from the
// scan so the scan reads as what it does with the rows.
//
// $1/$4/$5 are the source and captured_by the automation engine stamps —
// excluded only TOGETHER, since source alone is a client's to spell. $5 is
// the namespaced form: captured_by carries the principal's ID and every job
// binds its own ("system:time-scan"), so matching the bare "system" alone
// missed the engine's own writes and let a reminder reset the clock it
// reads. $2 the cutoff, $3 the cap.
func lastTouchCandidateQuery() string {
	return storekit.SQLf(`
			WITH genuine AS (
				SELECT a.id, a.occurred_at
				FROM activity a
				WHERE a.archived_at IS NULL
				  `+auth.OriginIsEngagement("a")+`
				  AND NOT (a.source = $1
				           AND (a.captured_by = $4 OR a.captured_by LIKE $5))
			), direct AS (
				SELECT al.entity_type AS entity_type,
				       %[1]s AS entity_id,
				       max(g.occurred_at) AS last_touch
				FROM activity_link al
				JOIN genuine g ON g.id = al.activity_id
				WHERE al.entity_type <> '%[3]s'
				GROUP BY al.entity_type, %[1]s
			), accounts AS (
				SELECT '%[3]s' AS entity_type,
				       reach.organization_id AS entity_id,
				       max(g.occurred_at) AS last_touch
				FROM (%[6]s) reach
				JOIN genuine g ON g.id = reach.activity_id
				GROUP BY reach.organization_id
			), quiet AS (
				SELECT entity_type, entity_id, last_touch FROM direct
				UNION ALL
				SELECT entity_type, entity_id, last_touch FROM accounts
			)
			SELECT q.entity_type, q.entity_id, q.last_touch
			FROM quiet q
			WHERE q.last_touch < $2
			  AND ((q.entity_type = '%[2]s' AND EXISTS (
			         SELECT 1 FROM deal d
			         WHERE d.id = q.entity_id
			           AND d.status = 'open' AND d.archived_at IS NULL
			           AND d.created_at < $2))
			   OR (q.entity_type = '%[3]s' AND EXISTS (
			         SELECT 1 FROM organization o
			         JOIN deal d ON d.organization_id = o.id
			                    AND d.status = 'open' AND d.archived_at IS NULL
			         WHERE o.id = q.entity_id
			           AND o.archived_at IS NULL
			           AND o.created_at < $2))
			   OR (q.entity_type = '%[4]s' AND EXISTS (
			         SELECT 1 FROM person p
			         JOIN relationship r ON r.person_id = p.id
			                    AND r.kind = 'deal_stakeholder'
			                    AND r.ended_at IS NULL AND r.archived_at IS NULL
			         JOIN deal d ON d.id = r.deal_id
			                    AND d.status = 'open' AND d.archived_at IS NULL
			         WHERE p.id = q.entity_id
			           AND p.archived_at IS NULL
			           AND p.created_at < $2))
			   OR (q.entity_type = '%[5]s' AND EXISTS (
			         SELECT 1 FROM lead l
			         WHERE l.id = q.entity_id
			           AND l.status IN ('new','contacted','engaged') AND l.archived_at IS NULL
			           AND l.created_at < $2)))
			ORDER BY q.last_touch, q.entity_id
			LIMIT $3`,
		linkIDCoalesceQualified("al"),
		datasource.RecordDeal, datasource.RecordOrganization,
		datasource.RecordPerson, datasource.RecordLead,
		OrgReachSet())
}

// LastTouchBefore returns the entities that are BOTH quiet and worth
// reminding about: linked through activity_link, most recent
// GENUINE-engagement activity.occurred_at before cutoff, and carrying
// live work as of that same cutoff — oldest-touch first, capped at limit.
// It is the read automation.TimeScanner's no_activity_for_n_days clock
// candidates are built from.
//
// "No activity for N days" means the rep has not ENGAGED the record — a
// human touch (call, email, meeting, note), an inbound reply, or a
// captured mail (Gmail/IMAP, source "gmail:…"/"imap:…") all count. The
// automation engine's OWN writes (source AND captured_by both "system") do NOT: a
// reminder task the engine created must not look like engagement, or the
// firing would reset its own anchor to ~now, age out of the candidate
// set, then re-surface with the task's timestamp as a fresh anchor and
// nag every N days forever. Excluding the engine's writes keeps the anchor
// pinned to the last real touch, so no_activity_reminder fires ONCE per
// quiet spell (recurring "check in every N days regardless" is
// check_in_cadence's job, not this trigger's).
//
// Eligibility is decided HERE, in the SQL, not in the automation
// handler's Match: one scan pass draws at most clockScanBatchLimit (200)
// candidates per instance (automation/timescan.go), so on a large
// workspace a quiet record nobody is working would occupy that batch on
// every tick forever and starve the records that do deserve a reminder.
// A post-filter cannot fix that; only excluding them from the draw can.
//
// What counts as live work, per type:
//
//   - deal — the deal itself is open and unarchived.
//   - organization — it has at least one open, unarchived deal. The
//     account is what the rep works, so the reminder belongs on the
//     account, once.
//
// An account's last touch is read through the three-arm walk (OrgReachSet)
// rather than off its own links, and that is the difference between this
// trigger working and not. Capture files mail against the PERSON it was with,
// so on a real workspace an account's correspondence carries no direct
// organization link at all: counting only direct links, an account whose reps
// mailed a contact yesterday looked untouched and earned a reminder about a
// relationship somebody is actively working, while an account that never got a
// direct link was never drawn at all. The other three types keep their own
// links, because each is the thing the activity names.
//
// It widens the draw in both directions and that is the point: accounts
// previously invisible become candidates, and accounts previously drawn while
// being worked stop being. Eligibility is unchanged — an account still needs an
// open unarchived deal — so the batch is spent on accounts somebody is working
// rather than on ones nobody is.
//   - person — they hold a live deal_stakeholder seat on an open deal.
//     Deliberately NOT "their employer has an open deal": that would mint
//     one reminder per employee of every busy account, each one a
//     duplicate of the single organization reminder that account already
//     earns.
//   - lead — still in the working part of its lifecycle ('new' or
//     'contacted'); a promoted or disqualified lead is finished business.
//
// Every other entity type activity_link can carry (project today) is
// outside this trigger's vocabulary and never becomes a candidate.
//
// The cutoff does double duty: the entity row's own created_at must also
// precede it, so a record created yesterday cannot be "stale" merely
// because activities backfilled onto it are older than N days. One
// cutoff, one meaning — the coarse scan and the handler's precise Match
// (automation/handlers_clock.go) cannot drift onto two thresholds.
func (s *Store) LastTouchBefore(ctx context.Context, cutoff time.Time, limit int) ([]LastTouchCandidate, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	var out []LastTouchCandidate
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The entity_type literals come from the module's own link
		// vocabulary (linktarget.go), the same source the coalesce
		// expression is built from, so a renamed record type cannot leave a
		// stale string behind in this query.
		rows, err := tx.Query(ctx, lastTouchCandidateQuery(), systemSource, cutoff, limit,
			systemCapturedBy, systemCapturedByPattern)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c LastTouchCandidate
			if err := rows.Scan(&c.EntityType, &c.EntityID, &c.LastTouch); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}
