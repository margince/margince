// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SchedulingCalendar is supplied by compose; activities never reads credentials.
type SchedulingCalendar interface {
	Check(context.Context, ids.UserID, string) error
	List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error)
	CheckRecipient(context.Context, ids.UserID, ids.UUID, string) error
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

func (s *Store) hostSchedulingProfile(ctx context.Context, host ids.UserID) (crmcontracts.SchedulingProfile, error) {
	profile := defaultSchedulingProfile()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{host}
		var stored *crmcontracts.SchedulingProfile
		var slug string
		err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT slug, scheduling_policy FROM booking_page
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
		return nil
	})
	if err == nil && profile.Slug != nil && s.publicOriginUsable() == nil {
		link := strings.TrimRight(s.publicBaseURL, "/") + "/#/book/" + url.PathEscape(*profile.Slug)
		profile.PublicUrl = &link
	}
	return profile, err
}

// SchedulingProfile returns the acting host’s reusable booking settings.
func (s *Store) SchedulingProfile(ctx context.Context) (crmcontracts.SchedulingProfile, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.SchedulingProfile{}, err
	}
	return s.hostSchedulingProfile(ctx, host)
}

func validateSchedulingProfile(p crmcontracts.SchedulingProfile) error {
	if err := validateSchedulingLimits(p); err != nil {
		return err
	}
	if p.Enabled && (p.HostName == nil || strings.TrimSpace(*p.HostName) == "") {
		return errBookingProfileBrand
	}
	if p.Enabled && p.Provider != "gcal" && p.Provider != "graphcal" {
		return &SchedulingArgumentError{Field: "provider", Code: "required", Message: "Connect a calendar before enabling bookings"}
	}
	if p.BlockingCalendars != nil && len(*p.BlockingCalendars) > 10 {
		return &SchedulingArgumentError{Field: "blocking_calendars", Code: "too_many", Message: "Choose at most ten calendars"}
	}
	return validateSchedulingBrand(p)
}

var errBookingProfileBrand = &SchedulingArgumentError{Field: "profile", Code: "invalid_brand", Message: "Use a short display name and an HTTPS company-logo address"}

// SaveSchedulingProfile validates live calendar authority before publishing a booking link.
func (s *Store) SaveSchedulingProfile(ctx context.Context, profile crmcontracts.SchedulingProfile) (crmcontracts.SchedulingProfile, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return profile, err
	}
	if err := validateSchedulingProfile(profile); err != nil {
		return profile, err
	}
	if profile.Enabled {
		if err := s.publicOriginUsable(); err != nil {
			return profile, err
		}
		if s.calendar == nil {
			return profile, apperrors.ErrPermissionDenied
		}
		if err := s.calendar.Check(ctx, host, string(profile.Provider)); err != nil {
			return profile, err
		}
		if err := s.validateBookingCalendars(ctx, host, &profile); err != nil {
			return profile, err
		}
	}
	err = s.persistSchedulingProfile(ctx, host, profile)
	if err != nil {
		return profile, err
	}
	return s.hostSchedulingProfile(ctx, host)
}

func (s *Store) validateBookingCalendars(ctx context.Context, host ids.UserID, profile *crmcontracts.SchedulingProfile) error {
	if s.calendar == nil {
		return apperrors.ErrPermissionDenied
	}
	calendars, err := s.calendar.List(ctx, host, string(profile.Provider))
	if err != nil {
		return err
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

func validateSchedulingLimits(p crmcontracts.SchedulingProfile) error {
	if p.DurationMinutes < 15 || p.DurationMinutes > 480 || p.NoticeMinutes < 0 || p.NoticeMinutes > 10080 ||
		p.HorizonDays < 1 || p.HorizonDays > 90 || p.BufferMinutes < 0 || p.BufferMinutes > 120 ||
		len(p.Title) > 200 || len(p.Location) > 1000 || strings.TrimSpace(p.Title) == "" || p.CalendarId == "" {
		return &SchedulingArgumentError{Field: "profile", Code: faultInvalid, Message: "Choose valid meeting details and availability limits"}
	}
	return nil
}

func validateSchedulingBrand(p crmcontracts.SchedulingProfile) error {
	if p.HostName != nil && len(*p.HostName) > 200 || p.CompanyName != nil && len(*p.CompanyName) > 200 {
		return errBookingProfileBrand
	}
	if p.LogoUrl != nil && *p.LogoUrl != "" {
		u, err := url.Parse(*p.LogoUrl)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(*p.LogoUrl) > 2000 {
			return errBookingProfileBrand
		}
	}
	return nil
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
			id, err = replaceBookingPage(ctx, tx, host)
			if err != nil {
				return err
			}
		}
		profile.Slug, profile.PublicUrl, profile.ReplaceLink = nil, nil, nil
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

func replaceBookingPage(ctx context.Context, tx pgx.Tx, host ids.UserID) (ids.UUID, error) {
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE booking_page SET revoked_at=now() WHERE host_user_id=`+args.add(host)+` AND revoked_at IS NULL`, args...); err != nil {
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
