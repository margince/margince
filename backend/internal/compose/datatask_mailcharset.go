// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// charsetDamagedMailScope is captured mail whose stored original survives and
// whose subject still holds an encoded word, or whose body holds a stand-in
// character. Mailmap decides per row, from that original, whether the stored
// text has the shape the older parser left. The filter only keeps the page off
// mail that cannot be damaged.
const charsetDamagedMailScope = `FROM activity a ` + mailOriginalJoin + `
 WHERE a.kind = 'email' AND a.restricted_at IS NULL AND a.archived_at IS NULL AND a.body IS NOT NULL
   AND rc.payload IS NOT NULL AND ` + capturedByMailConnector + `
   AND (position('=?' in a.subject) > 0 OR position('?' in a.body) > 0 OR position(chr(65533) in a.body) > 0)`

type charsetCandidate struct {
	ID           ids.UUID
	Version      int64
	Subject      *string
	Body         string
	Completeness string
	Payload      []byte
}

func repairMailCharsetsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.version, a.subject, a.body, a.body_completeness, rc.payload `+charsetDamagedMailScope+`
		AND a.id > @charset_after ORDER BY a.id LIMIT @charset_limit FOR UPDATE OF a`,
		pgx.NamedArgs{"charset_after": after, "charset_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[charsetCandidate])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	changed := 0
	for _, candidate := range candidates {
		scanned = append(scanned, candidate.ID)
		repaired, err := repairMailCharset(ctx, tx, candidate)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if repaired {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// repairMailCharset writes the subject and body the stored original yields now.
// It does so for text in the shape the parser left before it decoded legacy
// charsets. A mail the original cannot vouch for is left alone.
func repairMailCharset(ctx context.Context, tx pgx.Tx, candidate charsetCandidate) (bool, error) {
	raw, readable := decodedMailOriginal(candidate.Payload)
	if !readable {
		return false, nil
	}
	storedSubject := ""
	if candidate.Subject != nil {
		storedSubject = *candidate.Subject
	}
	recovery, ok := mailmap.RecoverCharsetDamage(raw, storedSubject, candidate.Body)
	if !ok {
		return false, nil
	}
	completeness := candidate.Completeness
	if recovery.BodyChanged {
		completeness = string(recovery.Completeness)
	}
	return writeMailRestoration(ctx, tx, activities.MailBodyRestoration{
		Activity: candidate.ID, Version: candidate.Version, StoredBody: candidate.Body,
		Body: recovery.Body, Completeness: completeness,
		Subject: recovery.Subject, StoredSubject: storedSubject,
	})
}

func estimateCharsetDamagedMail(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+charsetDamagedMailScope+" LIMIT @charset_cap) candidates",
		pgx.NamedArgs{"charset_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
