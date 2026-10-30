// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// clientCheckGate offers a Gated tool only while an admin has turned the
// experiment on and only to a passport whose human is a workspace admin. The
// switch is read first, so an installation that never turned it on pays no
// permission check.
func clientCheckGate(pool *pgxpool.Pool, log *slog.Logger) agents.ToolGate {
	if pool == nil {
		return func(context.Context) bool { return false }
	}
	cfg := StoredConfig(pool)
	return func(ctx context.Context) bool {
		on, err := identity.ReadClientCheck(ctx, cfg)
		if err != nil {
			log.Warn("mcp: reading the client check switch failed; the check is not offered", "err", err)
			return false
		}
		return on && auth.RequireAdmin(ctx) == nil
	}
}
