// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"github.com/margince/margince/backend/internal/shared/kernel/threadverdict"
)

// WorkspaceAudienceSQL applies settled holds before bounded audience reconciliation.
// Explicit internal-mail authorization keeps its independent admission policy.
func WorkspaceAudienceSQL(alias string) string { return threadverdict.WorkspaceAudienceSQL(alias) }
