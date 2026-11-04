// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether a booking page has been used, and the Home concern that rests on it.
// A page counts as used once somebody booked through it or its host copied its
// link. booking_page.first_used_at records the first time, set once and never
// cleared, so a page handed out at setup and never used stays off Home.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BookingPageConcern is a used, switched-on booking page that takes no bookings.
type BookingPageConcern struct {
	PageID  ids.UUID
	Blocker crmcontracts.SchedulingProfileBlocker
}

// bookingPageUse is the audited value of a page's first use.
type bookingPageUse struct {
	FirstUsedAt *time.Time `json:"first_used_at"`
}

// MarkBookingPageShared records that the acting host copied their booking
// link. Only the first copy writes; a later one changes nothing.
func (s *Store) MarkBookingPageShared(ctx context.Context) error {
	host, err := schedulingHost(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		found, err := stampBookingPageUseTx(ctx, tx, host, s.now())
		if err == nil && !found {
			return apperrors.ErrNotFound
		}
		return err
	})
}

// stampBookingPageUseTx records a page's first use, for a booking and a link
// copy alike. It holds the page's write identity, so a link replaced at the
// same moment cannot carry an unstamped page forward. It answers false when
// the host has no live page.
func stampBookingPageUseTx(ctx context.Context, tx pgx.Tx, host ids.UserID, at time.Time) (bool, error) {
	if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
		return false, err
	}
	args := schedulingArgs{}
	var page ids.UUID
	var used *time.Time
	var policy *crmcontracts.SchedulingProfile
	err := tx.QueryRow(ctx, `SELECT id, first_used_at, scheduling_policy FROM booking_page WHERE host_user_id=`+args.add(host)+
		` AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, args...).Scan(&page, &used, &policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil || used != nil {
		return err == nil, err
	}
	args = schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE booking_page SET first_used_at=`+args.add(at)+` WHERE id=`+args.add(page), args...); err != nil {
		return true, err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "booking_page", page, bookingPageUse{}, bookingPageUse{FirstUsedAt: &at})
	if err != nil {
		return true, err
	}
	return true, storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: policy != nil && policy.Enabled})
}

// carryBookingPageUseTx copies the host's first use onto their new page, so
// replacing a leaked link does not make a used page look unused.
func carryBookingPageUseTx(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID) error {
	args := schedulingArgs{}
	_, err := tx.Exec(ctx, `UPDATE booking_page SET first_used_at=(SELECT min(first_used_at) FROM booking_page WHERE host_user_id=`+
		args.add(host)+`) WHERE id=`+args.add(page), args...)
	return err
}

// BookingPageConcern answers the acting host's paused booking page, and false
// when there is none. Only a switched-on page that was used counts. Home stays
// quiet about a page its host never put to work.
func (s *Store) BookingPageConcern(ctx context.Context) (BookingPageConcern, bool, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return BookingPageConcern{}, false, err
	}
	var page ids.UUID
	var used *time.Time
	err = s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		return tx.QueryRow(ctx, `SELECT id, first_used_at FROM booking_page WHERE host_user_id=`+args.add(host)+
			` AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, args...).Scan(&page, &used)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingPageConcern{}, false, nil
	}
	if err != nil || used == nil {
		return BookingPageConcern{}, false, err
	}
	profile, err := s.hostSchedulingProfile(ctx, host)
	if err != nil || !profile.Enabled {
		return BookingPageConcern{}, false, err
	}
	blocker, err := s.calendarBlocker(ctx, host, profile)
	if err != nil || blocker == crmcontracts.SchedulingProfileBlockerNone {
		return BookingPageConcern{}, false, err
	}
	return BookingPageConcern{PageID: page, Blocker: blocker}, true, nil
}
