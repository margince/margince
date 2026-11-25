// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/composition"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// registerAndInstrument registers the composed extension set before anything
// serves, so a failing registration aborts the boot, and starts instrumentation.
// One snapshot serves registration and the boot inventory, so both observe the
// same declarations.
func registerAndInstrument(ctx context.Context, cfg apiConfig, stdout io.Writer) (*compose.Extensions, error) {
	composed, err := registerExtensions(composition.Extensions())
	if err != nil {
		return nil, err
	}
	if err := startInstrumentation(ctx, composed, cfg, stdout); err != nil {
		return nil, err
	}
	return composed, nil
}

// openBoundInstallation opens the installation and binds the extension runtime
// to it. The runtime is bound whether or not a custodian exists: a nil vault is
// the ErrNoCustodian posture the secret store documents. Objects bind before
// the boot records the composed set, because removing a unit sweeps its objects.
func openBoundInstallation(ctx context.Context, cfg apiConfig, composed *compose.Extensions) (*pgxpool.Pool, keyvault.Vault, error) {
	pool, vault, err := openInstallation(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	composed.BindRuntime(pool, vault)
	if err := composed.BindObjectsFromConfig(ctx, cfg.settings); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return pool, vault, nil
}
