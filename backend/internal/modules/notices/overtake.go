// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// A line that stopped being true without its reader doing anything.
//
// One approval's notice is written once per seat that could decide it. The
// moment any of them decides, every other line says a decision waits on a
// reader who can do nothing about it, and only a pass acting for all of them
// can take those back — which is why the door here is a system one and takes
// no seat.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalKeyPrefix heads the key that collapses every announcement about one
// approval onto the line each seat already holds. The fan-out that writes those
// lines and the pass that takes them back both compose it through
// ApprovalDedupeKey, because a key spelled by hand beside this one would match
// nothing and fail quietly.
const approvalKeyPrefix = KindApprovalPending + ":"

// ApprovalDedupeKey is that key, for the approval it names.
func ApprovalDedupeKey(approval ids.ApprovalID) string {
	return approvalKeyPrefix + approval.String()
}

// Overtaking is one approval whose lines have stopped being true, and the
// colleague whose decision did it. By is nil where nobody decided: the window
// closed on its own, or a flow replaced or withdrew the proposal.
type Overtaking struct {
	Approval ids.ApprovalID
	By       *ids.UserID
}

// OvertakeApprovalNotices marks every seat's line overtaken for approvals that
// can no longer be decided, and answers how many lines moved.
//
// One statement rather than a loop: each approval carries its own decider, so
// the pairs arrive as two arrays and the update joins against them.
//
// A line its reader already opened is left alone. Overtaking and reading answer
// different questions — one says nobody needs to act, the other says somebody
// did — and stamping both over a reader who acted would overwrite the only
// record that they did.
func (s *Store) OvertakeApprovalNotices(ctx context.Context, o []Overtaking) (int, error) {
	if err := onlyASystemPass(ctx, "taking back the lines about a decided approval"); err != nil {
		return 0, err
	}
	if len(o) == 0 {
		return 0, nil
	}
	keys, deciders := keysAndDeciders(o)
	var taken []ids.UUID
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// standingOnly is spelled out rather than composed: the arms are
		// table-qualified against the unnest join and the fragment is not.
		rows, txErr := tx.Query(ctx, `
			UPDATE notice n
			   SET overtaken_at = now(), overtaken_by = v.by
			  FROM unnest($1::text[], $2::uuid[]) AS v(key, by)
			 WHERE n.dedupe_key = v.key AND n.read_at IS NULL AND n.overtaken_at IS NULL
			RETURNING n.id`, keys, deciders)
		if txErr != nil {
			return txErr
		}
		taken, txErr = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if txErr != nil {
			return txErr
		}
		for _, line := range taken {
			if _, txErr = storekit.AuditEventWithEvidence(ctx, tx, "update", "notice", line,
				nil, map[string]any{"overtaken": true}); txErr != nil {
				return txErr
			}
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("notices: taking back the lines about a decided approval: %w", err)
	}
	return len(taken), nil
}

// keysAndDeciders splits the pairs into the two arrays the join reads, holding
// their order: a decider is nil where nobody decided, and lands as SQL NULL.
func keysAndDeciders(o []Overtaking) ([]string, []*ids.UUID) {
	keys := make([]string, len(o))
	deciders := make([]*ids.UUID, len(o))
	for i, one := range o {
		keys[i] = ApprovalDedupeKey(one.Approval)
		if one.By != nil {
			deciders[i] = &one.By.UUID
		}
	}
	return keys, deciders
}

// StandingApprovalReferences names every approval whose lines still claim a
// decision waits on somebody — what the backstop pass carries to approvals to
// ask which of them stopped being decidable.
//
// NO LIMIT, deliberately. The kinds the expiry sweep excludes never age out, so
// a bounded read fills with permanently pending approvals and never reaches a
// stale reference behind them, and the pass reports success with nothing
// failing. The set is bounded by the partial unread index instead, which is the
// size of the inbox fan-out.
func (s *Store) StandingApprovalReferences(ctx context.Context) ([]ids.ApprovalID, error) {
	if err := onlyASystemPass(ctx, "asking which approvals still have lines standing"); err != nil {
		return nil, err
	}
	var standing []ids.ApprovalID
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, `
			SELECT DISTINCT dedupe_key
			  FROM notice
			 WHERE kind = $1 AND starts_with(dedupe_key, $2) AND `+standingOnly,
			KindApprovalPending, approvalKeyPrefix)
		if txErr != nil {
			return txErr
		}
		keys, txErr := pgx.CollectRows(rows, pgx.RowTo[string])
		if txErr != nil {
			return txErr
		}
		standing = make([]ids.ApprovalID, 0, len(keys))
		for _, key := range keys {
			approval, parseErr := ids.ParseAs[ids.ApprovalKind](strings.TrimPrefix(key, approvalKeyPrefix))
			if parseErr != nil {
				return parseErr
			}
			standing = append(standing, approval)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("notices: asking which approvals still have lines standing: %w", err)
	}
	return standing, nil
}

// onlyASystemPass admits the passes that take a line back for seats other than
// the caller, and refuses everyone else.
//
// The principal TYPE and not a named actor: both passes that reach here run
// under names declared in approvals, and a module may not import a sibling to
// read them. The actor-specific door is approvals' own — each pass first asks
// it which approvals stopped being decidable, and it admits those two names and
// nothing else — so a system principal arriving here with no such answer has
// nothing to take back.
func onlyASystemPass(ctx context.Context, act string) error {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalSystem {
		return fmt.Errorf("notices: %s is a system pass and never a seat's own act: %w",
			act, apperrors.ErrPermissionDenied)
	}
	return nil
}
