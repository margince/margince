// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type redecodableMailText struct {
	ID           ids.UUID
	Version      int64
	Subject      *string
	Body         string
	Completeness string
	Payload      []byte
}

// redecodableMailTextScope is captured mail whose stored original survives and
// whose text could have been written without reading the charset.
//
// The two text tests are a superset of what the repair accepts, so the scope
// drops no row it would have corrected. A subject it rewrites still carries an
// encoded-word marker, and a body it rewrites holds a question mark where a
// non-ASCII rune belongs. Mail with neither cannot be the shape.
const redecodableMailTextScope = `FROM activity a ` + mailOriginalJoin + `
 WHERE a.kind = 'email' AND a.restricted_at IS NULL AND a.archived_at IS NULL
   AND a.body IS NOT NULL AND rc.payload IS NOT NULL
   AND (a.subject LIKE '%=?%' OR a.body LIKE '%?%')
   AND ` + capturedByMailConnector

// redecodeMailTextBatch inspects a page of stored mail and rewrites the text a
// parser wrote before it read the message's charset.
func redecodeMailTextBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	return pageStoredMail(ctx, tx, in,
		"a.id, a.version, a.subject, a.body, a.body_completeness, rc.payload",
		redecodableMailTextScope,
		func(c redecodableMailText) ids.UUID { return c.ID },
		redecodeMailText)
}

// redecodeMailText rewrites one row's text from its stored original. The
// completeness it reads is the completeness it writes: a charset is not a
// length, so nothing about how whole the body is changes here.
func redecodeMailText(ctx context.Context, tx pgx.Tx, candidate redecodableMailText) (bool, error) {
	raw, readable := decodedMailOriginal(candidate.Payload)
	if !readable {
		return false, nil
	}
	stored := ""
	if candidate.Subject != nil {
		stored = *candidate.Subject
	}
	subject, body, ok := mailmap.RecoverMisdecodedText(raw, stored, candidate.Body)
	if !ok {
		return false, nil
	}
	// Passed only when it changed, so a row whose body alone was wrong keeps
	// the subject guard it would have had.
	var corrected *string
	if subject != stored {
		corrected = &subject
	}
	rewritten, threadKey, err := activities.RestoreMailBodyTx(ctx, tx, activities.MailBodyRestoration{
		Activity: candidate.ID, Version: candidate.Version, StoredBody: candidate.Body,
		Body: body, Completeness: candidate.Completeness, Subject: corrected,
	})
	if err != nil || !rewritten || threadKey == nil {
		return rewritten, err
	}
	if err := markConversationDueTx(ctx, tx, *threadKey); err != nil {
		return false, fmt.Errorf("mail text redecode: %w", err)
	}
	return true, nil
}

// estimateRedecodableMailText counts the rows left to inspect, not the rows
// left to correct. That is why this step is not EmptyWhenDone. A finished pass
// still has rows in scope, which is the pass having read them.
func estimateRedecodableMailText(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	return countStoredMail(ctx, q, redecodableMailTextScope, maximum)
}
