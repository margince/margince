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
	gate, err := auth.ActivityContentClause(ctx, owedThreadRow, arg)
	if err != nil {
		return nil, err
	}
	body, prior := arg(storedBodyReadRunes), arg(priorBodyLimit)
	columns := storekit.SQLf(candidateColumns, body, 0, 0, prior)
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
 owed_thread.context_readable,
 `+provenance.MailBodyCompleteSQL("a")+` AND `+priorContextComplete+`
 FROM activity a CROSS JOIN LATERAL (%s) owed_thread
 LEFT JOIN LATERAL (SELECT prior.id, prior.subject, left(prior.body, $%d) AS body, prior.occurred_at
   FROM activity prior WHERE prior.id = owed_thread.prior_id) prior ON true
 LEFT JOIN activity_participant p ON p.activity_id=a.id
 WHERE a.direction='inbound' AND a.kind IN ('email','message') AND a.owed_verdict IS NOT NULL
 AND a.owed_verdict_at IS NOT NULL AND a.archived_at IS NULL AND `+auth.WorkspaceAudienceSQL("a")+` AND a.restricted_at IS NULL
 AND owed_thread.source_readable AND a.id>$%d AND ($%d::uuid IS NULL OR a.id=$%d)
 AND NOT EXISTS(SELECT 1 FROM activity_sales_state dismissed WHERE dismissed.thread_key=a.thread_key
 AND dismissed.kind=a.kind AND dismissed.channel_provider=coalesce(a.channel_provider,''))
 GROUP BY %s, owed_thread.context_readable ORDER BY a.id LIMIT $%d`, columns, owedThreadSQL(gate), prior, arg(after), exactArg, exactArg, candidateGroupBy, arg(min(max(limit, 1), 25)))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (OwedReviewCandidate, error) {
		var item OwedReviewCandidate
		candidate, err := scanOwedCandidate(row, &item.PriorID, &item.Version, &item.Verdict, &item.Ruleset, &item.JudgedAt, &item.Snapshot, &item.Pristine, &item.ContextReadable, &item.EvidenceComplete)
		candidate.Body = ClassifyBody(candidate.Body, bodyLimit)
		item.Candidate = candidate
		return item, err
	})
}

// owedThreadRow aliases the source and our earlier mail on its conversation
// inside owedThreadSQL, where the content gate renders once for all of them.
const owedThreadRow = "thread_row"

// owedThreadSQL judges the source a and our earlier mail on its conversation
// under one content gate. A message of ours is readable only while it is
// workspace mail and not restricted, because its body goes to the model too.
// It yields whether the source is readable, the newest readable earlier mail,
// and whether every earlier mail is readable.
func owedThreadSQL(gate string) string {
	return `SELECT coalesce(bool_or(thread_row.id = a.id AND thread_row.readable), false) AS source_readable,
   (array_agg(thread_row.id ORDER BY thread_row.occurred_at DESC, thread_row.id DESC)
      FILTER (WHERE thread_row.id <> a.id AND thread_row.readable))[1] AS prior_id,
   coalesce(bool_and(thread_row.readable) FILTER (WHERE thread_row.id <> a.id), true) AS context_readable
  FROM (SELECT thread_row.id, thread_row.occurred_at,
          (` + auth.WorkspaceAudienceSQL(owedThreadRow) + ` AND thread_row.restricted_at IS NULL AND (` + gate + `)) AS readable
        FROM activity thread_row
       WHERE thread_row.id = ANY(ARRAY(SELECT a.id UNION ALL SELECT earlier.id FROM activity earlier
         WHERE ` + ourOutboundInThisThread("earlier", "a") + `
           AND (earlier.occurred_at, earlier.id) < (a.occurred_at, a.id)))) thread_row`
}

// ApplyReviewedOwedVerdictTx keeps a reviewed correction on the ordinary audited writer.
func ApplyReviewedOwedVerdictTx(ctx context.Context, tx pgx.Tx, expected OwedReviewCandidate, verdict, ruleset, reason string, runID ids.UUID, bodyLimit, priorBodyLimit int) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
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
