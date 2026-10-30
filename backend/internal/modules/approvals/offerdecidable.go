// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The one decision a member takes on a record they cannot change yet: the
// colleague a record is offered to accepts or declines it. The write authority
// every other decision spends is the giver's, and the accept re-checks it as
// the giver under the record's row lock.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offeredRecordQueries answer which offered records still exist and are live.
var offeredRecordQueries = map[string]string{
	tableContact: `SELECT id FROM contact WHERE id = ANY($1) AND archived_at IS NULL`,
	tableCompany: `SELECT id FROM company WHERE id = ANY($1) AND archived_at IS NULL`,
}

// offeredToReader reports whether a is an ownership offer put to p. Only the
// receiver the payload names passes; the giver, a manager and any other kind
// keep the write check on the target.
func offeredToReader(p principal.Principal, a row) bool {
	if a.Kind != approvalsubject.KindOwnershipTransfer || p.UserID.IsZero() {
		return false
	}
	return approvalsubject.OwnershipTransferOfferedTo(a.ProposedChange) == p.UserID
}

// offeredRecordsLive answers, by position, whether each offered record is
// still live.
func offeredRecordsLive(ctx context.Context, tx pgx.Tx, refs []targetRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		if ref.Type == nil || ref.ID == nil {
			continue
		}
		query, ok := offeredRecordQueries[*ref.Type]
		if !ok {
			continue
		}
		live, err := idsAmong(ctx, tx, query, []ids.UUID{*ref.ID})
		if err != nil {
			return nil, fmt.Errorf("crmapprovals: reading an offered record: %w", err)
		}
		out[i] = live[*ref.ID]
	}
	return out, nil
}
