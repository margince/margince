// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// When one contact is bookable, on their own clock.
//
// Personal, for the reason docs/explanation/scheduling.md states: contacts on one
// team sit in different countries, some work part time, and one pair of numbers
// set by an admin is authoritatively wrong for most of them while the contacts it
// fails cannot change it. So this sits beside the display language — each
// contact sets their own and nobody sets it for anybody else.
//
// ABSENT MEANS NOBODY HAS CHOSEN, which is what makes the fallback honest: a
// contact who has chosen nothing is bookable 09:00–17:00 Monday to Friday in the
// installation's reporting zone, and that is a stated default rather than a
// leftover constant.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The default a contact who has chosen nothing is bookable in. Named, because it
// is a decision: treating unset as "no constraint" lets a customer book
// somebody at 3am, and requiring a choice breaks booking for everybody until
// they act.
const (
	defaultWorkStartMinute = 9 * 60
	defaultWorkEndMinute   = 17 * 60
	minutesInADay          = 24 * 60
)

// workingHoursFields name what a refusal points at and what the audit images
// carry. One spelling per field, so a 422 names the control the reader used.
const (
	fieldWorkStart    = "start_time"
	fieldWorkEnd      = "end_time"
	fieldWorkDays     = "days"
	fieldWorkTimezone = "timezone"
	// codeInvalid is the wire code every field refusal in this package answers
	// with. One spelling, because a settings form keys its per-control message
	// off it and two spellings would put one control's message somewhere else.
	codeInvalid = "invalid"
)

// WorkingHours is one contact's bookable window, as minutes past local midnight
// on the days they work.
//
// Minutes rather than a time.Time: the pair is compared against a slot's own
// minute of the day, and a time carries a date that would have to be invented
// and then ignored.
type WorkingHours struct {
	StartMinute int
	// EndMinute is exclusive, and may be minutesInADay: "until midnight" is a
	// working day somebody keeps, and 23:59 is not the same answer.
	EndMinute int
	// Days are ISO-8601 weekday numbers, 1 = Monday, ascending and unique.
	Days []int
	// Timezone is the IANA zone the two minutes are read on.
	Timezone string
}

// DefaultWorkingHours is what a contact who has chosen nothing is bookable in.
func DefaultWorkingHours(zone string) WorkingHours {
	return WorkingHours{
		StartMinute: defaultWorkStartMinute,
		EndMinute:   defaultWorkEndMinute,
		Days:        []int{1, 2, 3, 4, 5},
		Timezone:    zone,
	}
}

// Works reports whether the given ISO weekday is one this contact works.
func (h WorkingHours) Works(weekday int) bool {
	for _, day := range h.Days {
		if day == weekday {
			return true
		}
	}
	return false
}

// Location resolves the zone the hours are read on, refusing a name Go's
// database does not carry rather than silently falling back to UTC — which is
// the exact defect this setting exists to remove.
func (h WorkingHours) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(h.Timezone)
	if err != nil {
		return nil, fmt.Errorf("identity: working-hours timezone %q does not resolve: %w", h.Timezone, err)
	}
	return loc, nil
}

// WorkingHoursOf answers one contact's hours and whether they are that contact's
// own, inside a transaction the caller already holds.
//
// The second return is not a formality. A screen offering the fallback as a
// starting point and a screen showing a decision somebody made are different
// screens, and a caller that could not tell them apart would have to guess.
func WorkingHoursOf(ctx context.Context, tx pgx.Tx, user ids.UserID) (WorkingHours, bool, error) {
	// ApplyTx, not the gated read: this answers a rep asking about their own
	// week and a customer opening a public booking link, neither of whom holds
	// installation_settings.read — and the zone is disclosed by every time
	// either of them is shown anyway (see the entry's own note).
	installation, err := settings.ApplyTx(ctx, tx, Timezone)
	if err != nil {
		return WorkingHours{}, false, err
	}
	var start, end *int16
	var days []int16
	var zone *string
	if err := tx.QueryRow(ctx, `
		SELECT work_start_minute, work_end_minute, work_days, timezone
		  FROM app_user WHERE id = $1 AND `+LiveMemberSQL(""),
		user).Scan(&start, &end, &days, &zone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkingHours{}, false, apperrors.ErrNotFound
		}
		return WorkingHours{}, false, fmt.Errorf("identity: reading working hours: %w", err)
	}
	hours := DefaultWorkingHours(installation)
	// The zone is chosen separately from the hours, and either may stand alone:
	// somebody who has only ever told the product where they are still gets the
	// default hours read on their own clock, which is most of the value.
	if zone != nil && *zone != "" {
		hours.Timezone = *zone
	}
	if start == nil || end == nil || len(days) == 0 {
		return hours, false, nil
	}
	hours.StartMinute, hours.EndMinute = int(*start), int(*end)
	hours.Days = make([]int, 0, len(days))
	for _, day := range days {
		hours.Days = append(hours.Days, int(day))
	}
	return hours, true, nil
}

// MyWorkingHours answers the CALLER's own.
func (s *Service) MyWorkingHours(ctx context.Context) (WorkingHours, bool, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return WorkingHours{}, false, err
	}
	var hours WorkingHours
	var chosen bool
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		hours, chosen, err = WorkingHoursOf(ctx, tx, ids.From[ids.UserKind](human))
		return err
	})
	if err != nil {
		return WorkingHours{}, false, fmt.Errorf("identity: reading the caller's working hours: %w", err)
	}
	return hours, chosen, nil
}

