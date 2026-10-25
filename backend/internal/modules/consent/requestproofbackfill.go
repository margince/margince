// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The proof of a request closed before proofs were written, rebuilt from what
// the request row and audit_log still hold.
//
// What no record holds stays unknown. The identity check is `unknown` unless
// the request came through the confirm link. The counts are unknown when no
// erasure tombstone names the request. The closer is empty with no audit row.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/erasureaccount"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// proofBackfillSource marks a proof's audit row as rebuilt from the records.
const proofBackfillSource = "request_record"

// ProofBackfillPage is one page of the backfill: the closed requests it
// inspected, in id order, and how many of them it gave a proof.
type ProofBackfillPage struct {
	Scanned []ids.UUID
	Changed int
}

// ProofBackfillSeams are what the backfill reads from other modules: the
// installation's zone, and the account an erasure's tombstone certifies.
type ProofBackfillSeams struct {
	Zone InstallationZoneFunc
	// Erasure answers the account certified under a reason, and whether a
	// tombstone carries that reason at all.
	Erasure func(ctx context.Context, tx pgx.Tx, reason string) (erasureaccount.Account, bool, error)
}

// BackfillRequestProofsTx gives up to limit closed requests after `after` a
// proof, inside the data task's transaction. A request that has one keeps it,
// so a replay from the start writes nothing.
func BackfillRequestProofsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, seams ProofBackfillSeams) (ProofBackfillPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ProofBackfillPage{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM data_subject_request
		 WHERE id > $1 AND status = ANY($2)
		 ORDER BY id LIMIT $3`, after, closedDSRStatuses(), limit)
	if err != nil {
		return ProofBackfillPage{}, fmt.Errorf("consent: paging closed requests without a proof: %w", err)
	}
	var page ProofBackfillPage
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ProofBackfillPage{}, err
		}
		page.Scanned = append(page.Scanned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ProofBackfillPage{}, err
	}
	zone, err := seams.Zone(ctx, tx)
	if err != nil {
		return ProofBackfillPage{}, err
	}
	for _, id := range page.Scanned {
		wrote, err := backfillRequestProof(ctx, tx, id, zone, seams)
		if err != nil {
			return ProofBackfillPage{}, err
		}
		if wrote {
			page.Changed++
		}
	}
	return page, nil
}

// CountRequestsWithoutProof counts closed requests that have no proof, up to
// limit+1 so the estimate stays bounded.
func CountRequestsWithoutProof(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, "privacy_request", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM data_subject_request d
		 WHERE d.status = ANY($1)
		   AND NOT EXISTS (SELECT 1 FROM privacy_request_proof p WHERE p.request_id = d.id)
		 LIMIT $2) candidates`, closedDSRStatuses(), limit+1).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("consent: counting closed requests without a proof: %w", err)
	}
	return count, nil
}

// closedRequestRecord is what the records hold about one closed request.
type closedRequestRecord struct {
	kind, status, subjectRef string
	source                   proofSource
	channel, mailedTo        *string
	updatedAt                time.Time
	closerType, closerID     *string
	closedAt                 *time.Time
}

func backfillRequestProof(ctx context.Context, tx pgx.Tx, id ids.UUID, zone *time.Location, seams ProofBackfillSeams) (bool, error) {
	rec := closedRequestRecord{source: proofSource{zone: zone}}
	// The closing audit row is the latest update that moved the request into
	// the status it holds now. A write that left it there names no closer.
	err := tx.QueryRow(ctx, `
		SELECT d.kind, d.status, d.subject_ref, coalesce(d.receipt_reference, d.id::text),
		       coalesce(d.received_at, d.created_at), d.channel, d.updated_at,
		       `+linkMailedToSQL+`,
		       closing.actor_type, closing.actor_id, closing.occurred_at
		  FROM data_subject_request d
		  LEFT JOIN LATERAL (
		        SELECT a.actor_type, a.actor_id, a.occurred_at FROM audit_log a
		         WHERE a.entity_type = 'data_subject_request' AND a.entity_id = d.id
		           AND a.action = 'update' AND a.after->>'status' = d.status
		           AND a.before->>'status' IS DISTINCT FROM d.status
		         ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1) closing ON true
		 WHERE d.id = $1
		   FOR UPDATE OF d`, id,
	).Scan(&rec.kind, &rec.status, &rec.subjectRef, &rec.source.requestNumber, &rec.source.receivedAt,
		&rec.channel, &rec.updatedAt, &rec.mailedTo, &rec.closerType, &rec.closerID, &rec.closedAt)
	if err != nil {
		return false, fmt.Errorf("consent: reading a closed request for its proof: %w", err)
	}
	erased, known, err := recordedErasure(ctx, tx, id, rec, seams)
	if err != nil {
		return false, err
	}
	rec.source.identityCheck = identityUnknown
	if rec.channel != nil && *rec.channel == channelConfirmLink {
		rec.source.identityCheck = identityConfirmLink
	}
	if rec.source.requesterEmail, err = requesterAddress(nil, rec.mailedTo, rec.subjectRef); err != nil {
		return false, err
	}
	proof := requestProof{
		requestID: id, source: rec.source, kind: rec.kind, result: rec.status,
		erased: erased, countsKnown: known, answeredAt: rec.updatedAt, backfillSource: proofBackfillSource,
		completedByType: rec.closerType, completedByID: rec.closerID,
	}
	if rec.closedAt != nil {
		proof.answeredAt = *rec.closedAt
	}
	return insertRequestProof(ctx, tx, proof)
}

// recordedErasure reads what a fulfilled erasure removed from the tombstone
// the erasure engine wrote for this request. It finds that tombstone by its
// reason "dsr:<request-id>", since the case's own reference may be gone.
func recordedErasure(ctx context.Context, tx pgx.Tx, id ids.UUID, rec closedRequestRecord, seams ProofBackfillSeams) (erasureaccount.Account, bool, error) {
	if rec.kind != dsrKindErasure || rec.status != dsrStatusFulfilled {
		return erasureaccount.Empty(), true, nil
	}
	account, found, err := seams.Erasure(ctx, tx, "dsr:"+id.String())
	if err != nil || !found {
		return erasureaccount.Empty(), false, err
	}
	return account, true, nil
}
