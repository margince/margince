// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package backfillwire maps what a mail history run reports about its own
// course onto the status the clients read. A restarted or retried scan can then
// be told apart from one that lost its work.
package backfillwire

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// Course sets the status fields that describe how the run got to its counts.
// They say whether it can continue, how often its scan began again, and how
// many pages in a row failed.
func Course(st *crmcontracts.BackfillStatus, run *capture.BackfillRun) {
	st.Resumable = &run.Resumable
	st.WindowRestarts = &run.WindowRestarts
	st.RetryAttempts = &run.RetryAttempts
}
