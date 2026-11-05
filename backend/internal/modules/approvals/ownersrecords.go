// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withoutOwnersRecords takes the mailbox owner's record ids out of each
// approval handed to the reader (approvalsubject.WithoutOwnersRecords). It runs
// after the decision context is read, because the card's preview checks those
// records as the reader that filed them.
func withoutOwnersRecords(ctx context.Context, out []crmcontracts.Approval) {
	reader := ids.Nil
	if actor, ok := principal.Actor(ctx); ok {
		reader = actor.UserID
	}
	for i := range out {
		if out[i].ProposedChange == nil {
			continue
		}
		change := approvalsubject.WithoutOwnersRecords(out[i].Kind, *out[i].ProposedChange, reader)
		out[i].ProposedChange = &change
	}
}
