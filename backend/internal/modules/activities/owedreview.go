// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// OwedReviewCandidate binds the model input to a complete source snapshot.
type OwedReviewCandidate struct {
	Candidate       OwedCandidate
	Version         int64
	Verdict         string
	Ruleset         string
	JudgedAt        time.Time
	Snapshot        string
	Pristine        bool
	ContextReadable bool
	// EvidenceComplete says the source and our newest prior message were read
	// in full, the test every automatic request decision reads by.
	EvidenceComplete bool
	PriorID          *ids.UUID
}

// OwedReviewPageTx keeps classification review inside the reader's content scope.
func OwedReviewPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit, bodyLimit, priorBodyLimit int) ([]OwedReviewCandidate, error) {
	return owedReviewReadTx(ctx, tx, after, nil, limit, bodyLimit, priorBodyLimit)
}

// OwedReviewTx reuses the preview projection when a reviewed source is rechecked.
func OwedReviewTx(ctx context.Context, tx pgx.Tx, id ids.UUID, bodyLimit, priorBodyLimit int) (OwedReviewCandidate, error) {
	rows, err := owedReviewReadTx(ctx, tx, ids.Nil, &id, 1, bodyLimit, priorBodyLimit)
	if err != nil {
		return OwedReviewCandidate{}, err
	}
	if len(rows) == 0 {
		return OwedReviewCandidate{}, apperrors.ErrNotFound
	}
	return rows[0], nil
}

func owedReviewReadTx(ctx context.Context, tx pgx.Tx, after ids.UUID, exact *ids.UUID, limit, bodyLimit, priorBodyLimit int) ([]OwedReviewCandidate, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	priorScope, err := auth.ActivityContentClause(ctx, "prior", arg)
	if err != nil {
		return nil, err
	}
	body, prior := arg(bodyLimit), arg(priorBodyLimit)
	columns := storekit.SQLf(candidateColumns, body, 0, 0, prior)
	join := storekit.SQLf(priorOutboundVisibleJoin(priorScope), body, 0, 0, prior)
	exactArg := arg(exact)
	query := storekit.SQLf(`SELECT %s, prior.id, a.version,a.owed_verdict,coalesce(a.owed_verdict_ruleset,''),a.owed_verdict_at,
 encode(sha256(convert_to(jsonb_build_object('source',to_jsonb(a),'participants',jsonb_agg(to_jsonb(p) ORDER BY p.id),
 'prior',(SELECT to_jsonb(full_prior) FROM activity full_prior WHERE full_prior.id=prior.id))::text,'UTF8')),'hex'),
 -- The native verdict and its audit share the transaction's now().
 EXISTS(SELECT 1 FROM audit_log proof WHERE proof.entity_type='activity' AND proof.entity_id=a.id
 AND proof.actor_type IN ('system','agent') AND proof.occurred_at=a.owed_verdict_at
 AND proof.after->>'owed_verdict'=a.owed_verdict AND proof.after->>'owed_verdict_ruleset'=a.owed_verdict_ruleset)
 AND NOT EXISTS(SELECT 1 FROM audit_log human WHERE human.entity_type='activity' AND human.entity_id=a.id
 AND human.actor_type='human' AND human.occurred_at>=a.owed_verdict_at),
 NOT EXISTS(SELECT 1 FROM activity prior WHERE `+ourOutboundInThisThread("prior", "a")+`
 AND (prior.occurred_at,prior.id)<(a.occurred_at,a.id)
 AND (prior.audience<>'workspace' OR prior.restricted_at IS NOT NULL OR NOT (`+priorScope+`))),
 `+provenance.MailBodyCompleteSQL("a")+` AND `+priorContextComplete+`
 FROM activity a LEFT JOIN activity_participant p ON p.activity_id=a.id %s
 WHERE a.direction='inbound' AND a.kind IN ('email','message') AND a.owed_verdict IS NOT NULL
 AND a.owed_verdict_at IS NOT NULL AND a.archived_at IS NULL AND a.audience='workspace' AND a.restricted_at IS NULL
 AND (%s) AND a.id>$%d AND ($%d::uuid IS NULL OR a.id=$%d)
 AND NOT EXISTS(SELECT 1 FROM activity_sales_state dismissed WHERE dismissed.thread_key=a.thread_key
 AND dismissed.kind=a.kind AND dismissed.channel_provider=coalesce(a.channel_provider,''))
 GROUP BY %s ORDER BY a.id LIMIT $%d`, columns, join, scope, arg(after), exactArg, exactArg, candidateGroupBy, arg(min(max(limit, 1), 25)))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (OwedReviewCandidate, error) {
		var item OwedReviewCandidate
		candidate, err := scanOwedCandidate(row, &item.PriorID, &item.Version, &item.Verdict, &item.Ruleset, &item.JudgedAt, &item.Snapshot, &item.Pristine, &item.ContextReadable, &item.EvidenceComplete)
		item.Candidate = candidate
		return item, err
	})
}

// ApplyReviewedOwedVerdictTx keeps a human-reviewed correction on the ordinary audited writer.
func ApplyReviewedOwedVerdictTx(ctx context.Context, tx pgx.Tx, expected OwedReviewCandidate, verdict, ruleset, reason string, runID ids.UUID, bodyLimit, priorBodyLimit int) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return false, apperrors.ErrPermissionDenied
	}
	if err := auth.EnsureActivityWritable(ctx, tx, expected.Candidate.ID); err != nil {
		return false, err
	}
	lockIDs := []ids.UUID{expected.Candidate.ID}
	if expected.PriorID != nil {
		lockIDs = append(lockIDs, *expected.PriorID)
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM activity WHERE id=ANY(@ids) ORDER BY id FOR UPDATE`, pgx.NamedArgs{"ids": lockIDs}); err != nil {
		return false, err
	}
	if err := auth.EnsureActivityWritable(ctx, tx, expected.Candidate.ID); err != nil {
		return false, err
	}
	current, err := OwedReviewTx(ctx, tx, expected.Candidate.ID, bodyLimit, priorBodyLimit)
	if err != nil {
		return false, err
	}
	if !current.Pristine || !current.ContextReadable || current.Version != expected.Version || current.Snapshot != expected.Snapshot || current.Verdict != expected.Verdict || current.Ruleset != expected.Ruleset {
		return false, apperrors.ErrConflict
	}
	if current.Verdict == verdict {
		return false, nil
	}
	// The correction was predicted from the source's text. It stands only while
	// that text and the context it was read in were read in full.
	if !current.EvidenceComplete {
		return false, apperrors.ErrConflict
	}
	return setOwedVerdictTx(ctx, tx, current.Candidate.ID, verdict, ruleset, time.Time{}, map[string]any{"reviewed_classification": true, "repair_reason": reason, "repair_rule": ruleset, "repair_run": runID})
}
