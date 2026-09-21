// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TakeOverAssertedActivityTx writes a connector's own reading of a message over
// a row a caller ASSERTED, and restamps the provenance to name the connector
// that read it.
//
// An import states what its source system remembered about a message; a
// connector read the message itself. When both describe one Message-ID the read
// copy is the better one — an exporting CRM stores its own rendering, with the
// HTML stripped, the quoted history rewritten and inline images dropped.
//
// It lives here rather than in capture because `activity` is this module's
// table to write. Capture decides that a take-over is due and calls through the
// seam compose injects, the same shape RecomputeAudienceTx travels.
//
// Only the fields a connector observes directly are written. The row's links,
// its participants and its audience are left as they are: they were decided
// when the row was created, other records point at them, and a capture arriving
// second has nothing better to say about who the message is filed against.
//
// `version` is not written: trg_activity_updated bumps it on every UPDATE of
// this table, so naming it would advance it twice and make one take-over look
// like two edits to a reader comparing versions.
// `wasCapturedBy` is the provenance being replaced, supplied by the caller
// because the caller has already read it to decide the row was asserted at all.
// Reading it again here would be a second reader of an audience-bearing row for
// a value this call already holds.
//
// The ROW-level authority is established BEFORE this is reached, and it is
// ResolveBindableIdentity: capture holds this row's id only because that
// function answered with it. Capture enforces that by calling this only for an
// id the resolve returned — a natural-key collision reaches
// replayClaimIsProvenTx instead, because that id was never vetted against a
// seat.
//
// The resolve says yes two ways. The SAME SEAT captured the row, and then
// restamping captured_by costs that seat nothing, which is what makes the
// rewrite safe rather than merely authorized: every scope clause reading the
// column matches on the trailing user id (auth/activitywritescope.go,
// auth/inheritedscope.go), so `human:<seat>` becoming `connector:gmail:<seat>`
// keeps the same seat's read and write authority over their own row. Or the row
// is an IMPORT naming an address the arriving seat has proven is theirs, which
// ResolveBindableIdentityProving admits and compose injects on the capture door
// only. That arm does move authority between seats, which is why the proof
// behind it is deliberately narrow — provider-attested labels alone, stated in
// SeatProvedAddressTx.
//
// Two gates that look right here are deliberately not used:
//
//   - EnsureActivityVisible admits any row the caller can merely DISCOVER, and
//     a link-less activity is discoverable by every seat in the workspace.
//     Discovery must never authorize a rewrite.
//   - EnsureActivityWritable asks whether the ROW is the caller's to change —
//     captured_by, assignee, host, or a writable link. A connector satisfies
//     captured_by only AFTER the restamp this function performs, so it would
//     refuse every legitimate take-over, which is the whole feature.
func TakeOverAssertedActivityTx(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, subject, body, wasCapturedBy, capturedBy string,
) error {
	// The OBJECT permission still binds, and is this function's own to ask. A
	// connector acts with the permissions of the human who granted it, so an
	// ordinary member's mailbox passes and a create-only passport does not.
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	// The liveness refusal, and the reason it is a lock rather than a
	// predicate: an archived row may have been archived by ERASURE, and
	// writing a message back over it would restore content retention
	// destroyed. LiveOnly answers ErrNotFound for an archived row, which the
	// caller reads as "nothing to take over".
	//
	// ResolveBindableIdentity already refused an archived holder, so this
	// closes the window between that read and this write rather than repeating
	// it.
	//
	// This is ALSO the statutory-hold refusal, which is why no
	// `restricted_at IS NULL` predicate follows it. The
	// activity_restricted_is_archived CHECK makes held-but-live a state the
	// table cannot hold, so every held row is archived and LiveOnly refuses it —
	// and both writers that place a hold (privacy.PinToFloor and the erasure's
	// restrict arm) archive in the same statement for that reason. Adding the
	// predicate would be a fourth refusal behind three, unreachable by
	// construction, and a reader would take it for the one doing the work.
	// TestAMessageUnderAStatutoryHoldIsNotTakenOver holds the outcome.
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	// COALESCE on subject and body: a connector that reads a message with no
	// subject supplies an empty one, and letting that blank a subject the
	// import had would lose the only description the row carries.
	tag, err := tx.Exec(ctx, `
		UPDATE activity
		   SET subject     = COALESCE(NULLIF($2, ''), subject),
		       body        = COALESCE(NULLIF($3, ''), body),
		       captured_by = $4
		 WHERE id = $1`,
		activityID, subject, body, capturedBy)
	if err != nil {
		return fmt.Errorf("activities: writing the read message over %s: %w", activityID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("activities: the asserted row %s vanished under the take-over", activityID)
	}
	before := map[string]any{"captured_by": wasCapturedBy}
	after := map[string]any{"captured_by": capturedBy}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", activityID.UUID, before, after)
	if err != nil {
		return fmt.Errorf("activities: auditing the take-over of %s: %w", activityID, err)
	}
	// The body is announced as a presence flag, which is all this event carries
	// for it: a subscriber that must know re-reads the row under its own
	// audience rather than receiving content on the wire.
	touched := true
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Body: &touched},
	}); err != nil {
		return fmt.Errorf("activities: emitting the take-over of %s: %w", activityID, err)
	}
	return nil
}
