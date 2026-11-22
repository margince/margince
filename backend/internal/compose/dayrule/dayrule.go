// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dayrule answers "which day is it for this reader on this screen".
//
// A person's own lists read the day in that person's zone. A screen that
// combines several people reads it in the installation's zone. Every reader of
// a local day goes through here, so one person never gets two answers.
package dayrule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Zone is the zone the surface reads the day in. user is the person whose list
// it is, and Combined ignores it. A person with no zone of their own, or no
// seat any more, reads the installation's zone.
func Zone(ctx context.Context, tx pgx.Tx, surface occurredday.Surface, user ids.UUID) (*time.Location, error) {
	if surface == occurredday.Personal && !user.IsZero() {
		hours, _, err := identity.WorkingHoursOf(ctx, tx, ids.From[ids.UserKind](user))
		switch {
		case err == nil:
			return hours.Location()
		case !errors.Is(err, apperrors.ErrNotFound):
			return nil, fmt.Errorf("dayrule: reading the person's zone: %w", err)
		}
	}
	return identity.ZoneAppliedTx(ctx, tx)
}

// DayAt is the calendar date at now on the surface, at UTC midnight like a
// scanned date column. It also returns the wall clock in that zone.
func DayAt(
	ctx context.Context, tx pgx.Tx, surface occurredday.Surface, user ids.UUID, now time.Time,
) (day time.Time, local time.Time, err error) {
	loc, err := Zone(ctx, tx, surface, user)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return storekit.WorkspaceDay(now, loc), now.In(loc), nil
}

// ActingReaderZone resolves the acting person's own zone for their own list,
// with the installation's zone as fallback.
func ActingReaderZone(pool *pgxpool.Pool) func(context.Context) (*time.Location, error) {
	return func(ctx context.Context) (*time.Location, error) {
		user := ActingUser(ctx)
		var loc *time.Location
		err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			var err error
			loc, err = Zone(ctx, tx, occurredday.Personal, user)
			return err
		})
		return loc, err
	}
}

// ActingUser is the signed-in person, or the zero id when nobody is.
func ActingUser(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.Nil
	}
	return actor.UserID
}
