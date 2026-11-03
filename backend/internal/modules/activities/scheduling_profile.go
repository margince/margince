// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SchedulingCalendar is supplied by compose; activities never reads credentials.
type SchedulingCalendar interface {
	Check(context.Context, ids.UserID, string) error
	List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error)
	CheckRecipient(context.Context, ids.UserID, ids.UUID, string) error
	// ResolveAttendees says what each address is to the host: a contact they
	// can see, a live colleague, a lead, or host-named.
	ResolveAttendees(context.Context, ids.UserID, []string) ([]connector.CalendarAttendee, error)
	// InviteeAddress reads a linked contact's or lead's address under the
	// host's row scope, and the contact a promoted lead became.
	InviteeAddress(context.Context, ids.UserID, string, ids.UUID, []string) (string, ids.UUID, error)
	// Blocker says why the host's connected calendar cannot take bookings,
	// without a provider call.
	Blocker(context.Context, ids.UserID, string) (crmcontracts.SchedulingProfileBlocker, error)
	Busy(context.Context, ids.UserID, string, string, time.Time, time.Time) ([]connector.CalendarInterval, error)
	Inspect(context.Context, ids.UserID, string, string, string) (connector.CalendarState, error)
	Lookup(context.Context, ids.UserID, string, connector.CalendarAppointment) (*connector.CalendarReceipt, error)
	Save(context.Context, ids.UserID, string, connector.CalendarAppointment) (connector.CalendarReceipt, error)
	Cancel(context.Context, ids.UserID, string, string, string) error
}

// WithSchedulingCalendar binds provider operations without coupling domain modules.
func (s *Store) WithSchedulingCalendar(calendar SchedulingCalendar) *Store {
	clone := *s
	clone.calendar = calendar
	return &clone
}

// WithSchedulingCalendar binds provider operations without coupling domain modules.
func (h Handlers) WithSchedulingCalendar(calendar SchedulingCalendar) Handlers {
	h.store = h.store.WithSchedulingCalendar(calendar)
	return h
}

func schedulingHost(ctx context.Context) (ids.UserID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return ids.UserID{}, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return ids.UserID{}, err
	}
	return ids.From[ids.UserKind](actor.UserID), nil
}

func defaultSchedulingProfile() crmcontracts.SchedulingProfile {
	return crmcontracts.SchedulingProfile{
		CalendarId: "primary", DurationMinutes: 30,
		NoticeMinutes: 120, HorizonDays: 30, BufferMinutes: 10, Title: "Let's meet",
	}
}

// videoCallFor decides whether a new meeting asks the calendar for a video
// link: the request's own choice when it made one, else the host's setting,
// which a profile saved before the setting existed does not carry.
func videoCallFor(profile crmcontracts.SchedulingProfile, requested *bool) bool {
	if requested != nil {
		return *requested
	}
	return profile.VideoCall == nil || *profile.VideoCall
}

func (s *Store) hostSchedulingProfile(ctx context.Context, host ids.UserID) (crmcontracts.SchedulingProfile, error) {
	profile := defaultSchedulingProfile()
	var name string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{host}
		var err error
		name, err = schedulingAccountName(ctx, tx, host)
		if err != nil {
			return err
		}
		var stored *crmcontracts.SchedulingProfile
		var slug string
		err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT slug, scheduling_policy FROM booking_page
   WHERE host_user_id = $%d AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, len(args)), args...).Scan(&slug, &stored)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if stored != nil {
			profile = *stored
		}
		profile.Slug = &slug
		profile.Alias = nil
		alias, err := liveBookingPageAlias(ctx, tx, host)
		if alias != "" {
			profile.Alias = &alias
		}
		return err
	})
	profile.HostName = &name

	if err == nil && profile.Slug != nil {
		address := *profile.Slug
		if profile.Alias != nil {
			address = *profile.Alias
		}
		if link, ok := s.publicBookingURL(address); ok {
			profile.PublicUrl = &link
		}
	}
	return profile, err
}

// SchedulingProfile returns the acting host’s reusable booking settings.
func (s *Store) SchedulingProfile(ctx context.Context) (crmcontracts.SchedulingProfile, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.SchedulingProfile{}, err
	}
	profile, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return profile, err
	}
	blocker, err := s.calendarBlocker(ctx, host, profile)
	profile.Blocker = &blocker
	return s.brandSchedulingProfile(ctx, host, profile), err
}

