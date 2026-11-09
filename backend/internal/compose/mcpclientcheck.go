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

// clientCheckGate offers a Gated tool only to a passport whose human is a
// workspace admin, and only while the experiment is on. The admin check comes
// first: a non-admin's tools/list and call read no setting, so neither its
// latency nor its answer depends on the switch.
func clientCheckGate(pool *pgxpool.Pool, log *slog.Logger) agents.ToolGate {
	if pool == nil {
		return func(context.Context) bool { return false }
	}
	cfg := StoredConfig(pool)
	return adminsWhileOn(func(ctx context.Context) (bool, error) { return identity.ReadClientCheck(ctx, cfg) }, log)
}

// adminsWhileOn is the gate's rule over whatever reads the switch.
func adminsWhileOn(switchOn func(context.Context) (bool, error), log *slog.Logger) agents.ToolGate {
	return func(ctx context.Context) bool {
		if !auth.AdminGranted(ctx) {
			return false
		}
		on, err := switchOn(ctx)
		if err != nil {
			log.Warn("mcp: reading the client check switch failed; the check is not offered", "err", err)
			return false
		}
		return on
	}
}
