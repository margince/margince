// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The boot step that records what this binary composed. Both halves are facts
// derived from the same registered extension set, so they are one step rather
// than two a role could wire half of.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/extensionprivacy"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/ports/companytypes"
	"github.com/margince/margince/backend/pkg/extension"
)

// RecordComposition writes the boot facts of the composed set. They are its
// inventory, the channel vocabulary its units declare, their starting role
// grants, and the service member of each unit that lands records.
//
// One function because the ordering between them is not a caller's to choose.
// The reconcile reads the composed set and refuses a unit that shadows a core
// transport. So it belongs after the set is observed and before anything serves. A role that wired only the inventory would boot with units installed
// and their transports unregistered. This step exists to make that state
// unreachable.
//
// Every process role calls it, after RegisterExtensions and before it assembles
// anything. Server assembly loads the transport directory from the rows written
// here.
func RecordComposition(
	ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, composed *Extensions,
) error {
	exts := composed.Units()
	if err := ObserveExtensionInventory(ctx, pool, log, exts); err != nil {
		return fmt.Errorf("compose: recording the composed extension set: %w", err)
	}
	if err := VerifyExtensionPrivacy(ctx, pool, exts); err != nil {
		return fmt.Errorf("compose: checking the extensions' privacy declarations: %w", err)
	}
	if err := ReconcileChannelProviders(ctx, pool, composed); err != nil {
		return fmt.Errorf("compose: registering this binary's channel vocabulary: %w", err)
	}
	if err := FillExtensionRbacDefaults(ctx, pool, log); err != nil {
		return fmt.Errorf("compose: filling the units' default role grants: %w", err)
	}
	if err := EnsureExtensionServiceMembers(ctx, pool, log, exts); err != nil {
		return fmt.Errorf("compose: making the units' service members: %w", err)
	}
	return nil
}

// extensionData holds the composed units the privacy registry is derived from.
// RegisterExtensions installs it in its apply phase, beside corereach.Install,
// and the mutex orders that write against every later read.
var extensionData struct {
	mu    sync.RWMutex
	units []extension.Extension
}

func installExtensionData(units []extension.Extension) {
	extensionData.mu.Lock()
	defer extensionData.mu.Unlock()
	extensionData.units = units
}

// ExtensionData is the registry every eraser, export and merge store outside a test is handed.
func ExtensionData() extprivacy.Registry {
	extensionData.mu.RLock()
	defer extensionData.mu.RUnlock()
	return extensionprivacy.Registry(extensionData.units)
}

// installedCompanyTypes is the company types the installed units declare, for
// the record provider, which is built where no composition is in reach.
func installedCompanyTypes() companytypes.Registry {
	extensionData.mu.RLock()
	defer extensionData.mu.RUnlock()
	return companytypes.Of(extensionData.units)
}

// VerifyExtensionPrivacy checks the units' declarations at boot (extensionprivacy.Verify).
func VerifyExtensionPrivacy(ctx context.Context, pool *pgxpool.Pool, exts []extension.Extension) error {
	return extensionprivacy.Verify(ctx, pool, exts, extensionprivacy.Ledger{
		Scope: func(ctx context.Context) (context.Context, bool, error) {
			return bootLedgerScope(ctx, pool, "system:extension-privacy")
		},
		Lock: bootLedgerLock, Installation: installationMarker,
	})
}
