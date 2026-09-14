// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Taking a disclosure duty, excusing it, or recording that it was met
// elsewhere.
//
// Opening a case and discharging one by sending a mail were the only two things
// anybody could do to a notice case, which left the queue with no way to say
// who is working a duty and no way to close one that the product cannot
// discharge by sending anything. An officer facing a case they had already met
// in a meeting had two options: leave it open forever, or mark it not_required,
// which records the conclusion and destroys the reason. Both read to an auditor
// as a duty nobody handled.
//
// Every write here is a compliance record, so each one is audited with its
// before-image and each one names the seat that made it. None of them emits an
// event: nothing outside this module acts on a case being claimed or excused,
// and an event no consumer reads is a contract nobody can change later. That is
// the same call OpenNoticeCaseTx made.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fieldOwner and fieldNote name the two fields these refusals can be about,
// once each. A caller reading a refusal has to find the field it names, and an
// audit reader filtering on one has to match what every writer wrote.
const (
	fieldOwner          = "owner_user_id"
	fieldResolutionNote = "resolution_note"
)

// noticeNoteLimit mirrors the table's resolution_note CHECK. Refused here as
// well so the caller gets a named field rather than a wrapped constraint
// violation, which reads as a database fault and gets the writer blamed for
// what is a bad argument. Same reason OpenNoticeCaseTx pre-checks the blocked
// shape the table also holds.
const noticeNoteLimit = 500

