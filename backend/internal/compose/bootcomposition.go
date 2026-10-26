// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The boot step that records what this binary composed and installs what is new.
// The facts derive from the same registered extension set, so they are one step
// rather than several a role could wire part of.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/pkg/extension"
)

// RecordComposition writes the boot facts of the composed set: its inventory,
// the channel vocabulary its units declare and their starting role grants.
//
// One function because the ordering between them is not a caller's to choose —
// the reconcile reads ComposedExtensions() and refuses a unit that shadows a
// core transport, so it belongs after the set is observed and before anything
// serves. A role that wired only the inventory would boot with units installed
// and their transports unregistered, which is the state this step exists to
// make unreachable.
//
// Every process role calls it, after RegisterExtensions and before it assembles
// anything: server assembly loads the transport directory FROM the rows written
// here.
func RecordComposition(
	ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, log *slog.Logger, exts []extension.Extension,
) error {
	if err := ObserveExtensionInventory(ctx, pool, log, exts); err != nil {
		return fmt.Errorf("compose: recording the composed extension set: %w", err)
	}
	if err := VerifyExtensionPrivacy(ctx, pool, exts); err != nil {
		return fmt.Errorf("compose: checking the extensions' privacy declarations: %w", err)
	}
	if err := ActivateExtensions(ctx, pool, vault, log, exts, ComposedVerbs()); err != nil {
		return err
	}
	if err := ReconcileChannelProviders(ctx, pool); err != nil {
		return fmt.Errorf("compose: registering this binary's channel vocabulary: %w", err)
	}
	if err := FillExtensionRbacDefaults(ctx, pool, log); err != nil {
		return fmt.Errorf("compose: filling the units' default role grants: %w", err)
	}
	return nil
}