// pagePaused is the one answer to whether a standing booking page takes
// bookings: the host switched it on, and its calendar can take them. The
// guest profile, the free-time read and both booking reads ask it.
func (s *Store) pagePaused(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile) (bool, error) {
	if !profile.Enabled {
		return true, nil
	}
	blocker, err := s.calendarBlocker(ctx, host, profile)
	return blocker != crmcontracts.SchedulingProfileBlockerNone, err
}

// calendarBlocker is why bookings cannot reach the host's calendar. A page that
// names no calendar, which is every page saved before the profile existed, is
// blocked without asking the seam.
func (s *Store) calendarBlocker(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile) (crmcontracts.SchedulingProfileBlocker, error) {
	switch {
	case profile.Provider == "":
		return crmcontracts.SchedulingProfileBlockerNoCalendar, nil
	case s.calendar == nil:
		return crmcontracts.SchedulingProfileBlockerCalendarUnavailable, nil
	}
	return s.calendar.Blocker(ctx, host, string(profile.Provider))
}

// calendarNotConnected is the refusal a host can act on when their calendar
// refused us; calendarAuthFailure recognises it by value.
var calendarNotConnected = SchedulingArgumentError{Field: "provider", Code: "required", Message: "Connect a calendar first"}

func errCalendarNotConnected() error {
	refusal := calendarNotConnected
	return &refusal
}

// unconnectedAsRefusal turns the provider seam's "no usable connection" into
// the request the host can act on; every other error is not ours to rename.
func unconnectedAsRefusal(err error) error {
	if errors.Is(err, connector.ErrAuthRejected) {
		return errCalendarNotConnected()
	}
	return err
}

// checkCalendarConnected asks the provider seam whether the host's calendar
// answers. A host with no connection, or one the provider no longer honours,
// is told to connect one: a request of theirs, never a server fault.
func (s *Store) checkCalendarConnected(ctx context.Context, host ids.UserID, provider string) error {
	return unconnectedAsRefusal(s.calendar.Check(ctx, host, provider))
}

func validateSchedulingProfile(p crmcontracts.SchedulingProfile) error {
	if err := validateSchedulingLimits(p); err != nil {
		return err
	}
	if !p.Provider.Valid() {
		return &SchedulingArgumentError{Field: "provider", Code: "invalid", Message: "Choose a supported calendar provider"}
	}
	if p.Enabled && (p.HostName == nil || strings.TrimSpace(*p.HostName) == "") {
		return &SchedulingArgumentError{Field: "host_name", Code: "required", Message: "Set your name in Settings → Account before enabling bookings"}
	}
	if p.Enabled && p.Provider != "gcal" && p.Provider != "graphcal" {
		return errCalendarNotConnected()
	}
	if p.BlockingCalendars != nil && len(*p.BlockingCalendars) > 10 {
		return &SchedulingArgumentError{Field: "blocking_calendars", Code: "too_many", Message: "Choose at most ten calendars"}
	}
	if p.BlockingCalendars != nil && slices.ContainsFunc(*p.BlockingCalendars, func(id string) bool { return !values.HasVisibleText(id) }) {
		return &SchedulingArgumentError{Field: "blocking_calendars", Code: "invalid", Message: "Remove the empty calendar entry"}
	}
	if p.BlockingCalendars != nil && slices.ContainsFunc(*p.BlockingCalendars, func(id string) bool { return utf8.RuneCountInString(id) > maxSchedulingCalendar }) {
		return &SchedulingArgumentError{
			Field: "blocking_calendars", Code: codeTooLong,
			Message: fmt.Sprintf("A calendar id may be at most %d characters", maxSchedulingCalendar),
		}
	}
	return nil
}

