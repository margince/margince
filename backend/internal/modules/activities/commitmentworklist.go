// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// CurrentCommitmentTaskClause admits current automatic promises and human work
// in record task reads; alias names a compile-time activity alias, never request input.
func CurrentCommitmentTaskClause(alias string, asOf time.Time, arg func(any) int) string {
	return storekit.CurrentCommitmentTaskClause(alias, asOf, arg)
}
