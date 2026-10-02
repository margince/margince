// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Shared fixtures for the ranked worklist.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func leadReader() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.MustParse("01a05500-0000-7000-8000-000000000001"),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

func kindsOf(row crmcontracts.WorklistItem) []string {
	out := make([]string, 0, len(row.Because))
	for _, because := range row.Because {
		out = append(out, string(because.Kind))
	}
	return out
}
