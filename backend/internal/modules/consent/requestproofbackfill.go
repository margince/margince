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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
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

// BackfillRequestProofsTx gives up to limit closed requests after `after` a
// proof, inside the data task's transaction. A request that has one keeps it,
// so a replay from the start writes nothing.
func BackfillRequestProofsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (ProofBackfillPage, error) {
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
	for _, id := range page.Scanned {
		wrote, err := backfillRequestProof(ctx, tx, id)
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

func backfillRequestProof(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	var rec closedRequestRecord
	// The closing audit row is the first update that left the request in the
	// status it holds now.
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
		         ORDER BY a.occurred_at, a.id LIMIT 1) closing ON true
		 WHERE d.id = $1
		   FOR UPDATE OF d`, id,
	).Scan(&rec.kind, &rec.status, &rec.subjectRef, &rec.source.requestNumber, &rec.source.receivedAt,
		&rec.channel, &rec.updatedAt, &rec.mailedTo, &rec.closerType, &rec.closerID, &rec.closedAt)
	if err != nil {
		return false, fmt.Errorf("consent: reading a closed request for its proof: %w", err)
	}
	erased, known, err := recordedErasure(ctx, tx, id, rec)
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
		erasedCounts: erased, countsKnown: known, answeredAt: rec.updatedAt, backfillSource: proofBackfillSource,
		completedByType: rec.closerType, completedByID: rec.closerID,
	}
	if rec.closedAt != nil {
		proof.answeredAt = *rec.closedAt
	}
	return insertRequestProof(ctx, tx, proof)
}

// recordedErasure reads what a fulfilled erasure removed from the tombstone the
// erasure engine wrote for this request. The tombstone sits on the contact the
// request names, which only an unretired subject_ref still holds.
func recordedErasure(ctx context.Context, tx pgx.Tx, id ids.UUID, rec closedRequestRecord) (map[string]int64, bool, error) {
	if rec.kind != dsrKindErasure || rec.status != dsrStatusFulfilled {
		return map[string]int64{}, true, nil
	}
	contactID, resolved := resolveDSRSubject(dsrRow{SubjectRef: rec.subjectRef})
	if !resolved {
		return map[string]int64{}, false, nil
	}
	var evidence []byte
	err := tx.QueryRow(ctx, `
		SELECT evidence FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'erase'
		   AND evidence->>'reason' = $2
		 ORDER BY occurred_at, id LIMIT 1`, contactID, "dsr:"+id.String()).Scan(&evidence)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]int64{}, false, nil
		}
		return nil, false, fmt.Errorf("consent: reading the erasure tombstone of a closed request: %w", err)
	}
	return erasureCounts(evidence)
}

// erasureCounts keeps the whole-number entries of a tombstone's evidence, which
// are its counts; the reason and any other text are not categories.
func erasureCounts(evidence []byte) (map[string]int64, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(evidence))
	decoder.UseNumber()
	var entries map[string]any
	if err := decoder.Decode(&entries); err != nil {
		return nil, false, fmt.Errorf("consent: reading an erasure tombstone's counts: %w", err)
	}
	out := map[string]int64{}
	for category, entry := range entries {
		number, isNumber := entry.(json.Number)
		if !isNumber {
			continue
		}
		if count, err := number.Int64(); err == nil {
			out[category] = count
		}
	}
	return out, true, nil
}
