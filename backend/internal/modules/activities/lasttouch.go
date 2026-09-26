// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The one read the automation module's clock scan needs (Task 14a,
// automation/seams.go's ActivityScan): which linked entities have gone
// quiet. Sourced from this module's OWN tables (activity + activity_link)
// rather than the schema-maintained last_activity_at columns (deal, contact,
// company; migration 1787032690's triggers), because this scan asks a
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
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
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
// the link-id coalesce, and the company walk — and it sits apart from the
// scan so the scan reads as what it does with the rows.
//
// $1/$4/$5 are the source and captured_by the automation engine stamps —
// excluded only TOGETHER, since source alone is a client's to spell. $5 is
// the namespaced form: captured_by carries the principal's ID and every job
// binds its own ("system:time-scan"), so matching the bare "system" alone
// missed the engine's own writes and let a reminder reset the clock it
// reads. $2 the cutoff, $3 the cap, $6 the asking handler; providersPos is
// where the mail providers are bound when the store asks about the owner's
// mailbox.
func lastTouchCandidateQuery(mailbox *OwnerMailbox, providersPos int) string {
	return storekit.SQLf(`
			WITH genuine AS (
				SELECT a.id, a.occurred_at
				FROM activity a
				WHERE a.archived_at IS NULL
				  `+auth.OriginIsEngagement("a")+`
				  `+auth.AudienceWorkspaceOnly("a")+`
				  AND NOT (a.source = $1
				           AND (a.captured_by = $4 OR a.captured_by LIKE $5))
			), live_accounts AS (
				SELECT o.id, o.owner_id
				FROM company o
				JOIN deal d ON d.company_id = o.id
				           AND d.status = 'open' AND d.archived_at IS NULL
				WHERE o.archived_at IS NULL AND o.created_at < $2
			), direct AS (
				SELECT al.entity_type AS entity_type,
				       %[1]s AS entity_id,
				       max(g.occurred_at) AS last_touch
				FROM activity_link al
				JOIN genuine g ON g.id = al.activity_id
				WHERE al.entity_type <> '%[2]s'
				GROUP BY al.entity_type, %[1]s
			), accounts AS (
				SELECT '%[2]s' AS entity_type,
				       reach.company_id AS entity_id,
				       max(g.occurred_at) AS last_touch
				FROM (%[3]s) reach
				JOIN genuine g ON g.id = reach.activity_id
				GROUP BY reach.company_id
			), quiet AS (
				SELECT entity_type, entity_id, last_touch FROM direct
				UNION ALL
				SELECT entity_type, entity_id, last_touch FROM accounts
			), absorbing_accounts AS (
				SELECT la.id
				FROM live_accounts la
				JOIN accounts a ON a.entity_id = la.id
				WHERE a.last_touch < $2%[6]s
			)
			SELECT q.entity_type, q.entity_id, q.last_touch
			FROM quiet q
			WHERE q.last_touch < $2
			  AND (%[4]s)
			  AND NOT EXISTS (%[5]s)%[7]s
			ORDER BY q.last_touch, q.entity_id
			LIMIT $3`,
		linkIDCoalesceQualified("al"),
		datasource.RecordCompany,
		CompanyReachSet(),
		lastTouchEligibility(),
		openReminderHoldsEntity(),
		mailbox.ownerMailboxUnseen("la.owner_id", providersPos),
		mailbox.ownerMailboxUnseen(quietRecordOwner(), providersPos))
}

