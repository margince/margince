// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// personalSweepBatch bounds one pass, matching noiseSweepBatch: the backlog is a
// query, so what a pass does not reach this tick it reaches the next.
const personalSweepBatch = 500

// personalSweepActor is the principal the personal sweep destroys under.
//
// The sweep runs from a fleet job whose context carries a workspace and nothing
// else. The purge refuses a context with no actor or no correlation id. Without
// this binding, every pass that found mail due failed, and the mail stayed.
const personalSweepActor = "system:capture-personal-purge"

// SweepPersonalMail destroys the personal correspondence whose undo window has
// closed, for every seat in the workspace that has any. It is the only place
// where a classification leads to irreversible deletion with nobody in the loop.
//
// Three guards make that survivable, and removing any one of them breaks it:
//
//   - SelectPersonalPurgeTx measures the window per message and refuses any
//     address carrying a live `business` override, which is how a contact cancels.
//   - A verdict the classifier reached alone waits four times as long as one a
//     contact reached, because nobody has looked at it.
//   - The statutory floor shields correspondence the law requires kept.
//
// It reuses carryOut, so a message destroyed here goes through the same
// sequence an owner's own purge uses.
func (p *CapturePurger) SweepPersonalMail(ctx context.Context, windows capture.PersonalPurgeWindows) (int, error) {
	// Bound here rather than by the caller, because the sweep is the system's act
	// by definition. Its audit rows name the sweep, whoever started the pass.
	ctx = principal.SystemActing(ctx, personalSweepActor)
	var seats []ids.UUID
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		var err error
		seats, err = capture.SeatsWithPersonalMailDueTx(ctx, tx, windows, personalSweepBatch)
		return err
	}); err != nil {
		return 0, fmt.Errorf("verdict: finding seats with personal mail due: %w", err)
	}
	destroyed := 0
	for _, seat := range seats {
		var subject capture.PurgeSubject
		if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
			floor, _, err := purgeFloorTx(ctx, tx)
			if err != nil {
				return err
			}
			subject, err = capture.SelectPersonalPurgeTx(ctx, tx, seat, windows, floor, personalSweepBatch)
			return err
		}); err != nil {
			return destroyed, fmt.Errorf("verdict: selecting personal mail for one seat: %w", err)
		}
		// No contacts are anonymised here. A `personal` verdict creates no
		// counterparty record in the first place (captureverdict's KindPersonal
		// arm returns without one), so there is nothing this sweep could match.
		// A selector that looked for one would find a contact somebody made for
		// another reason, or find nothing while implying it had looked.
		if _, err := p.carryOut(ctx, subject, nil, nil, seat, privacy.PurgePersonalVerdict); err != nil {
			return destroyed, fmt.Errorf("verdict: destroying personal mail: %w", err)
		}
		destroyed += len(subject.SoleImports)
	}
	return destroyed, nil
}