// NoticeCase is one duty as a reader of the queue sees it, with everything the
// three writes below can have put on it.
type NoticeCase struct {
	ID             ids.UUID
	ContactID      ids.ContactID
	AcquisitionID  ids.UUID
	Rule           NoticeRule
	DueAt          time.Time
	State          NoticeState
	AllowedRoutes  []string
	OwnerUserID    *ids.UUID
	AssignedAt     *time.Time
	Attempts       int
	ResolutionNote *string
	ResolvedBy     *ids.UUID
	BlockedReason  *string
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// noticeCaseColumns is the one spelling of the row, so the read below and any
// later one cannot disagree about what a case is. A second SELECT list over
// this table would drift the moment a column is added: one reader would see it
// and the other would not, and the shape a case has would depend on which path
// asked.
//
// Held by: TestOneSelectListSpellsTheNoticeCaseRow (backend/gates/noticecasecolumns_test.go)
const noticeCaseColumns = `
	id, contact_id, acquisition_id, rule, due_at, state, allowed_routes,
	owner_user_id, assigned_at, attempts, resolution_note, resolved_by,
	blocked_reason, completed_at, created_at, updated_at`

func scanNoticeCase(row pgx.Row) (NoticeCase, error) {
	var c NoticeCase
	err := row.Scan(
		&c.ID, &c.ContactID, &c.AcquisitionID, &c.Rule, &c.DueAt, &c.State,
		&c.AllowedRoutes, &c.OwnerUserID, &c.AssignedAt, &c.Attempts,
		&c.ResolutionNote, &c.ResolvedBy, &c.BlockedReason, &c.CompletedAt,
		&c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// ListNoticeCases answers the queue: every case in the states asked for,
// soonest deadline first.
//
// Gated exactly as OpenNoticeCasesDueSoonest is — a notice case says how a
// named contact was obtained and whether we have told them, which is the same
// disclosure the subject-request queue makes about who exercised a right.
//
// An empty `states` means every unresolved one, which is what a queue asks for
// when it asks for nothing in particular. A caller that wants closed cases has
// to name them, so a screen cannot accidentally show a duty as owed because it
// forgot to filter.
func (s *Store) ListNoticeCases(ctx context.Context, states []NoticeState, limit int) ([]NoticeCase, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > noticeCaseListMax {
		limit = noticeCaseListMax
	}
	wanted := make([]string, 0, len(states))
	for _, st := range states {
		if !knownNoticeState(st) {
			return nil, &ValidationError{Field: fieldState, Reason: "not a notice-case state"}
		}
		wanted = append(wanted, string(st))
	}
	if len(wanted) == 0 {
		wanted = unresolvedNoticeStates()
	}
	var out []NoticeCase
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT`+noticeCaseColumns+`
			  FROM privacy_notice_case
			 WHERE state = ANY($1)
			 ORDER BY due_at, id
			 LIMIT $2`, wanted, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanNoticeCase(rows)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// GetNoticeCase reads one duty, for a surface that opens a single case rather
// than working a page of them.
//
// Gated exactly as the queue is. There is deliberately no narrower read: a seat
// that may not see the list may not see one of its rows either, and a detail
// read that hid what the list showed would be a different answer to the same
// question.
//
// It goes through noticeCaseColumns like every other whole-row read here, so a
// column added to the row reaches the list and the detail together.
//
// Held by: TestOneSelectListSpellsTheNoticeCaseRow
// (backend/gates/noticecasecolumns_test.go)
func (s *Store) GetNoticeCase(ctx context.Context, id ids.UUID) (NoticeCase, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return NoticeCase{}, err
	}
	var out NoticeCase
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanNoticeCase(tx.QueryRow(ctx, `
			SELECT`+noticeCaseColumns+`
			  FROM privacy_notice_case
			 WHERE id = $1`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	})
	return out, err
}

// noticeCaseListMax bounds the queue read. A privacy officer works a page at a
// time, and an unbounded list over a table with one row per acquisition is a
// read that grows with the contact book.
const noticeCaseListMax = 200

// knownNoticeState reports whether a caller-supplied state is in the
// vocabulary, derived from noticeStates rather than listed again — a ninth
// state reaches this by existing.
func knownNoticeState(state NoticeState) bool {
	for _, s := range noticeStates {
		if s == state {
			return true
		}
	}
	return false
}

// AssignNoticeCase gives a case an owner, so the queue shows who is working a
// duty rather than a pile nobody has claimed.
//
// The state moves to `assigned` only from `open`. A queued case already has a
// disclosure on its way and a terminal one is done, so assigning either would
// claim work that is not there to do; a blocked case keeps its state because
// its obstacle is the fact that matters, and claiming it does not clear one.
// Those cases still take the owner — somebody is looking at the obstacle — they
// just do not pretend the duty moved.
func (s *Store) AssignNoticeCase(ctx context.Context, id ids.UUID, owner ids.UUID) (NoticeCase, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return NoticeCase{}, err
	}
	var out NoticeCase
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := lockNoticeCase(ctx, tx, id)
		if err != nil {
			return err
		}
		if terminalNoticeStates()[current.State] {
			return &ValidationError{
				Field:  fieldState,
				Reason: "a duty that has already ended takes no owner",
			}
		}
		next := current.State
		if current.State == NoticeOpen {
			next = NoticeAssigned
		}
		out, err = scanNoticeCase(tx.QueryRow(ctx, `
			UPDATE privacy_notice_case
			   SET state = $2, owner_user_id = $3, assigned_at = now(), updated_at = now()
			 WHERE id = $1
			RETURNING`+noticeCaseColumns, id, string(next), owner))
		if err != nil {
			return fmt.Errorf("give this notice case an owner: %w", err)
		}
		return auditNoticeCase(ctx, tx, current, out, map[string]any{
			fieldOwner: owner.String(),
		})
	})
	return out, err
}

// ExcuseInput is an officer saying a duty ends without this installation
// sending anything for it.
type ExcuseInput struct {
	// State is NoticeExemptWithReason or NoticeProvidedElsewhere. The caller
	// chooses which, because they are different claims: one says the duty does
	// not apply, the other says it was met somewhere else, and an auditor
	// asking "why is this closed" needs them apart.
	State NoticeState
	// Note is the ground, in the officer's own words. Required, which is the
	// whole point of these two states over not_required.
	Note string
	// Now is supplied by the caller rather than read from the wall clock here,
	// the same division OpenNoticeCaseTx makes for CompletedAt — it is what
	// lets a test drive a deadline without waiting for one.
	Now time.Time
}

// ExcuseNoticeCase ends a duty on a stated ground, or records that it was met
// elsewhere.
//
// This is the write not_required could not make. Both this and not_required
// close a case without a mail; only this one keeps the reason, and the reason
// is the record — "the subject already had the information" and "notice was
// impossible" are different defences, and a closed case that cannot say which
// one it relied on is a case nobody can defend.
//
// Refuses a case that has already ended. Re-excusing a completed duty would
// overwrite a real discharge with a claim about one, and the second note would
// read as the reason the first thing happened.
func (s *Store) ExcuseNoticeCase(ctx context.Context, id ids.UUID, in ExcuseInput) (NoticeCase, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return NoticeCase{}, err
	}
	if !excusingNoticeStates()[in.State] {
		return NoticeCase{}, &ValidationError{
			Field:  fieldState,
			Reason: "excusing a duty says it was provided elsewhere or is exempt with a reason",
		}
	}
	note := strings.TrimSpace(in.Note)
	// Trimmed before the emptiness check, so whitespace cannot stand in for a
	// ground — the table's CHECK sees a non-null value and would let " " close
	// a compliance record with nothing written on it.
	if note == "" {
		return NoticeCase{}, &ValidationError{
			Field:  fieldResolutionNote,
			Reason: "ending a duty without sending anything says why",
		}
	}
	// Counted in CHARACTERS, matching the table's char_length CHECK and the
	// contract's maxLength. len() counts bytes, so a 251-character note in
	// German or Vietnamese would be refused here while both the database and
	// the API contract accept it — and the officer writing it would read a
	// limit that does not exist.
	if utf8.RuneCountInString(note) > noticeNoteLimit {
		return NoticeCase{}, &ValidationError{
			Field:  fieldResolutionNote,
			Reason: "a ground is a justification, not a case file",
		}
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return NoticeCase{}, fmt.Errorf("excusing a notice case names the seat that did it: %w", apperrors.ErrPermissionDenied)
	}
	// UserID, not the principal's string ID: that one is prefixed ("human:…")
	// and is an identity, not a foreign key. resolved_by references app_user,
	// so it takes the seat's own id.
	resolver := actor.UserID
	if resolver.IsZero() {
		return NoticeCase{}, fmt.Errorf("excusing a notice case names the seat that did it: %w", apperrors.ErrPermissionDenied)
	}
	var out NoticeCase
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := lockNoticeCase(ctx, tx, id)
		if err != nil {
			return err
		}
		if terminalNoticeStates()[current.State] {
			return &ValidationError{
				Field:  fieldState,
				Reason: "a duty that has already ended is not excused a second time",
			}
		}
		// blocked_reason IS CLEARED, and this is the case that makes it
		// necessary rather than tidy. The table holds that a reason is present
		// exactly when the state is blocked, so an excuse that left the
		// obstacle behind would be refused by the database — and blocked is
		// precisely the state an officer excuses FROM: "we cannot send this"
		// is what leads to "notice is impossible". Keeping the obstacle would
		// also leave the row asserting both that the duty ended and that
		// something is stopping it.
		out, err = scanNoticeCase(tx.QueryRow(ctx, `
			UPDATE privacy_notice_case
			   SET state = $2, resolution_note = $3, resolved_by = $4,
			       completed_at = $5, blocked_reason = NULL, updated_at = now()
			 WHERE id = $1
			RETURNING`+noticeCaseColumns,
			id, string(in.State), note, resolver, in.Now))
		if err != nil {
			return fmt.Errorf("record why this duty ends without a disclosure: %w", err)
		}
		return auditNoticeCase(ctx, tx, current, out, map[string]any{
			// The note itself is NOT audited. It is free prose an officer
			// wrote about a named contact, and copying it into audit_log would
			// put the same personal data in a second place with its own
			// retention. The row carries it, the audit records that a ground
			// was given, and privacy's erasure reaches the row.
			fieldResolutionNote: true,
		})
	})
	return out, err
}

// lockNoticeCase reads a case FOR UPDATE, so the state this write validated
// against is still the state it writes over. Nothing else holds the row between
// a plain read and the update, and two officers excusing the same case would
// otherwise both pass their checks and the second would overwrite the first's
// ground with its own.
func lockNoticeCase(ctx context.Context, tx pgx.Tx, id ids.UUID) (NoticeCase, error) {
	current, err := scanNoticeCase(tx.QueryRow(ctx, `
		SELECT`+noticeCaseColumns+`
		  FROM privacy_notice_case
		 WHERE id = $1
		   FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return NoticeCase{}, apperrors.ErrNotFound
	}
	return current, err
}

// auditNoticeCase writes the compliance record for one change to a case, with
// the before-image the write shape requires. "It is exempt now" without "it was
// open then" cannot tell an excuse from a restatement of one.
func auditNoticeCase(
	ctx context.Context, tx pgx.Tx, before, after NoticeCase, extra map[string]any,
) error {
	now := map[string]any{fieldState: string(after.State), fieldRule: string(after.Rule)}
	for k, v := range extra {
		now[k] = v
	}
	// The before-image carries the OWNER as well as the state, because a
	// reassignment moves the owner while the state stays `assigned` — an image
	// of the state alone would record "assigned to assigned" and lose which
	// seat the work was taken from, which is the only fact a reassignment has.
	was := map[string]any{fieldState: string(before.State)}
	if before.OwnerUserID != nil {
		was[fieldOwner] = before.OwnerUserID.String()
	}
	if _, err := storekit.Audit(ctx, tx, "update", "privacy_notice_case", after.ID,
		was, now); err != nil {
		return fmt.Errorf("audit the notice case: %w", err)
	}
	return nil
}
