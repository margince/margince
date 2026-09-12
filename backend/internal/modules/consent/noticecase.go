// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the installation owes a contact it obtained without asking them, and
// whether it has told them yet.
//
// contact_acquisition_evidence records HOW a contact came to exist. The duty
// that follows is a separate fact: a contact acquired from a list, a referral or
// an import never asked to hear from us, and Art. 14 gives the controller one
// month to say we hold their data and how to object. A case per acquisition
// makes that duty enumerable — "who have we not told yet" is a query rather
// than an audit.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NoticeRule names which disclosure duty a case records. Art. 13 is owed when
// the data came FROM the subject, Art. 14 when it came from somewhere else —
// different deadlines and different content, so the case says which it is
// rather than leaving a later reader to re-derive it from the acquisition kind.
type NoticeRule string

const (
	// RuleArt13 — collected from the subject. The disclosure is owed AT
	// collection, so a case in this rule is already due when it is created.
	RuleArt13 NoticeRule = "art13"
	// RuleArt14 — obtained from anywhere else. One month from the acquisition.
	RuleArt14 NoticeRule = "art14"
)

// NoticeState is where a case stands.
//
// There is deliberately NO `overdue`. Overdue is a reading of the clock against
// due_at, not a fact anybody writes: a stored one would make a case overdue
// only once a sweep had run, so a job that failed to fire would leave every
// late case looking on time — with nothing failing anywhere to say so.
// data_subject_request settles it the same way: its status vocabulary carries
// no overdue, and OpenDSRsDueSoonest (dsrlane.go) orders by due_at rather than
// filtering on one. The paged queue in dsr.go orders by id — a different read
// with a different job, and not the precedent meant here.
type NoticeState string

// fieldState is the name of the state field wherever it crosses a boundary:
// the request property, the validation error that names it, the response key
// and the audit payload. One spelling, because a caller reading a refusal has
// to find the field the refusal names, and an audit reader filtering on it has
// to match what every writer wrote.
const fieldState = "state"

// fieldRule is the name of the rule field wherever it crosses a boundary, for
// the same reason fieldState is: three audit payloads carry it, and a reader
// filtering audit rows on which disclosure duty moved has to match what every
// writer wrote.
const fieldRule = "rule"

// The states a notice case rests in. Open is owed and untouched; assigned is
// owed with somebody's name on it; queued means a disclosure is on its way;
// blocked means we cannot send one yet and says why. Four states END a duty and
// all four record when: completed is a disclosure that was sent and delivered,
// provided_elsewhere and exempt_with_reason each say why no disclosure was
// needed from here, and not_required is the older way of closing a case with no
// reason attached.
const (
	NoticeOpen     NoticeState = "open"
	NoticeAssigned NoticeState = "assigned"
	NoticeQueued   NoticeState = "queued"
	// NoticeCompleted — a disclosure this installation sent was delivered.
	NoticeCompleted NoticeState = "completed"
	// NoticeProvidedElsewhere — the subject already has the information, and
	// it did not come from a mail this installation sent. Somebody told them
	// in a meeting, or a colleague wrote from their own mailbox. The duty is
	// met; the evidence is the officer's statement, and the row carries it.
	NoticeProvidedElsewhere NoticeState = "provided_elsewhere"
	// NoticeExemptWithReason — the duty does not apply, on a ground the officer
	// states. Art. 14(5) disapplies it where the subject already has the
	// information, where notice is impossible or disproportionate, or where
	// disclosure is laid down by law. Distinct from NoticeNotRequired, which
	// records the same conclusion with no reason attached.
	NoticeExemptWithReason NoticeState = "exempt_with_reason"
	NoticeBlocked          NoticeState = "blocked"
	NoticeNotRequired      NoticeState = "not_required"
)

// noticeStates is every state a case can be in, and the ONE place they are
// enumerated. Both the queue's idea of "still owed" and the census that checks
// it read this slice, so a sixth state cannot be added to the vocabulary while
// one of them keeps an older idea of it.
//
// Held by: TestTheQueueAsksForEveryUnresolvedState (noticecase_test.go) — it
// fails when a state is added here without somebody deciding whether a case
// sitting in it is still owed, which is the drift a second list would hide.
//
// gatekit:fixture the notice-case state vocabulary, mirroring the table CHECK
var noticeStates = []NoticeState{
	NoticeOpen, NoticeAssigned, NoticeQueued, NoticeCompleted,
	NoticeProvidedElsewhere, NoticeExemptWithReason,
	NoticeBlocked, NoticeNotRequired,
}

// terminalNoticeStates are the states a discharged duty rests in. Membership
// here is what takes a case off the queue, so a new state is on the queue until
// somebody decides otherwise — the safe direction: a duty wrongly shown costs a
// look, one wrongly hidden is invisible.
func terminalNoticeStates() map[NoticeState]bool {
	return map[NoticeState]bool{
		NoticeCompleted:         true,
		NoticeNotRequired:       true,
		NoticeProvidedElsewhere: true,
		NoticeExemptWithReason:  true,
	}
}

// excusingNoticeStates are the terminal states that end a duty WITHOUT this
// installation having sent anything. Both say why in resolution_note, which the
// table's resolution_shape CHECK also holds, and membership here is what makes
// the note required rather than each writer remembering to ask for one.
func excusingNoticeStates() map[NoticeState]bool {
	return map[NoticeState]bool{
		NoticeProvidedElsewhere: true,
		NoticeExemptWithReason:  true,
	}
}

