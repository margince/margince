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
		found, err := stampBookingPageUseTx(ctx, tx, host, ids.Nil, s.now())
		if err == nil && !found {
			return apperrors.ErrNotFound
		}
		return err
	})
}

// stampBookingPageUseTx records the first use of the host's page, their
// personal page when page is Nil, for a booking and a link copy alike. It
// holds the host's write identity, so a link replaced at the same moment
// cannot carry an unstamped page forward. It answers false when that page is
// not live.
func stampBookingPageUseTx(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID, at time.Time) (bool, error) {
	if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
		return false, err
	}
	args := schedulingArgs{}
	var used *time.Time
	var policy *crmcontracts.SchedulingProfile
	err := tx.QueryRow(ctx, `SELECT id, first_used_at, scheduling_policy FROM booking_page WHERE `+livePageSQL(&args, host, page)+` FOR UPDATE`, args...).Scan(&page, &used, &policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil || used != nil {
		return err == nil, err
	}
	return true, writeBookingPageUseTx(ctx, tx, page, at, policy != nil && policy.Enabled)
}

// writeBookingPageUseTx sets a page's first use, with its audit row and event.
func writeBookingPageUseTx(ctx context.Context, tx pgx.Tx, page ids.UUID, at time.Time, enabled bool) error {
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE booking_page SET first_used_at=`+args.add(at)+` WHERE id=`+args.add(page), args...); err != nil {
		return err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "booking_page", page, bookingPageUse{}, bookingPageUse{FirstUsedAt: &at})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: enabled})
}

// carryBookingPageUseTx copies the first use of the host's personal page onto
// their new one. Replacing a leaked link then does not make a used page look unused.
func carryBookingPageUseTx(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID, enabled bool) error {
	args := schedulingArgs{}
	var used *time.Time
	if err := tx.QueryRow(ctx, `SELECT min(first_used_at) FROM booking_page WHERE NOT team_link AND host_user_id=`+args.add(host), args...).Scan(&used); err != nil || used == nil {
		return err
	}
	return writeBookingPageUseTx(ctx, tx, page, *used, enabled)
}

// BookingPageConcerns answers each of the acting host's paused booking pages,
// their personal page first and then their team links. Only a switched-on page
// that was used counts. Home stays quiet about a page its host never put to work.
func (s *Store) BookingPageConcerns(ctx context.Context) ([]BookingPageConcern, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return nil, err
	}
	var pages []BookingPage
	err = s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		rows, err := tx.Query(ctx, `SELECT host_user_id, id, team_link FROM booking_page WHERE host_user_id=`+args.add(host)+
			` AND revoked_at IS NULL AND first_used_at IS NOT NULL ORDER BY team_link, created_at, id`, args...)
		if err != nil {
			return err
		}
		pages, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BookingPage])
		return err
	})
	if err != nil {
		return nil, err
	}
	var out []BookingPageConcern
	// Every page of a host books into the one calendar, so one card names a
	// calendar fault. Only a team that is not ready is each link's own.
	named := map[crmcontracts.SchedulingProfileBlocker]bool{crmcontracts.SchedulingProfileBlockerNone: true}
	for _, page := range pages {
		profile, err := s.bookingPageProfile(ctx, page)
		if err != nil {
			return nil, err
		}
		if !profile.Enabled {
			continue
		}
		blocker, err := s.pageBlocker(ctx, host, profile)
		if err != nil {
			return nil, err
		}
		if named[blocker] {
			continue
		}
		named[blocker] = blocker != crmcontracts.SchedulingProfileBlockerTeamNotReady
		out = append(out, BookingPageConcern{PageID: page.ID, Blocker: blocker})
	}
	return out, nil
}
