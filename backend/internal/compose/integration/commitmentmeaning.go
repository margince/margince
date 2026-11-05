// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// OpenCommitmentMeaning preserves an explicit deadline without inventing one.
func OpenCommitmentMeaning(due, words string) *crmcontracts.CommitmentMeaning {
	meaning := &crmcontracts.CommitmentMeaning{State: "open", DeadlinePrecision: "none"}
	if due != "" {
		meaning.DeadlinePrecision, meaning.DeadlineQuote = "day", words
	}
	return meaning
}
