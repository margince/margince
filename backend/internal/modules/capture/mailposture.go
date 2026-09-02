// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What one mailbox asks of the mail it brings in.
//
// The workspace setting (capture.mail_sharing) is a floor over everybody; this
// is one seat's answer for their own mailbox, and the two compose: a workspace
// with sharing off holds every message whatever a mailbox asks, and a mailbox
// asking to be held is held whatever the workspace allows. Only the direction
// that OPENS a message needs both to agree.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The postures a mailbox can be in, in the order the audience derivation reads
// them: shared opens a message on arrival, classified holds it until something
// judges it, held keeps it whatever any verdict concludes.
const (
	PostureShared     = "shared"
	PostureClassified = "classified"
	PostureHeld       = "held"
)

// validPosture answers whether a word names a posture. The database CHECK says
// the same thing; this one turns a bad request into a 422 naming the field
// rather than a constraint violation naming a column.
func validPosture(p string) bool {
	return p == PostureShared || p == PostureClassified || p == PostureHeld
}

// InvalidPostureError is a posture word the product does not have.
type InvalidPostureError struct{ Posture string }

func (e *InvalidPostureError) Error() string {
	return "capture: " + e.Posture + " is not a mail posture"
}

// FieldFault answers a malformed posture as a 422 on the field that carried it.
func (e *InvalidPostureError) FieldFault() (field, code, message string) {
	return "posture", "invalid_posture", e.Error()
}

// SharedPostureNotAllowedError refuses `shared` in a workspace that has not
// opted into it.
type SharedPostureNotAllowedError struct{}

func (e *SharedPostureNotAllowedError) Error() string {
	return "capture: this workspace does not allow a shared mailbox — an admin turns that on, and the works-council agreement it implies is the customer's to hold"
}

// FieldFault answers the refusal as a 422 rather than a 403: the caller's
// authority over their own mailbox is not in question, the workspace's posture
// is.
func (e *SharedPostureNotAllowedError) FieldFault() (field, code, message string) {
	return "posture", "shared_posture_not_allowed", e.Error()
}

// SetMailPosture records what this seat's mailbox asks of its mail.
//
// Human-only and own-mailbox-only: the statement matches on the acting user's
// id, so there is no id a caller could pass to reach a colleague's connection.
// An agent never answers this — what a seat's colleagues may read of that
// seat's correspondence is not a question an agent acts on behalf of anybody.
//
// `shared` additionally needs the workspace to allow it. That gate is here
// rather than in the handler because the posture is also settable at grant, and
// a check in one transport would leave the other open.
func (r *Registry) SetMailPosture(
	ctx context.Context, name, posture string, applyToHistory bool,
) (ConnectionView, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ConnectionView{}, fmt.Errorf("capture: only a human sets a mailbox's posture: %w", apperrors.ErrPermissionDenied)
	}
	if !validPosture(posture) {
		return ConnectionView{}, &InvalidPostureError{Posture: posture}
	}
	if _, ok := r.connectors[name]; !ok {
		return ConnectionView{}, ErrNoConnection
	}

	var before string
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := refuseSharedWithoutOptInTx(ctx, tx, posture); err != nil {
			return err
		}
		// The prior answer comes out of the same statement that replaces it, so
		// the audit row's before-image is the row as it actually stood rather
		// than a value read separately that something else could have moved in
		// between.
		row := tx.QueryRow(ctx, `
			UPDATE capture_connection
			   SET mail_posture = $3
			 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL
			RETURNING (SELECT c.mail_posture
			             FROM capture_connection c WHERE c.id = capture_connection.id)`,
			actor.UserID, name, posture)
		if err := row.Scan(&before); err != nil {
			return err
		}
		// Audit-only, like every other capture-configuration change beside it:
		// the closed public-event catalog carries no type for a posture, and
		// inventing one would put a mailbox's own setting on the outbound bus.
		if _, err := storekit.Audit(ctx, tx, "update", captureSettingsObject, storekit.MustWorkspace(ctx),
			mailPostureImage(name, before), mailPostureImage(name, posture)); err != nil {
			return err
		}
		if !applyToHistory {
			return nil
		}
		// In the SAME transaction as the posture change, so a caller who asked
		// for both never sees the posture moved and the history not — the two
		// are one answer to one question.
		//
		// Bounded per pass, looped here. A mailbox with a year of history has
		// tens of thousands of import rows, and the batch is what keeps this
		// from holding locks across all of them while captures wait.
		return r.narrowHistoryTo(ctx, tx, actor.UserID, posture)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionView{}, ErrNoConnection
	}
	if err != nil {
		return ConnectionView{}, fmt.Errorf("capture: setting the mailbox's posture: %w", err)
	}
	return r.connectionFor(ctx, name)
}

// narrowHistoryTo applies one posture to the mail this seat already imported,
// batch by batch, until nothing is left to do.
//
// The loop ends when a pass claims nothing, which holds only while the batch
// predicate excludes the value it writes. A predicate that stopped doing so
// would spin here forever, holding this transaction's locks and turning a
// settings change into a request that never returns.
//
// So progress is checked rather than assumed: every pass must leave FEWER rows
// to do than the one before. That fails on the first repeat instead of after a
// ceiling nobody would wait out, and it names the posture and the count, which
// is what somebody debugging it needs.
func (r *Registry) narrowHistoryTo(ctx context.Context, tx pgx.Tx, seat ids.UUID, posture string) error {
	remaining := -1
	for {
		moved, err := NarrowHistoryTx(ctx, tx, seat, posture, r.sink.recomputeAudience)
		if err != nil {
			return err
		}
		if moved == 0 {
			return nil
		}
		left, err := NarrowRemainingTx(ctx, tx, seat, posture)
		if err != nil {
			return err
		}
		if remaining >= 0 && left >= remaining {
			return fmt.Errorf(
				"capture: applying the %s posture to history made no progress: %d rows still due after a pass that moved %d",
				posture, left, moved)
		}
		remaining = left
	}
}

// refuseSharedWithoutOptInTx is the workspace gate on `shared`.
//
// Narrowing a mailbox never needs it: a seat may always ask for MORE privacy
// than the workspace requires, and making them ask an admin first would be a
// product that argues with somebody protecting their own mail.
func refuseSharedWithoutOptInTx(ctx context.Context, tx pgx.Tx, posture string) error {
	if posture != PostureShared {
		return nil
	}
	allowed, err := settings.ApplyTx(ctx, tx, SharedPostureAllowed)
	if err != nil {
		return fmt.Errorf("capture: reading the shared-posture opt-in: %w", err)
	}
	if !allowed {
		return &SharedPostureNotAllowedError{}
	}
	return nil
}

// mailPostureImage renders one side of the posture audit diff. The provider
// names WHICH mailbox moved, because a seat may have several and an image
// carrying only the posture would not say which one this row is about.
func mailPostureImage(provider, posture string) map[string]any {
	return map[string]any{auditKeyProvider: provider, auditKeyPosture: posture}
}
