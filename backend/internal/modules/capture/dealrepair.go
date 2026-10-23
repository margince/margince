// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// DealUnknownProvenance is mail the repair leaves alone because the record
// does not say which mailbox captured it, or that mailbox's seat is gone.
const DealUnknownProvenance DealOutcome = "unknown_provenance"

// unfiledCapturedMailSQL selects the mail the deal repair inspects: live mail
// a connector captured, on no deal. Mail a human logged or filed on a deal is
// never touched. Callers append the cursor and order.
const unfiledCapturedMailSQL = `
	SELECT a.id FROM activity a
	 WHERE a.kind = 'email' AND a.channel_provider IS NULL
	   AND a.captured_by LIKE 'connector:%'
	   AND a.archived_at IS NULL AND a.restricted_at IS NULL
	   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'deal')`

// DealRepair files mail captured before the deal rule under the deal the rule
// finds for it now. AttributeDealTx decides each mail as the connector of a
// mailbox that imported it, with that seat's live authority. So the repair
// sees only what live capture would have seen. It asks nobody.
type DealRepair struct {
	Candidates DealCandidates
	Authority  authz.Resolver
}

// BatchTx links one page of mail. A second run changes nothing: a filed mail
// leaves the selection and an unfiled one decides the same way.
func (r DealRepair) BatchTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, err := unfiledCapturedMail(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	linked := 0
	for _, id := range scanned {
		outcome, err := r.attribute(ctx, tx, ids.From[ids.ActivityKind](id))
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if outcome == DealLinked {
			linked++
		}
	}
	return datatask.AdvanceUUID(in, scanned, linked)
}

// PreviewTx is what BatchTx would do to the first limit mails, by outcome,
// decided the same way and rolled back.
func (r DealRepair) PreviewTx(ctx context.Context, tx pgx.Tx, limit int) (map[DealOutcome]int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	scanned, err := unfiledCapturedMail(ctx, tx, ids.Nil, limit)
	if err != nil {
		return nil, err
	}
	counts := map[DealOutcome]int{}
	for _, id := range scanned {
		trial, err := tx.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("capture: opening a deal repair preview: %w", err)
		}
		outcome, err := r.attribute(ctx, trial, ids.From[ids.ActivityKind](id))
		if rollback := trial.Rollback(ctx); err == nil && rollback != nil {
			err = rollback
		}
		if err != nil {
			return nil, err
		}
		counts[outcome]++
	}
	return counts, nil
}

// attribute decides one mail as each mailbox that imported it, as live
// capture does once per capture, and stops at the first link.
func (r DealRepair) attribute(ctx context.Context, tx pgx.Tx, mail ids.ActivityID) (DealOutcome, error) {
	var capturedBy string
	if err := tx.QueryRow(ctx, `SELECT captured_by FROM activity WHERE id = $1`, mail).Scan(&capturedBy); err != nil {
		return "", fmt.Errorf("capture: reading who captured a mail: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT user_id FROM capture_import WHERE activity_id = $1 ORDER BY user_id`, mail)
	if err != nil {
		return "", fmt.Errorf("capture: reading the mailboxes a mail came from: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return "", fmt.Errorf("capture: reading the mailboxes a mail came from: %w", err)
	}
	outcome := DealUnknownProvenance
	name, ok := strings.CutPrefix(capturedBy, "connector:")
	if !ok || name == "" {
		return outcome, nil
	}
	for _, seat := range seats {
		acting, err := connectorActing(ctx, r.Authority, name, ids.From[ids.UserKind](seat))
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		verdict, err := AttributeDealTx(acting, tx, mail, r.Candidates, nil)
		if err != nil || verdict.Outcome == DealLinked {
			return verdict.Outcome, err
		}
		outcome = verdict.Outcome
	}
	return outcome, nil
}

func unfiledCapturedMail(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, unfiledCapturedMailSQL+` AND a.id > $1 ORDER BY a.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing mail for the deal repair: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: listing mail for the deal repair: %w", err)
	}
	return scanned, nil
}

// EstimateDealRepair bounds the work: how many captured mails the repair will
// inspect, at most maximum.
func EstimateDealRepair(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (`+unfiledCapturedMailSQL+` LIMIT $1) AS candidates`, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
