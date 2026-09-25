// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Meeting scheduling (features/07 §5c): availability is a 🟢 read that
// PROPOSES, booking is the 🟡 action that commits. Until a calendar
// connector is connected, free/busy derives from the CRM's own record
// — the host's meeting activities in the window — which is exactly the
// single-source-of-truth posture: the CRM cannot see a calendar it was
// never granted, and says so by construction rather than pretending.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/calendarbacking"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The assumed length of a meeting whose end the record does not carry (activity
// has only occurred_at); it refines when a real calendar connector lands.
// defaultSlotDuration applies inside Availability when the caller names no
// duration, so the REST and MCP transports cannot drift.
//
// The business-hours envelope is NOT here any more. Which hours a host is
// bookable in is a fact about that CONTACT — see docs/explanation/scheduling.md —
// so it arrives through hostHours rather than being a pair of numbers this
// package holds for everybody.
const (
	assumedMeetingDuration = time.Hour
	maxProposedSlots       = 20
	maxAvailabilityWindow  = 31 * 24 * time.Hour
	defaultSlotDuration    = 30 * time.Minute
	minSlotDuration        = 15 * time.Minute
	maxSlotDuration        = 8 * time.Hour
)

// SchedulingArgumentError refuses a from/to/start/end/duration combination no
// calendar can answer. Availability and booking share it because they refuse the
// same shape of mistake about the same kind of argument.
//
// Field carries a wire field name and nothing else: both surfaces publish it as
// the MACHINE field — REST as details.errors[].field, the MCP dispatcher as the
// field token in `<field>=<code>` — so an explanation there is unreadable by the
// only party that would act on it. The explanation is Message. And the Code says
// what is actually wrong: a value that was supplied and merely inconsistent is
// not `required`.
type SchedulingArgumentError struct {
	Field   string
	Code    string
	Message string
}

func (e *SchedulingArgumentError) Error() string { return e.Field + ": " + e.Message }

// FieldFault names the argument, the code, and what to fix — one mapping for
// every surface.
func (e *SchedulingArgumentError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}

// The scheduling argument refusals. Package-level values, not built at the call
// site: every bound in the text is read off the constant that enforces it, so a
// bound and its message cannot drift apart.
var (
	errAvailabilityToNotAfterFrom = &SchedulingArgumentError{
		Field: "to", Code: "invalid_date_range",
		Message: "`to` must be later than `from`",
	}
	errAvailabilityWindowTooWide = &SchedulingArgumentError{
		Field: "to", Code: "window_too_wide",
		Message: fmt.Sprintf("the window between `from` and `to` may span at most %d days",
			int(maxAvailabilityWindow/(24*time.Hour))),
	}
	errAvailabilityDurationOutOfRange = &SchedulingArgumentError{
		Field: "duration_minutes", Code: "out_of_range",
		Message: fmt.Sprintf("`duration_minutes` must be between %d and %d",
			int(minSlotDuration.Minutes()), int(maxSlotDuration.Minutes())),
	}
	// The booking twin of errAvailabilityToNotAfterFrom. book_meeting's StageInfo
	// pre-empts the same condition in its own terms so no approval is minted for
	// an unbookable slot; this is what makes it true on every path that does not
	// stage, REST included.
	errBookingEndNotAfterStart = &SchedulingArgumentError{
		Field: "end", Code: "invalid_date_range",
		Message: "`end` must be later than `start`",
	}
	// `links` carries minItems: 1 in crm.yaml, and nothing generated enforces
	// it: the decoder fills the slice and asks no question about its length. So
	// this is where the contract's bound becomes true, on every door — REST,
	// the public page, and the tool seam's post-approval execute.
	//
	// The bound is not bookkeeping. A meeting linked to nothing lands on no
	// record's timeline, which means nobody encounters it again by looking at
	// the account, the contact or the deal it was about; the only way back to
	// it is to already know it exists. The account send refuses an unlinked
	// message for the same reason and in the same words.
	errBookingLinksEmpty = &SchedulingArgumentError{
		Field: "links", Code: codeRequired,
		Message: "`links` needs at least one entry: name the contact, company, deal, lead or " +
			"project the meeting is about — one attached to nothing appears on no timeline",
	}
)

type slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Availability computes free slots for one host inside the window:
// business-hour candidates minus the host's existing meetings. A
// non-positive duration means the caller named none and takes the
// default.
//
// truncated reports that at least one more free slot exists after the last one
// returned. It is returned rather than left implicit because every caller puts
// these slots on a wire, and a capped list presented as the whole truth is how a
// model comes to tell someone there is no later opening. To reach the rest, ask
// again with `from` after the last slot returned.
func (s *Store) Availability(ctx context.Context, host ids.UserID, from, to time.Time, duration time.Duration) (free []slot, truncated bool, err error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, false, err
	}
	if duration <= 0 {
		duration = defaultSlotDuration
	}
	if !to.After(from) {
		return nil, false, errAvailabilityToNotAfterFrom
	}
	if to.Sub(from) > maxAvailabilityWindow {
		return nil, false, errAvailabilityWindowTooWide
	}
	if duration < minSlotDuration || duration > maxSlotDuration {
		return nil, false, errAvailabilityDurationOutOfRange
	}

	// The busy read is a read of the host's meetings and carries the
	// activity row scope: a caller sees only the busy blocks whose
	// linked records their timeline would show. A hidden meeting can
	// still surface as slot_taken at booking time — that reveals one
	// bit at one attempted slot, not another rep's calendar.
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	hostPos := arg(host)
	fromPos := arg(from.Add(-assumedMeetingDuration))
	toPos := arg(to)
	scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, false, err
	}
	if scope == "" {
		scope = "TRUE"
	}

	var busy []slot
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT a.occurred_at FROM activity a
			WHERE a.kind = 'meeting' AND a.archived_at IS NULL
			  AND a.host_user_id = $%d
			  AND a.occurred_at BETWEEN $%d AND $%d
			  AND %s
			ORDER BY a.occurred_at`, hostPos, fromPos, toPos, scope), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var start time.Time
			if err := rows.Scan(&start); err != nil {
				return err
			}
			busy = append(busy, slot{Start: start, End: start.Add(assumedMeetingDuration)})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}

	slots, truncated := freeSlots(from, to, duration, busy, s.hoursOf(ctx, host))
	return slots, truncated, nil
}

// hoursOf answers when this host is bookable, degrading to the fallback rather
// than refusing.
//
// A resolver error is the same case as no resolver at all: the public booking
// page reaches this path, and a customer told "could not read the host's
// settings" cannot act on it. The refusal that matters — the host being
// unbookable at the time they pick — is enforced at booking, not here.
func (s *Store) hoursOf(ctx context.Context, host ids.UserID) WorkingHours {
	if s.workingHours == nil {
		return fallbackWorkingHours()
	}
	hours, err := s.workingHours(ctx, host)
	if err != nil || hours.Location == nil || len(hours.Days) == 0 {
		return fallbackWorkingHours()
	}
	return hours
}

// freeSlots walks the duration-aligned candidate grid inside the window
// and keeps business-hour slots that miss every busy block. Candidates
// align to the duration grid, never before the caller's window, and
// must END inside business hours (17:00 sharp is fine, 17:01 is not).
//
// truncated reports that a further free slot EXISTS beyond the ones returned —
// established by finding it, not inferred from hitting the cap. A window holding
// exactly maxProposedSlots free slots withheld nothing, and saying otherwise
// would send a caller looking for slots that are not there.
func freeSlots(from, to time.Time, duration time.Duration, busy []slot, hours WorkingHours) (free []slot, truncated bool) {
	// Empty, never nil: "the host is booked solid" is a real answer and arrives
	// shaped like the array the contract declares. Normalized here, once, so no
	// transport can put `null` on the wire — which a model reads as "unknown"
	// rather than "none free".
	free = []slot{}
	cursor := from.UTC().Truncate(duration)
	if cursor.Before(from.UTC()) {
		cursor = cursor.Add(duration)
	}
	for ; !cursor.Add(duration).After(to.UTC()); cursor = cursor.Add(duration) {
		if !hours.covers(cursor, cursor.Add(duration)) {
			continue
		}
		candidate := slot{Start: cursor, End: cursor.Add(duration)}
		if overlapsAny(candidate, busy) {
			continue
		}
		// One past the cap, then trimmed: the extra candidate is the evidence
		// that something was actually withheld. Stopping AT the cap could not
		// tell "twenty and no more" from "twenty of many", and only the second
		// is a truncated answer.
		free = append(free, candidate)
		if len(free) > maxProposedSlots {
			return free[:maxProposedSlots], true
		}
	}
	return free, false
}

func overlapsAny(candidate slot, busy []slot) bool {
	for _, b := range busy {
		if candidate.Start.Before(b.End) && b.Start.Before(candidate.End) {
			return true
		}
	}
	return false
}

func (h Handlers) GetAvailability(w http.ResponseWriter, r *http.Request, params crmcontracts.GetAvailabilityParams) {
	actor, ok := principal.Actor(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "availability needs an authenticated caller")
		return
	}
	host := ids.From[ids.UserKind](actor.UserID)
	if params.HostUserId != nil {
		host = ids.From[ids.UserKind](ids.UUID(*params.HostUserId))
	}
	var duration time.Duration
	if params.DurationMinutes != nil {
		duration = time.Duration(*params.DurationMinutes) * time.Minute
	}
	slots, truncated, err := h.store.Availability(r.Context(), host, params.From, params.To, duration)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	backing, err := h.calendars.BackingFor(r.Context(), actor.UserID, host)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// truncated and calendar_backing BOTH travel on both transports, because
	// the cap is the store's and so is the obligation to admit it, and because
	// a window says different things depending on what it was read from
	// (ADR-0055: the two surfaces do not get to disagree about what an answer
	// means). Without the backing an empty grid reads as an empty diary.
	httperr.WriteJSON(w, http.StatusOK, map[string]any{
		"slots": slots, "truncated": truncated, "calendar_backing": backing,
	})
}

// WithWorkingHours binds the resolver on the transport too, so the REST and
// public booking paths read the same hours the seam does.
func (h Handlers) WithWorkingHours(resolve WorkingHoursResolver) Handlers {
	h.store = h.store.WithWorkingHours(resolve)
	return h
}

// CalendarConnected answers whether a host's own diary reaches this product.
//
// compose owns it: the connections live in capture, which this module may not
// import. Nil answers NO rather than unknown, matching the seam it is wired
// from — an answer that claimed a diary nobody established it had read is the
// failure this whole field exists to prevent, and an unwired deployment must
// not be able to make it.
type CalendarConnected func(ctx context.Context, host ids.UserID) (bool, error)

// BackingFor is what the answer rests on, in the vocabulary both doors publish.
//
// Somebody else's host is answered Unknown WITHOUT consulting the seam, so the
// reply costs the same whatever that contact has connected — a version that
// read first and withheld afterwards would still be a timing signal.
func (connected CalendarConnected) BackingFor(
	ctx context.Context, actor ids.UUID, host ids.UserID,
) (string, error) {
	if actor.IsZero() || actor != host.UUID {
		return calendarbacking.Unknown, nil
	}
	if connected == nil {
		return calendarbacking.Unbacked, nil
	}
	live, err := connected(ctx, host)
	if err != nil {
		return "", err
	}
	if live {
		return calendarbacking.Backed, nil
	}
	return calendarbacking.Unbacked, nil
}