// SaveSchedulingProfile validates live calendar authority before publishing a booking link.
func (s *Store) SaveSchedulingProfile(ctx context.Context, profile crmcontracts.SchedulingProfile) (crmcontracts.SchedulingProfile, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return profile, err
	}
	stored, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return profile, err
	}
	profile.HostName = stored.HostName
	profile.CompanyName, profile.LogoUrl = nil, nil
	if err := validateSchedulingProfile(profile); err != nil {
		return profile, err
	}
	if profile.Enabled {
		if err := s.publicOriginUsable(); err != nil {
			return profile, err
		}
		if err := s.checkBookingCalendar(ctx, host, stored, &profile); err != nil {
			return profile, err
		}
	}
	err = s.persistSchedulingProfile(ctx, host, profile)
	if err != nil {
		return profile, err
	}
	return s.SchedulingProfile(ctx)
}

// checkBookingCalendar asks the provider only when a live page gains a calendar
// it has not been checked against: switched on, or pointed somewhere new. A
// lapsed grant otherwise blocks a notice or buffer edit the calendar never sees.
func (s *Store) checkBookingCalendar(ctx context.Context, host ids.UserID, stored crmcontracts.SchedulingProfile, profile *crmcontracts.SchedulingProfile) error {
	if stored.Enabled && !bookingCalendarChanged(stored, *profile) {
		return nil
	}
	if s.calendar == nil {
		return apperrors.ErrPermissionDenied
	}
	if err := s.checkCalendarConnected(ctx, host, string(profile.Provider)); err != nil {
		return err
	}
	return s.validateBookingCalendars(ctx, host, profile)
}

// bookingCalendarChanged compares what the provider check vouches for. An
// absent blocking list and an empty one block the same nothing.
func bookingCalendarChanged(stored, next crmcontracts.SchedulingProfile) bool {
	return stored.Provider != next.Provider || stored.CalendarId != next.CalendarId ||
		!slices.Equal(blockingCalendars(stored), blockingCalendars(next))
}

func blockingCalendars(p crmcontracts.SchedulingProfile) []string {
	if p.BlockingCalendars == nil {
		return nil
	}
	return *p.BlockingCalendars
}

func (s *Store) validateBookingCalendars(ctx context.Context, host ids.UserID, profile *crmcontracts.SchedulingProfile) error {
	if s.calendar == nil {
		return apperrors.ErrPermissionDenied
	}
	calendars, err := s.calendar.List(ctx, host, string(profile.Provider))
	if err != nil {
		return unconnectedAsRefusal(err)
	}
	available := map[string]bool{}
	writable := false
	for _, calendar := range calendars {
		available[calendar.ID] = true
		if profile.CalendarId == "primary" && calendar.Primary {
			profile.CalendarId = calendar.ID
		}
		if calendar.ID == profile.CalendarId {
			writable = calendar.Writable
		}
	}
	if !writable {
		return &SchedulingArgumentError{Field: "calendar_id", Code: "not_writable", Message: "Choose a calendar you can create invitations in"}
	}
	if profile.BlockingCalendars != nil {
		for _, id := range *profile.BlockingCalendars {
			if !available[id] {
				return &SchedulingArgumentError{Field: "blocking_calendars", Code: "unavailable", Message: "Choose calendars that are available on this connection"}
			}
		}
	}
	return nil
}

// schedulingRanges are the whole-number settings and the closed range the
// contract gives each, walked in the contract's order.
var schedulingRanges = []struct {
	field, label, unit string
	lo, hi             int
	value              func(crmcontracts.SchedulingProfile) int
}{
	{"duration_minutes", "The meeting length", unitMinutes, 15, 480, func(p crmcontracts.SchedulingProfile) int { return p.DurationMinutes }},
	{"notice_minutes", "The minimum notice", unitMinutes, 0, 10080, func(p crmcontracts.SchedulingProfile) int { return p.NoticeMinutes }},
	{"horizon_days", "How far ahead guests can book", "days", 1, 90, func(p crmcontracts.SchedulingProfile) int { return p.HorizonDays }},
	{"buffer_minutes", "The buffer between meetings", unitMinutes, 0, 120, func(p crmcontracts.SchedulingProfile) int { return p.BufferMinutes }},
}

const (
	codeTooLong           = "too_long"
	codeOutOfRange        = "out_of_range"
	unitMinutes           = "minutes"
	maxSchedulingTitle    = 200
	maxSchedulingLocation = 1000
	maxSchedulingCalendar = 512
)