// lastTouchEligibility is the per-type live-work test, and the collapse that
// keeps one silence to one question.
//
// Each arm asks two things: does this record carry live work worth a reminder,
// and is somebody else already being asked about the same silence. A deal and
// an employed stakeholder fold into the account absorbing them; an account is
// drawn on its own liveness, less any record it absorbs whose reminder is still
// unanswered. A lead answers to nobody, so it has no collapse.
func lastTouchEligibility() string {
	return storekit.SQLf(`
			(q.entity_type = '%[1]s' AND EXISTS (
			   SELECT 1 FROM deal d
			   WHERE d.id = q.entity_id
			     AND d.status = 'open' AND d.archived_at IS NULL
			     AND d.created_at < $2)
			 AND NOT EXISTS (
			   SELECT 1 FROM deal d
			   JOIN absorbing_accounts aa ON aa.id = d.company_id
			   WHERE d.id = q.entity_id))
		 OR (q.entity_type = '%[2]s' AND EXISTS (
			   SELECT 1 FROM live_accounts la WHERE la.id = q.entity_id)
			 AND NOT EXISTS (%[5]s))
		 OR (q.entity_type = '%[3]s' AND EXISTS (
			   SELECT 1 FROM contact p
			   JOIN relationship r ON r.contact_id = p.id
			              AND r.kind = 'deal_stakeholder'
			              AND r.ended_at IS NULL AND r.archived_at IS NULL
			   JOIN deal d ON d.id = r.deal_id
			              AND d.status = 'open' AND d.archived_at IS NULL
			   WHERE p.id = q.entity_id
			     AND p.archived_at IS NULL
			     AND p.created_at < $2)
			 AND NOT EXISTS (%[4]s))
		 OR (q.entity_type = '%[6]s' AND EXISTS (
			   SELECT 1 FROM lead l
			   WHERE l.id = q.entity_id
			     AND l.status IN ('new','contacted','engaged') AND l.archived_at IS NULL
			     AND l.created_at < $2))`,
		datasource.RecordDeal, datasource.RecordCompany, datasource.RecordContact,
		contactCollapsesIntoAccount(),
		openChildReminderHoldsAccount(),
		datasource.RecordLead)
}

// openChildReminderHoldsAccount stops an account being drawn while a record it
// would absorb is still carrying an unanswered reminder.
//
// The hold in openReminderHoldsEntity is keyed on the entity its task is linked
// to, so a task on a CONTACT is invisible when the query asks about that
// contact's employer. Without this arm the two holds miss each other in one
// ordinary sequence: a contact goes quiet while the account is being worked and
// earns its own reminder; later the account goes quiet too and absorbs the
// contact; nothing has answered the first question, so the rep is handed a
// second open task about the same silence.
//
// The collapse and the hold have to agree on scope. Once an account can absorb
// a record, an open reminder on that record is a question about the account,
// and the account waits for the same answer.
func openChildReminderHoldsAccount() string {
	return `SELECT 1 FROM activity t
		         JOIN activity_link tl ON tl.activity_id = t.id
		         LEFT JOIN deal cd ON cd.id = tl.deal_id
		         LEFT JOIN relationship ce ON ce.contact_id = tl.contact_id
		                    AND ce.kind = 'employment'
		                    AND ` + employment.IsCurrentSQL("ce.ended_at") + `
		                    AND ce.archived_at IS NULL
		         WHERE t.kind = 'task'
		           AND t.is_done = false AND t.archived_at IS NULL
		           AND t.source_system = $6
		           AND t.source = $1
		           AND (t.captured_by = $4 OR t.captured_by LIKE $5)
		           AND coalesce(cd.company_id, ce.company_id) = q.entity_id`
}

// contactCollapsesIntoAccount is the contact arm's collapse test: a stakeholder
// currently employed by an ABSORBING account is already covered by that
// account's own reminder, because CompanyReachSet folds a contact's touches
// into their employer (companyscope.go's companyArms). Reminding them
// separately asks one rep about one silence twice.
//
// Absorbing, not merely live: an account only absorbs a record when it is
// ITSELF being drawn. A contact who has gone quiet while their employer is
// worked regularly folds into an account no reminder is ever written for, and
// the silence would be reported by nobody — five reminders turned into none,
// which is worse than the duplication this collapse exists to end.
//
// Employment, not the stakeholder seat: the seat is what makes the contact a
// candidate at all, while employment is what makes the account's anchor include
// this contact's mail. A seat on a live account's deal does not fold the touch,
// so collapsing on the seat would silence a contact nobody else covers.
//
// Its own statement rather than a clause on the arm above, so the employment
// currency test stands alone: gates/employmentcurrency_test.go matches per
// STATEMENT, and mixing this with the seat's own `ended_at IS NULL` would read
// as an employment arm that skips the helper.
func contactCollapsesIntoAccount() string {
	return `SELECT 1 FROM relationship e
		         JOIN absorbing_accounts aa ON aa.id = e.company_id
		         WHERE e.contact_id = q.entity_id
		           AND e.kind = 'employment'
		           AND ` + employment.IsCurrentSQL("e.ended_at") + `
		           AND e.archived_at IS NULL`
}

