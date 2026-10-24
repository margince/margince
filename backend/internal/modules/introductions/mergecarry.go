// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// Carrying the asks that name a retired contact onto the survivor of a merge.
//
// contacts owns the merge and calls this inside its transaction (contacts's
// IntroCarrier seam, wired in compose). An ask can name the retired contact
// twice: as the contact to be introduced to, and as the contact the route
// runs through. Both follow, or the survivor's page misses asks about them and a
// route keeps running through a record no read returns.
//
// intro_request_open_route allows one open ask per (contact, colleague,
// route). Two asks that were distinct before the merge can be the same ask
// after it. The older ask stands, the survivor's own before either; the rest
// are closed as cancelled, the way a requester closes one. So is an open ask
// the merge turned into a route through the very contact it is about, which no
// longer describes a way in.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// What an ask the merge closed says about itself.
const (
	mergedDuplicateReason = "the contact was merged into a record that already had this ask open"
	mergedSelfRouteReason = "the contact was merged into the one this route ran through"
	// mergedRecordedDuplicateReason is shown in the merge preview for the recorded
	// introduction the merge archives, the newer of two for one pair.
	mergedRecordedDuplicateReason = "the contact was merged into a record that already had this introduction recorded"
)

// CarryIntrosTx implements contacts.IntroCarrier.
//
// contact:update and write authority over both contacts, which is what a
// contact merge asks: this rewrites which contact an ask is about, and must
// not let an in-process caller re-home asks between contacts it may not change.
func (s *Store) CarryIntrosTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	if err := admitIntroCarry(ctx, tx, from, to); err != nil {
		return err
	}
	if err := s.closeMergedDuplicates(ctx, tx, from, to); err != nil {
		return err
	}
	archived, archivedAt, err := s.archiveMergedRecordedDuplicates(ctx, tx, from, to)
	if err != nil {
		return err
	}
	for _, id := range archived {
		if _, err := storekit.Audit(ctx, tx, "update", "intro_request", id,
			map[string]any{fieldArchivedAt: nil}, map[string]any{fieldArchivedAt: archivedAt}); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `
		WITH prior AS (
			SELECT id, contact_id, through_contact_id, `+introCarryContact+` AS next_contact, `+introCarryThrough+` AS next_through FROM intro_request
			 WHERE `+introCarrySource+`
			 FOR UPDATE
		)
		UPDATE intro_request r
		   SET contact_id = prior.next_contact,
		       through_contact_id = prior.next_through,
		       version = r.version + 1, updated_at = now()
		  FROM prior
		 WHERE r.id = prior.id
		RETURNING r.id, prior.contact_id, prior.through_contact_id, r.contact_id, r.through_contact_id`,
		introCarryArgs(from, to))
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
	fieldArchivedAt       = "archived_at"
)

// closeMergedDuplicates cancels every open ask naming the retired contact that
// would share an open route with an earlier one once both columns are mapped,
// or that would run through the contact it is about. It runs before the
// re-home, so the re-home never meets the unique index.
//
// Every ask naming either contact is locked first, in id order, so an ask
// completed while the merge waited is read in its new state and is no longer
// a candidate; the update is guarded on the status and version it decided on.
func (s *Store) closeMergedDuplicates(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	if _, err := tx.Exec(ctx, `
		SELECT id FROM intro_request
		 WHERE contact_id IN ($1, $2) OR through_contact_id IN ($1, $2)
		 ORDER BY id FOR UPDATE`, from, to); err != nil {
		return fmt.Errorf("introductions: locking the asks the merge will re-home: %w", err)
	}
	args := introCarryArgs(from, to)
	args["closed"] = s.now().UTC()
	rows, err := tx.Query(ctx, `
`+introCarryClosures+`
		UPDATE intro_request r
		   SET status = 'cancelled', closed_at = @closed, version = r.version + 1, updated_at = now(),
		       decision_reason = CASE WHEN losers.self_route THEN @self_reason ELSE @duplicate_reason END
		  FROM losers
		 WHERE r.id = losers.id AND r.status = losers.status AND r.version = losers.version
		RETURNING r.id, losers.status, losers.about`,
		args)
	if err != nil {
		return fmt.Errorf("introductions: closing asks the merge made duplicates: %w", err)
	}
	type closedAsk struct {
		id, about ids.UUID
		before    Status
	}
	closed, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (closedAsk, error) {
		var c closedAsk
		err := row.Scan(&c.id, &c.before, &c.about)
		return c, err
	})
	if err != nil {
		return fmt.Errorf("introductions: closing asks the merge made duplicates: %w", err)
	}
	for _, c := range closed {
		if err := auditMergeClosedAsk(ctx, tx, c.id, c.about, c.before); err != nil {
			return err
		}
	}
	return nil
}

// archiveMergedRecordedDuplicates archives the newer of two recorded
// introductions the merge would put on one pair, and answers which and when so
// the caller audits them with the re-home. closeMergedDuplicates has already
// locked every row of both contacts.
func (s *Store) archiveMergedRecordedDuplicates(
	ctx context.Context, tx pgx.Tx, from, to ids.ContactID,
) ([]ids.UUID, time.Time, error) {
	archivedAt := s.now().UTC()
	rows, err := tx.Query(ctx, `
		UPDATE intro_request r
		   SET archived_at = @archived_at, version = r.version + 1, updated_at = now()
		 WHERE r.id IN (`+introCarryRecordedLosers+`)
		RETURNING r.id`,
		pgx.NamedArgs{"source": from, "target": to, "archived_at": archivedAt})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("introductions: archiving recorded introductions the merge made duplicates: %w", err)
	}
	archived, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("introductions: archiving recorded introductions the merge made duplicates: %w", err)
	}
	return archived, archivedAt, nil
}

// auditMergeClosedAsk lands the write shape a cancellation lands: the status
// before and after, and the closed event about the contact the ask is about
// once re-homed — the one a consumer can still resolve.
func auditMergeClosedAsk(ctx context.Context, tx pgx.Tx, id, about ids.UUID, before Status) error {
	auditID, err := storekit.Audit(ctx, tx, "update", "intro_request", id,
		map[string]any{auditedField: string(before)},
		map[string]any{auditedField: string(StatusCancelled)})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, about, crmcontracts.PublicEventIntroRequestClosed{
		IntroRequestId: openapi_types.UUID(id),
		ContactId:      openapi_types.UUID(about),
		Reason:         crmcontracts.IntroRequestClosedCancelled,
	})
}

// openStatuses is Open() as a list the database can compare against, derived
// from the census rather than spelled again.
func openStatuses() []string {
	var open []string
	for _, status := range EveryStatus() {
		if Open(status) {
			open = append(open, string(status))
		}
	}
	return open
}

func admitIntroCarry(ctx context.Context, tx pgx.Tx, from, to ids.ContactID) error {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	for _, named := range []ids.ContactID{from, to} {
		if err := auth.EnsureWritable(ctx, tx, "contact", named.UUID); err != nil {
			return err
		}
	}
	return nil
}
