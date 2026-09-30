// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// Has this approval stopped being decidable, and who decided it.
//
// Five routes reach a terminal approval and only two of them emit anything:
// supersession, withdrawal and erasure all write the row and announce nothing,
// by design. So a caller taking back what it told the seats while the approval
// was live cannot learn the answer from an event, and asking the ROW here is
// what keeps the instant path and the backstop pass answering the same way.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NotifyActor is the fan-out consumer that writes the approval notices.
const NotifyActor = "system:approval-notify"

// OvertakenSweepActor is the pass that takes those notices back. Its own
// identity rather than the expiry sweep's: onlyTheExpirySweep admits
// ExpiryActor to a bulk WRITE, and widening that admission to reach a read
// would loosen the gate that stands in front of that write.
const OvertakenSweepActor = "system:notice-overtaken-sweep"

// Terminal is an approval that can no longer be decided, and the colleague who
// decided it. DecidedBy is nil where nobody did: the window closed on its own,
// or a flow replaced or withdrew it without recording a colleague.
type Terminal struct {
	ID        ids.ApprovalID
	DecidedBy *ids.UserID
}

// TerminalAmong reports which of these approvals have stopped being decidable.
//
// The clock is asked rather than the status column alone: a row whose window
// closed before the five-minute expiry sweep stamped it is already undecidable,
// and a notice still claiming it waits on somebody is already untrue.
//
// Candidates this installation does not hold are simply absent, on the same
// existence-hiding terms the inbox reads by — the callers are passes over rows
// they already read, not a question anybody asks about one approval.
func (s *Service) TerminalAmong(ctx context.Context, candidates []ids.ApprovalID) ([]Terminal, error) {
	if err := onlyAnOvertakingPass(ctx); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	asked := make([]ids.UUID, len(candidates))
	for i, id := range candidates {
		asked[i] = id.UUID
	}
	var terminal []Terminal
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, kind, status, expires_at, decided_by
			  FROM approval
			 WHERE id = ANY($1)`, asked)
		if err != nil {
			return fmt.Errorf("crmapprovals: reading which approvals stopped being decidable: %w", err)
		}
		defer rows.Close()
		now := s.now()
		for rows.Next() {
			var a row
			if err := rows.Scan(&a.ID, &a.Kind, &a.Status, &a.ExpiresAt, &a.DecidedBy); err != nil {
				return err
			}
			// effectiveStatus, not the column: it is the tree's single
			// definition of pending, and a second spelling here would read a
			// closed window as still open until the sweep caught up.
			if a.effectiveStatus(now) == statusPending {
				continue
			}
			terminal = append(terminal, Terminal{ID: a.ID, DecidedBy: a.DecidedBy})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return terminal, nil
}

// onlyAnOvertakingPass admits the two system passes that take an approval
// notice back, and refuses everyone else.
//
// The ACTOR ID is checked as well as the principal type, for the reason
// onlyTheExpirySweep checks it: "some system principal" is not the claim being
// made. Nothing here is scoped to a caller's row visibility — the candidates
// are handed in — so an open door would answer, for any approval id a caller
// cared to guess, whether it exists and which colleague decided it.
func onlyAnOvertakingPass(ctx context.Context) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf("crmapprovals: asking which approvals are terminal without a bound actor: %w", apperrors.ErrPermissionDenied)
	}
	if p.Type != principal.PrincipalSystem || (p.ID != NotifyActor && p.ID != OvertakenSweepActor) {
		return fmt.Errorf("crmapprovals: %s may not ask which approvals are terminal — the passes that do run as %s or %s: %w",
			p.ID, NotifyActor, OvertakenSweepActor, apperrors.ErrPermissionDenied)
	}
	return nil
}