// validateSchedulingLimits refuses the first setting outside its limit, naming
// that field and its limit, so the host is told which setting to correct.
func validateSchedulingLimits(p crmcontracts.SchedulingProfile) error {
	for _, r := range schedulingRanges {
		if v := r.value(p); v < r.lo || v > r.hi {
			return &SchedulingArgumentError{
				Field: r.field, Code: codeOutOfRange,
				Message: fmt.Sprintf("%s must be between %d and %d %s", r.label, r.lo, r.hi, r.unit),
			}
		}
	}
	if !values.HasVisibleText(p.Title) {
		return &SchedulingArgumentError{Field: "title", Code: "required", Message: "Give the meeting a title guests can read"}
	}
	if utf8.RuneCountInString(p.Title) > maxSchedulingTitle {
		return &SchedulingArgumentError{
			Field: "title", Code: codeTooLong,
			Message: fmt.Sprintf("The title may be at most %d characters", maxSchedulingTitle),
		}
	}
	if utf8.RuneCountInString(p.Location) > maxSchedulingLocation {
		return &SchedulingArgumentError{
			Field: "location", Code: codeTooLong,
			Message: fmt.Sprintf("The location may be at most %d characters", maxSchedulingLocation),
		}
	}
	if p.CalendarId == "" {
		return &SchedulingArgumentError{Field: "calendar_id", Code: "required", Message: "Choose the calendar meetings are created in"}
	}
	if utf8.RuneCountInString(p.CalendarId) > maxSchedulingCalendar {
		return &SchedulingArgumentError{
			Field: "calendar_id", Code: codeTooLong,
			Message: fmt.Sprintf("The calendar id may be at most %d characters", maxSchedulingCalendar),
		}
	}
	return nil
}

func schedulingAccountName(ctx context.Context, tx pgx.Tx, host ids.UserID) (string, error) {
	// A host has an Account name before their first booking page exists.
	args := []any{host}
	var name string
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT display_name FROM app_user WHERE id = $%d`, len(args)), args...).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	return schedulingDisplayName(name), err
}

func (s *Store) persistSchedulingProfile(ctx context.Context, host ids.UserID, profile crmcontracts.SchedulingProfile) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
			return err
		}
		var before *crmcontracts.SchedulingProfile
		var id ids.UUID
		args := []any{host}
		err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT id, scheduling_policy FROM booking_page WHERE host_user_id=$%d AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, len(args)), args...).Scan(&id, &before)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if errors.Is(err, pgx.ErrNoRows) || (profile.ReplaceLink != nil && *profile.ReplaceLink) {
			id, err = s.replaceBookingPage(ctx, tx, host)
			if err != nil {
				return err
			}
		}
		profile.Slug, profile.Alias, profile.PublicUrl, profile.ReplaceLink = nil, nil, nil, nil
		profile.HostName, profile.Blocker = nil, nil
		args = []any{profile, id}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE booking_page SET scheduling_policy=$%d WHERE id=$%d`, len(args)-1, len(args)), args...); err != nil {
			return err
		}
		var audit ids.UUID
		if before == nil {
			audit, err = storekit.AuditEvent(ctx, tx, "create", "booking_page", id, profile)
		} else {
			audit, err = storekit.Audit(ctx, tx, "update", "booking_page", id, before, profile)
		}
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.InternalEventBookingPageUpdated{Enabled: profile.Enabled})
	})
}

func (s *Store) replaceBookingPage(ctx context.Context, tx pgx.Tx, host ids.UserID) (ids.UUID, error) {
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE booking_page SET revoked_at=now() WHERE host_user_id=`+args.add(host)+` AND revoked_at IS NULL`, args...); err != nil {
		return ids.Nil, err
	}
	// Replace address stops every public address of the old page, the alias
	// included. The alias is retired, so its cooldown starts now.
	if err := s.retireBookingAliasTx(ctx, tx, host); err != nil {
		return ids.Nil, err
	}
	slug, err := SeedBookingPageTx(ctx, tx, host)
	if err != nil {
		return ids.Nil, err
	}
	args = schedulingArgs{}
	var id ids.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM booking_page WHERE slug=`+args.add(slug), args...).Scan(&id)
	return id, err
}