// openReminderHoldsEntity excludes an entity that already carries an OPEN
// reminder from this handler — the SQL half of "a task still open is not asked
// twice".
//
// Here rather than in the handler's Plan for the reason the eligibility arms
// are here: one pass draws at most 200 candidates, so an entity whose reminder
// is already open would occupy that batch every tick and starve the records
// that still need one. A post-filter cannot give the batch back.
//
// Keyed on source_system so a handler holds only its OWN reminders: widened to
// "any system task", a lead follow-up would hold an entity out of the check-in
// draw entirely. The source/captured_by pair rides along because source alone
// is a client's to spell, exactly as the genuine CTE reads it.
func openReminderHoldsEntity() string {
	return `SELECT 1 FROM activity t
		         JOIN activity_link tl ON tl.activity_id = t.id
		         WHERE tl.entity_type = q.entity_type
		           AND ` + linkIDCoalesceQualified("tl") + ` = q.entity_id
		           AND t.kind = 'task'
		           AND t.is_done = false AND t.archived_at IS NULL
		           AND t.source_system = $6
		           AND t.source = $1
		           AND (t.captured_by = $4 OR t.captured_by LIKE $5)`
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
//   - company — it has at least one open, unarchived deal. The
//     account is what the rep works, so the reminder belongs on the
//     account, once.
//
// An account's last touch is read through the three-arm walk (CompanyReachSet)
// rather than off its own links, and that is the difference between this
// trigger working and not. Capture files mail against the CONTACT it was with,
// so on a real workspace an account's correspondence carries no direct
// company link at all: counting only direct links, an account whose reps
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
//   - contact — they hold a live deal_stakeholder seat on an open deal.
//     Deliberately NOT "their employer has an open deal": that would mint
//     one reminder per employee of every busy account, each one a
//     duplicate of the single company reminder that account already
//     earns.
//   - lead — still in the working part of its lifecycle ('new' or
//     'contacted'); a promoted or disqualified lead is finished business.
//
// Every other entity type activity_link can carry (project today) is
// outside this trigger's vocabulary and never becomes a candidate.
//
// On a store wired WithOwnerMailbox, an owned record is drawn only while its
// owner's mail is visible (quietmailbox.go), in the SQL for the same batch
// reason as eligibility.
//
// The cutoff does double duty: the entity row's own created_at must also
// precede it, so a record created yesterday cannot be "stale" merely
// because activities backfilled onto it are older than N days. One
// cutoff, one meaning — the coarse scan and the handler's precise Match
// (automation/handlers_clock.go) cannot drift onto two thresholds.
// reminder names the handler asking, so the draw can skip an entity whose
// reminder from THAT handler is still open. Each clock handler passes its own
// Spec().Name, which is also the source_system its task carries.
func (s *Store) LastTouchBefore(ctx context.Context, cutoff time.Time, limit int, reminder string) ([]LastTouchCandidate, error) {
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
		args := []any{systemSource, cutoff, limit, systemCapturedBy, systemCapturedByPattern, reminder}
		providersPos := 0
		if s.ownerMailbox != nil {
			args = append(args, s.ownerMailbox.Providers)
			providersPos = len(args)
		}
		rows, err := tx.Query(ctx, lastTouchCandidateQuery(s.ownerMailbox, providersPos), args...)
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
