// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// When a host is bookable, as the scheduler reads it.
//
// Working hours are a fact about a PERSON and identity owns the columns;
// availability is computed in activities, which may not import a sibling. So
// the edge is wired here like every other cross-module edge (ADR-0054 §9), and
// what crosses is one function: given a host, the hours and the zone.
//
// docs/explanation/scheduling.md is the design, including why the setting is
// personal rather than installation-wide and what an unset one falls back to.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// workingHoursResolver answers when one host is bookable, from the row identity
// keeps for them.
//
// The zone is resolved to a *time.Location HERE rather than passed as a name:
// activities compares instants, and a name it would have to load per candidate
// slot is a name it would have to fail on per candidate slot.
func workingHoursResolver(pool *pgxpool.Pool) activities.WorkingHoursResolver {
	db := InstallationDB(pool)
	return func(ctx context.Context, host ids.UserID) (activities.WorkingHours, error) {
		var hours identity.WorkingHours
		err := db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			hours, _, err = identity.WorkingHoursOf(ctx, tx, host)
			return err
		})
		if err != nil {
			return activities.WorkingHours{}, fmt.Errorf("compose: reading the host's working hours: %w", err)
		}
		location, err := hours.Location()
		if err != nil {
			return activities.WorkingHours{}, err
		}
		return activities.WorkingHours{
			StartMinute: hours.StartMinute, EndMinute: hours.EndMinute,
			Days: hours.Days, Location: location,
		}, nil
	}
}
