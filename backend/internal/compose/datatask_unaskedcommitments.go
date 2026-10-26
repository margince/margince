// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// unaskedCommitmentsDataTask closes the commitment proposals staged before a
// proposal had to name the colleague who made the promise. Such a row was
// shared with every decider, or put to the mailbox or deal owner instead.
func unaskedCommitmentsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792645467_unasked_commitments_withdrawn", Version: 1,
		Title: "Withdrawing promise requests sent to the wrong colleague", Requires: []string{"core/1792562859"},
		Steps: []datatask.Step{{
			Name: "withdraw_unasked_commitments", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: approvals.EstimateUnaskedCommitments, Batch: approvals.WithdrawUnaskedCommitmentsTx,
		}},
	}
}
