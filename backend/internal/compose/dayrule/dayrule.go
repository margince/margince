// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dayrule answers "which day is it for this reader on this screen".
//
// A member's own lists read the day in that member's zone. A screen that
// combines several colleagues reads it in the installation's zone. Every reader of
// a local day goes through here, so one member never gets two answers.
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

// Zone is the zone the surface reads the day in. user is the member whose list
// it is, and Combined ignores it. A member with no zone of their own, or no
// seat any more, reads the installation's zone.
func Zone(ctx context.Context, tx pgx.Tx, surface occurredday.Surface, user ids.UUID) (*time.Location, error) {
	if zone, known := rememberedZone(ctx, surface, user); known {
		return zone, nil
	}
	zone, err := readZone(ctx, tx, surface, user)
	if err == nil {
		rememberZone(ctx, surface, user, zone)
	}
	return zone, err
}

func readZone(ctx context.Context, tx pgx.Tx, surface occurredday.Surface, user ids.UUID) (*time.Location, error) {
	if surface == occurredday.Personal && !user.IsZero() {
		hours, _, err := identity.WorkingHoursOf(ctx, tx, ids.From[ids.UserKind](user))
		switch {
		case err == nil:
			// A stored zone this build cannot load reads the installation's
			// day. The member's list still opens.
			if loc, locErr := hours.Location(); locErr == nil {
				return loc, nil
			}
		case !errors.Is(err, apperrors.ErrNotFound):
			return nil, fmt.Errorf("dayrule: reading the member's zone: %w", err)
		}
		// The installation's zone, through the read's memo like any other ask.
		return Zone(ctx, tx, occurredday.Combined, ids.Nil)
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

// ActingReaderZone resolves the acting member's own zone for their own list,
// with the installation's zone as fallback.
func ActingReaderZone(pool *pgxpool.Pool) func(context.Context) (*time.Location, error) {
	return func(ctx context.Context) (*time.Location, error) {
		return zoneIn(ctx, pool, occurredday.Personal)
	}
}

// SurfaceOf is the surface of a read narrowed by owners. Only the acting
// member's own records make their own list. Any other set, or none, is a screen
// that combines colleagues.
func SurfaceOf(ctx context.Context, owners *storekit.OwnerSet) occurredday.Surface {
	user := ActingUser(ctx)
	if owners == nil || owners.Unassigned || user.IsZero() || len(owners.IDs) != 1 || owners.IDs[0] != user {
		return occurredday.Combined
	}
	return occurredday.Personal
}

// ZoneForOwners is the zone the day is read in for a read narrowed by owners.
func ZoneForOwners(ctx context.Context, pool *pgxpool.Pool, owners *storekit.OwnerSet) (*time.Location, error) {
	return zoneIn(ctx, pool, SurfaceOf(ctx, owners))
}

// ForAssignee is the zone for a read of one assignee's records, or of everyone's
// when assignee is nil. name is the IANA zone to hand a store that judges due
// dates itself, empty when the screen reads the installation's zone.
func ForAssignee(ctx context.Context, pool *pgxpool.Pool, assignee *ids.UUID) (zone *time.Location, name string, err error) {
	var owners *storekit.OwnerSet
	if assignee != nil {
		owners = &storekit.OwnerSet{IDs: []ids.UUID{*assignee}}
	}
	surface := SurfaceOf(ctx, owners)
	if zone, err = zoneIn(ctx, pool, surface); err == nil && surface == occurredday.Personal {
		name = zone.String()
	}
	return zone, name, err
}

func zoneIn(ctx context.Context, pool *pgxpool.Pool, surface occurredday.Surface) (*time.Location, error) {
	if zone, known := rememberedZone(ctx, surface, ActingUser(ctx)); known {
		return zone, nil
	}
	var loc *time.Location
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		loc, err = Zone(ctx, tx, surface, ActingUser(ctx))
		return err
	})
	return loc, err
}

// ActingUser is the signed-in member, or the zero id when nobody is.
func ActingUser(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.Nil
	}
	return actor.UserID
}

// InstallationZone loads the zone the installation's days are counted in. It
// reads ungated: which day it is does not need the settings grant.
func InstallationZone(ctx context.Context, pool *pgxpool.Pool) (*time.Location, error) {
	var name string
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		name, err = identity.TimezoneAppliedTx(ctx, tx)
		return err
	}); err != nil {
		return nil, fmt.Errorf("read the installation's timezone: %w", err)
	}
	return zoneNamed(name), nil
}

// zoneNamed resolves a stored zone name, answering UTC for one Go cannot load.
// The deals module makes the same fallback for a nil zone. A setting nobody can
// load then leaves both surfaces wrong about the day in the same direction.
func zoneNamed(name string) *time.Location {
	if zone, err := time.LoadLocation(name); err == nil {
		return zone
	}
	return time.UTC
}