// SaveMyWorkingHours records the CALLER's own hours, days and zone.
//
// Self-scoped from the authenticated principal, never from a body field: an id
// on the request would be an admin's way to set a colleague's working hours,
// which is the shape this setting exists to refuse.
func (s *Service) SaveMyWorkingHours(ctx context.Context, in WorkingHours) (WorkingHours, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return WorkingHours{}, err
	}
	normalized, err := validWorkingHours(in)
	if err != nil {
		return WorkingHours{}, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, _, err := WorkingHoursOf(ctx, tx, ids.From[ids.UserKind](human))
		if err != nil {
			return err
		}
		days := weekdayColumn(normalized.Days)
		tag, err := tx.Exec(ctx, `
			UPDATE app_user
			   SET work_start_minute = $2, work_end_minute = $3, work_days = $4, timezone = $5
			 WHERE id = $1 AND `+LiveMemberSQL(""),
			human, minuteColumn(normalized.StartMinute), minuteColumn(normalized.EndMinute),
			days, normalized.Timezone)
		if err != nil {
			return fmt.Errorf("identity: saving working hours: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		// Audit-only: the closed event catalog carries no working-hours verb,
		// and there is nothing downstream waiting on one — availability reads
		// the row on the next question, which no subscriber could do sooner.
		// The images carry both windows, which is what answers "why did my
		// bookings halve".
		if _, err := storekit.Audit(ctx, tx, "update", "user", human,
			workingHoursImage(before), workingHoursImage(normalized)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return WorkingHours{}, err
	}
	return normalized, nil
}

// selfScoped is the caller, refusing anybody who is not a human seat.
//
// Deliberately NOT actingHuman: that resolves the human an agent is acting
// under, which is right for attributing work and wrong here. An agent carrying
// its grantor's authority must not change when its grantor is bookable.
func selfScoped(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

// workingHoursImage renders one side of the audit's before/after pair.
func workingHoursImage(hours WorkingHours) map[string]any {
	return map[string]any{
		fieldWorkStart:    minuteOfDay(hours.StartMinute),
		fieldWorkEnd:      minuteOfDay(hours.EndMinute),
		fieldWorkDays:     hours.Days,
		fieldWorkTimezone: hours.Timezone,
	}
}

// minuteOfDay renders a minute past midnight as the HH:MM the wire and the
// audit row both carry — the reader's own spelling, not an integer they would
// have to divide.
func minuteOfDay(minute int) string {
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

// validWorkingHours refuses what no calendar can answer and normalizes the rest.
func validWorkingHours(in WorkingHours) (WorkingHours, error) {
	switch {
	case in.StartMinute < 0 || in.StartMinute >= minutesInADay:
		return WorkingHours{}, &WorkingHoursError{
			Field:   fieldWorkStart,
			Message: "a start time is a moment in the day it starts",
		}
	case in.EndMinute <= 0 || in.EndMinute > minutesInADay:
		return WorkingHours{}, &WorkingHoursError{
			Field:   fieldWorkEnd,
			Message: "an end time is a moment in the day it ends, up to 24:00",
		}
	case in.EndMinute <= in.StartMinute:
		return WorkingHours{}, &WorkingHoursError{
			Field:   fieldWorkEnd,
			Message: "the working day must end after it starts",
		}
	case len(in.Days) == 0:
		return WorkingHours{}, &WorkingHoursError{
			Field:   fieldWorkDays,
			Message: "choose at least one day you work; nobody is bookable on none",
		}
	}
	seen := map[int]bool{}
	days := make([]int, 0, len(in.Days))
	for _, day := range in.Days {
		if day < 1 || day > 7 {
			return WorkingHours{}, &WorkingHoursError{
				Field:   fieldWorkDays,
				Message: "a working day is 1 (Monday) through 7 (Sunday)",
			}
		}
		if seen[day] {
			continue
		}
		seen[day] = true
		days = append(days, day)
	}
	sort.Ints(days)
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		return WorkingHours{}, &WorkingHoursError{
			Field:   fieldWorkTimezone,
			Message: "a timezone is an IANA name, such as Europe/Berlin",
		}
	}
	return WorkingHours{
		StartMinute: in.StartMinute, EndMinute: in.EndMinute,
		Days: days, Timezone: in.Timezone,
	}, nil
}

// WorkingHoursError names the control a refusal is about, so a settings form
// puts the message against the field the reader used rather than at the bottom
// of the page.
type WorkingHoursError struct {
	Field   string
	Message string
}

func (e *WorkingHoursError) Error() string { return e.Field + ": " + e.Message }

// FieldFault carries it to a 422 naming the field on every surface.
func (e *WorkingHoursError) FieldFault() (field, code, message string) {
	return e.Field, codeInvalid, e.Message
}

// minuteColumn narrows a minute of the day to the column's width.
//
// The narrowing is safe by the validation above — every value here is between
// 0 and 1440 — and it is written as a function rather than a cast at the call
// site so the bound and the conversion sit together instead of a reader having
// to go and find the guard.
func minuteColumn(minute int) int16 {
	if minute < 0 || minute > minutesInADay {
		return 0
	}
	return int16(minute)
}

// weekdayColumn narrows the working days the same way, bounded by the same
// validation: every value is 1 through 7.
func weekdayColumn(days []int) []int16 {
	out := make([]int16, 0, len(days))
	for _, day := range days {
		if day < 1 || day > 7 {
			continue
		}
		out = append(out, int16(day))
	}
	return out
}
