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
	"github.com/margince/margince/backend/internal/platform/auth"
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

// offeredRecordsLive answers, by position, whether each offer put to p may
// still be decided: the record is live, and p may still receive it. That is a
// live human seat that can write (auth.EligibleHandOverRecipients) and a role
// that may read and change the record's type. One statement asks p's seat, and
// one more each record type.
func offeredRecordsLive(ctx context.Context, tx pgx.Tx, p principal.Principal, refs []targetRef) ([]bool, error) {
	out := make([]bool, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	seated, err := auth.EligibleHandOverRecipients(ctx, tx, []ids.UUID{p.UserID})
	if err != nil {
		return nil, fmt.Errorf("crmapprovals: asking whether the receiver may still take records: %w", err)
	}
	if !seated[p.UserID] {
		return out, nil
	}
	byType := map[string][]int{}
	for i, ref := range refs {
		if ref.Type == nil || ref.ID == nil {
			continue
		}
		if _, ok := offeredRecordQueries[*ref.Type]; !ok {
			continue
		}
		if p.Permissions.Allows(*ref.Type, principal.ActionRead) && p.Permissions.Allows(*ref.Type, principal.ActionUpdate) {
			byType[*ref.Type] = append(byType[*ref.Type], i)
		}
	}
	for recordType, at := range byType {
		rowIDs := make([]ids.UUID, len(at))
		for j, i := range at {
			rowIDs[j] = *refs[i].ID
		}
		live, err := idsAmong(ctx, tx, offeredRecordQueries[recordType], distinct(rowIDs))
		if err != nil {
			return nil, fmt.Errorf("crmapprovals: reading offered records: %w", err)
		}
		for _, i := range at {
			out[i] = live[*refs[i].ID]
		}
	}
	return out, nil
}
