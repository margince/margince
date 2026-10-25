// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The end of a proof's life. The January run deletes each proof past its date,
// and what the request still holds of the requester.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// proofExpiryCause is the evidence both expiry audit rows carry.
const proofExpiryCause = "privacy_request_proof_expired"

// ExpireRequestProofs deletes up to limit proofs due by now, and answers how
// many. It acts only in January, so one year's proofs go in one run. A proof
// whose request has an open dispute waits for the next January.
//
// The request loses subject_ref, its prose and its contact link. audit_log is
// append-only, so the anonymize row hides the creation image on every read.
func (s *Store) ExpireRequestProofs(ctx context.Context, now time.Time, limit int) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	if now.Month() != time.January {
		return 0, nil
	}
	var expired int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		due, err := lockExpiredProofs(ctx, tx, now, limit)
		if err != nil {
			return err
		}
		for _, proof := range due {
			if err := expireRequestProof(ctx, tx, proof); err != nil {
				return err
			}
		}
		expired = len(due)
		return nil
	})
	return expired, err
}

// expiredProof names one proof the run deletes and the request it proves.
type expiredProof struct {
	id        ids.UUID
	requestID ids.UUID
}

// lockExpiredProofs locks the due proofs and their requests, so a dispute
// opened while the run reads cannot be overtaken by the delete.
func lockExpiredProofs(ctx context.Context, tx pgx.Tx, now time.Time, limit int) ([]expiredProof, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.request_id
		  FROM privacy_request_proof p
		  JOIN data_subject_request d ON d.id = p.request_id
		 WHERE p.keep_until <= $1 AND NOT d.dispute_open
		 ORDER BY p.keep_until, p.id
		 LIMIT $2
		   FOR UPDATE OF p, d`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []expiredProof
	for rows.Next() {
		var proof expiredProof
		if err := rows.Scan(&proof.id, &proof.requestID); err != nil {
			return nil, err
		}
		out = append(out, proof)
	}
	return out, rows.Err()
}

func expireRequestProof(ctx context.Context, tx pgx.Tx, proof expiredProof) error {
	if _, err := tx.Exec(ctx, `DELETE FROM privacy_request_proof WHERE id = $1`, proof.id); err != nil {
		return err
	}
	if _, err := storekit.AuditWithEvidence(ctx, tx, "expire", proofEntity, proof.id, nil, nil,
		map[string]any{"cause": proofExpiryCause, "request_id": proof.requestID}); err != nil {
		return err
	}
	// The same tombstone the erasure engine writes on a subject's other cases,
	// so a reader meets one spelling of a request whose subject is gone.
	tag, err := tx.Exec(ctx, `
		UPDATE data_subject_request
		   SET subject_ref = 'erased+' || id,
		       resolution = CASE WHEN resolution IS NULL THEN NULL ELSE '`+erasedResolution+`' END,
		       contact_id = NULL
		 WHERE id = $1 AND NOT dispute_open`, proof.requestID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("consent: the request %s behind an expired proof changed while it was locked", proof.requestID)
	}
	_, err = storekit.AuditWithEvidence(ctx, tx, "anonymize", "data_subject_request", proof.requestID, nil, nil,
		map[string]any{"cause": proofExpiryCause})
	return err
}
