// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// Carrying the asks that name a retired contact onto the survivor of a merge.
//
// contacts owns the merge and calls this inside its transaction (contacts's
// IntroCarrier seam, wired in compose). An ask can name the retired contact
// twice: as the person to be introduced to, and as the person the route runs
// through. Both follow, or the survivor's page misses asks about them and a
// route keeps running through a record no read returns.
//
// intro_request_open_route allows one open ask per (contact, colleague,
// route). Two asks that were distinct before the merge can be the same ask
// after it — including two of the retired contact's own, once both of their
// columns point at the survivor. The older ask stands, the survivor's own
// before either; the rest are closed as cancelled, the way a requester closes
// one.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mergedDuplicateReason is what a closed duplicate says about itself.
const mergedDuplicateReason = "the contact was merged into a record that already had this ask open"

// CarryIntrosTx implements contacts.IntroCarrier.
//
// contact:update, the grant a contact merge runs under: this rewrites which
// contact an ask is about, and must not be a way for a caller with no contacts
// grant to re-home somebody else's asks.
func (s *Store) CarryIntrosTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	if err := s.closeMergedDuplicates(ctx, tx, from, to); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		WITH prior AS (
			SELECT id, contact_id, through_contact_id FROM intro_request
			 WHERE contact_id = $1 OR through_contact_id = $1
			 FOR UPDATE
		)
		UPDATE intro_request r
		   SET contact_id = CASE WHEN r.contact_id = $1 THEN $2 ELSE r.contact_id END,
		       through_contact_id = CASE WHEN r.through_contact_id = $1 THEN $2 ELSE r.through_contact_id END,
		       version = r.version + 1, updated_at = now()
		  FROM prior
		 WHERE r.id = prior.id
		RETURNING r.id, prior.contact_id, prior.through_contact_id, r.contact_id, r.through_contact_id`,
		from, to)
	if err != nil {
		return fmt.Errorf("introductions: carrying the asks onto the surviving contact: %w", err)
	}
	type rehome struct {
		id                          ids.UUID
		beforeContact, afterContact ids.UUID
		beforeThrough, afterThrough *ids.UUID
	}
	moved, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rehome, error) {
		var r rehome
		err := row.Scan(&r.id, &r.beforeContact, &r.beforeThrough, &r.afterContact, &r.afterThrough)
		return r, err
	})
	if err != nil {
		return fmt.Errorf("introductions: carrying the asks onto the surviving contact: %w", err)
	}
	for _, r := range moved {
		if _, err := storekit.Audit(ctx, tx, "update", "intro_request", r.id,
			map[string]any{fieldContactID: r.beforeContact, fieldThroughContactID: r.beforeThrough},
			map[string]any{fieldContactID: r.afterContact, fieldThroughContactID: r.afterThrough}); err != nil {
			return err
		}
	}
	return nil
}

// The two columns a re-home changes, in its before and after images.
const (
	fieldContactID        = "contact_id"
	fieldThroughContactID = "through_contact_id"
)

// closeMergedDuplicates cancels every open ask naming the retired contact that
// would share an open route with an earlier one once both columns are mapped.
// It runs before the re-home, so the re-home never meets the unique index.
func (s *Store) closeMergedDuplicates(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	open := openStatuses()
	rows, err := tx.Query(ctx, `
		WITH mapped AS (
			SELECT id, status, requested_at, introducer_user_id AS colleague,
			       CASE WHEN contact_id = $1 THEN $2 ELSE contact_id END AS about,
			       CASE WHEN through_contact_id = $1 THEN $2 ELSE through_contact_id END AS via
			  FROM intro_request
			 WHERE (contact_id = $1 OR through_contact_id = $1)
			   AND status = ANY($3) AND archived_at IS NULL
		),
		losers AS (
			SELECT m.id, m.status FROM mapped m
			 WHERE EXISTS (
			         SELECT 1 FROM intro_request k
			          WHERE k.contact_id = m.about AND k.introducer_user_id = m.colleague
			            AND k.through_contact_id IS NOT DISTINCT FROM m.via
			            AND k.status = ANY($3) AND k.archived_at IS NULL
			            AND k.contact_id <> $1 AND k.through_contact_id IS DISTINCT FROM $1)
			    OR EXISTS (
			         SELECT 1 FROM mapped o
			          WHERE o.about = m.about AND o.colleague = m.colleague
			            AND o.via IS NOT DISTINCT FROM m.via
			            AND (o.requested_at, o.id) < (m.requested_at, m.id))
		)
		UPDATE intro_request r
		   SET status = 'cancelled', decision_reason = $4, closed_at = $5,
		       version = r.version + 1, updated_at = now()
		  FROM losers
		 WHERE r.id = losers.id
		RETURNING r.id, losers.status`,
		from, to, open, mergedDuplicateReason, s.now().UTC())
	if err != nil {
		return fmt.Errorf("introductions: closing asks the merge made duplicates: %w", err)
	}
	type closedAsk struct {
		id     ids.UUID
		before Status
	}
	closed, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (closedAsk, error) {
		var c closedAsk
		err := row.Scan(&c.id, &c.before)
		return c, err
	})
	if err != nil {
		return fmt.Errorf("introductions: closing asks the merge made duplicates: %w", err)
	}
	for _, c := range closed {
		// The write shape a cancellation lands: the status before and after,
		// and the closed event. The event names the survivor, the only contact
		// a consumer can still resolve.
		auditID, err := storekit.Audit(ctx, tx, "update", "intro_request", c.id,
			map[string]any{auditedField: string(c.before)},
			map[string]any{auditedField: string(StatusCancelled)})
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, to.UUID, crmcontracts.PublicEventIntroRequestClosed{
			IntroRequestId: openapi_types.UUID(c.id),
			ContactId:      openapi_types.UUID(to.UUID),
			Reason:         crmcontracts.IntroRequestClosedCancelled,
		}); err != nil {
			return err
		}
	}
	return nil
}

// openStatuses is Open() as a list the database can compare against, derived
// from the census rather than spelled again.
func openStatuses() []string {
	var open []string
	for _, status := range everyStatus() {
		if Open(status) {
			open = append(open, string(status))
		}
	}
	return open
}
