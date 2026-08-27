// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What is waiting for one digest reader, counted the way the surfaces that
// serve those queues count it.
//
// Both numbers name records whose visibility is per-reader: a duplicate pair is
// shown only to someone who can open BOTH sides, and a staged proposal only to
// someone who could decide it. The digest used to count each with a raw
// workspace-wide subquery and write the same number into every reader's
// payload, which told everyone how many records exist whether or not they could
// open one.
//
// Neither count is computed here. attentionApprovals and attentionDuplicates
// already answer these two questions for the Today surface, under the caller's
// own authority, and a second implementation would drift from the queues the
// numbers are supposed to describe — a digest promising nine duplicates over a
// page that shows two.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// newDigestReviewSource binds the two readers the nightly build asks.
//
// capture calls this once per reader with that reader bound as the acting
// principal, so both counts resolve through their live grants and row scope
// without either seam needing to know it is serving a digest.
func newDigestReviewSource(pool *pgxpool.Pool, svc *approvals.Service) capture.DigestReviewSource {
	duplicates := attentionDuplicates{store: people.NewStore(InstallationDB(pool))}
	pending := attentionApprovals{svc: svc}
	return func(ctx context.Context) (capture.DigestReviewCounts, error) {
		open, err := countOrNoneVisible(duplicates.CountOpen(ctx))
		if err != nil {
			return capture.DigestReviewCounts{}, err
		}
		waiting, err := countOrNoneVisible(pending.CountPending(ctx))
		if err != nil {
			return capture.DigestReviewCounts{}, err
		}
		return capture.DigestReviewCounts{DedupeOpen: open, ApprovalsPending: waiting}, nil
	}
}

// countOrNoneVisible reads a refusal as an empty queue rather than a fault.
//
// A store refuses a reader who holds no grant for the records it counts, which
// is the right answer to "show me these" and the wrong one to "how many are
// waiting for you". None are: the queue that surfaces them would be empty for
// this reader too. Propagating the refusal would fail the whole nightly build
// for that reader — one seat without a person grant would cost a colleague
// their digest — which is the same reasoning the projects section states for
// answering no section instead of an error.
func countOrNoneVisible(count int, err error) (int, error) {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}