// unresolvedNoticeStates is "still owed", derived from the vocabulary rather
// than listed a second time. A state added to noticeStates reaches the queue by
// existing, instead of leaving this read with an older idea of open.
func unresolvedNoticeStates() []string {
	terminal := terminalNoticeStates()
	var out []string
	for _, s := range noticeStates {
		if terminal[s] {
			continue
		}
		out = append(out, string(s))
	}
	return out
}

// OpenNoticeCase is one duty still owed: whose it is, what rule put it there,
// and by when.
//
// ContactID is carried because the duty is discharged on that contact's own
// screen — there is no notice-case screen to route to, so a card naming only the
// case would prompt a reader with nowhere to go.
type OpenNoticeCase struct {
	ID        ids.UUID
	ContactID ids.ContactID
	Rule      NoticeRule
	DueAt     time.Time
	Blocked   bool
}

// openNoticeLaneDefault mirrors the DSR lane's small page for the same reason:
// the lane prompts, it is not the queue.
const openNoticeLaneDefault = 8

// NoticeCaseInput is one duty, as the writer states it.
type NoticeCaseInput struct {
	ContactID     ids.ContactID
	AcquisitionID ids.UUID
	Rule          NoticeRule
	DueAt         time.Time
	AllowedRoutes []string
	State         NoticeState
	OwnerUserID   *ids.UUID
	BlockedReason *string
	// CompletedAt is supplied by the caller, never read from the wall clock
	// here — the same division data_subject_request makes for due_at, and what
	// lets a test drive a deadline without waiting for one. Required exactly
	// when State is terminal, which the table's own CHECK also holds.
	CompletedAt *time.Time
}

// OpenNoticeCasesDueSoonest lists the duties nobody has discharged, soonest
// deadline first.
//
// Gated as the subject-request queue is: a notice case says how a named contact
// was obtained and whether we have told them, which is the same disclosure the
// DSR queue makes about who exercised a right. Reusing privacy_request rather
// than minting an object means an installation that delegated its privacy inbox
// already delegated this with it.
func (s *Store) OpenNoticeCasesDueSoonest(ctx context.Context, limit int) ([]OpenNoticeCase, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = openNoticeLaneDefault
	}
	var out []OpenNoticeCase
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, contact_id, rule, due_at, state = 'blocked'
			  FROM privacy_notice_case
			 WHERE state = ANY($1)
			 ORDER BY due_at, id
			 LIMIT $2`, unresolvedNoticeStates(), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c OpenNoticeCase
			if err := rows.Scan(&c.ID, &c.ContactID, &c.Rule, &c.DueAt, &c.Blocked); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// OpenNoticeCaseTx records one duty, in the transaction that created the
// acquisition it is owed for.
//
// IDEMPOTENT BY THE DATABASE, not by a check here. The contact.created consumer
// is at-least-once, so a redelivery reaches this a second time; the unique
// index on acquisition_id refuses the duplicate and ON CONFLICT DO NOTHING
// makes that refusal the expected outcome rather than an error the caller has
// to tell apart from a real one.
func OpenNoticeCaseTx(ctx context.Context, tx pgx.Tx, in NoticeCaseInput) error {
	if in.State == "" {
		in.State = NoticeOpen
	}
	routes := in.AllowedRoutes
	if routes == nil {
		routes = []string{}
	}
	terminal := terminalNoticeStates()[in.State]
	if terminal == (in.CompletedAt == nil) {
		return &ValidationError{
			Field:  "completed_at",
			Reason: "a terminal notice case states when it became one, and a live one has not",
		}
	}
	// The table's blocked_shape CHECK holds this too. Refused HERE as well so
	// the caller gets a named field rather than a wrapped constraint violation
	// — a raw one reads as a database fault, and the writer would be blamed for
	// what is a bad argument.
	if (in.State == NoticeBlocked) == (in.BlockedReason == nil) {
		return &ValidationError{
			Field:  "blocked_reason",
			Reason: "a blocked notice case says why, and one that is not blocked gives no reason",
		}
	}
	// RETURNING id, so the audit below fires only when a row was actually
	// written. ON CONFLICT DO NOTHING makes a redelivery a no-op, and auditing
	// one would record a duty being opened that already existed — an audit
	// trail that says something happened twice is worse than one that is quiet.
	var caseID ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO privacy_notice_case
		       (contact_id, acquisition_id, rule, due_at, allowed_routes,
		        state, owner_user_id, blocked_reason, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (acquisition_id) DO NOTHING
		RETURNING id`,
		in.ContactID, in.AcquisitionID, string(in.Rule), in.DueAt, routes,
		string(in.State), in.OwnerUserID, in.BlockedReason, in.CompletedAt).Scan(&caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The duty was already recorded for this acquisition. Nothing happened,
		// so nothing is audited.
		return nil
	}
	if err != nil {
		return fmt.Errorf("record the notice case owed for this acquisition: %w", err)
	}
	// Audited because a notice case is a compliance record: "when did this
	// workspace learn it owed this contact a disclosure, and what opened the
	// case" is the question an auditor asks, and audit_log is where the answer
	// lives. No event: nothing outside this module acts on a case being opened,
	// and an event no consumer reads is a contract nobody can change later.
	if _, err := storekit.Audit(ctx, tx, "create", "privacy_notice_case", caseID, nil, map[string]any{
		fieldRule: string(in.Rule), "due_at": in.DueAt, fieldState: string(in.State),
	}); err != nil {
		return fmt.Errorf("audit the notice case: %w", err)
	}
	return nil
}
