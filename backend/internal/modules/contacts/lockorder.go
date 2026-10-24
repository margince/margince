// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The lock order of every contact writer that runs the duplicate lanes:
//
//  1. existing contact rows the writer changes or lands on;
//  2. the lane locks: phone, then address base, then name (plain and digraph);
//  3. new rows, and lead rows.
//
// So a writer holding a lane lock never waits on an existing contact. A create
// takes the lanes and then writes a new contact. An edit locks its contact and
// takes the lanes before it writes an address. A provider fill, whose contact
// the hand-off already holds, takes the base lane before it writes the
// address. So a create of the same address never holds a lane while it waits
// on the other writer's uncommitted row. Capture locks the lanes only on its
// create path, because its exact path locks the incumbent. A promotion takes
// the lanes between its contact and its lead.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// lockContactLanes takes the address-base and name locks for a contact that
// will hold these addresses under this name, in the lane order above.
func lockContactLanes(ctx context.Context, tx pgx.Tx, emails []string, name string) error {
	if err := lockAddressBaseLane(ctx, tx, emails); err != nil {
		return err
	}
	return lockNameLane(ctx, tx, name)
}

// lockLanesAndReview takes the candidate's lane locks and reads the review lanes
// under them, so a create that committed while this writer waited is seen.
func lockLanesAndReview(ctx context.Context, tx pgx.Tx, c ContactCandidate) (ContactResolution, error) {
	if err := lockContactLanes(ctx, tx, c.Emails, c.FullName); err != nil {
		return ContactResolution{}, err
	}
	return reviewContact(ctx, tx, c)
}
