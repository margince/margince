// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's jurisdiction at boot. The deployment file seeds it once;
// after that the stored value decides, and boot refuses one this build carries
// no pack for.

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// configuredCountry is the file's country for a fresh bootstrap, or nil when
// the file names none, so an omitted key seeds nothing and the screen can
// still choose.
func configuredCountry(cfg deployconfig.Config) *string {
	if cfg.Workspace.Country == "" {
		return nil
	}
	code := cfg.Workspace.Country
	return &code
}

// seedConfiguredCountry gives an existing installation the file's country when
// it has none stored. A stored value is kept and the file's is reported as
// discarded, the same warning the other identity seeds give.
func seedConfiguredCountry(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, cfg deployconfig.Config) error {
	if cfg.Workspace.Country == "" {
		return nil
	}
	stored, err := identity.SeedCountryOnce(ctx, pool, cfg.Workspace.Country)
	if err != nil {
		return fmt.Errorf("compose: seeding workspace.country: %w", err)
	}
	if stored {
		log.Info("installation jurisdiction seeded from margince.yaml", "country", cfg.Workspace.Country)
	}
	return nil
}

// RequireCompiledInCountry refuses the boot when the stored jurisdiction names
// a pack this build does not carry. It reads the STORED value, after seeding,
// because a pack dropped from a later build would otherwise leave the
// installation selecting a jurisdiction that applies nothing. Both the api and
// the worker call it, after RegisterExtensions.
func RequireCompiledInCountry(ctx context.Context, pool *pgxpool.Pool) error {
	code, err := identity.StoredCountry(ctx, pool)
	if err != nil {
		return fmt.Errorf("compose: reading the installation's jurisdiction: %w", err)
	}
	if code == "" {
		return nil
	}
	if _, ok := jurisdiction.For(code); ok {
		return nil
	}
	return fmt.Errorf("compose: the installation's jurisdiction is %q, but this build carries no pack for it (compiled in: %v); "+
		"build with that pack, or change the jurisdiction from a build that carries it", code, compiledInCodes())
}

// compiledInCodes lists the pack codes this build carries, sorted.
func compiledInCodes() []string {
	codes := []string{}
	for _, p := range jurisdiction.Applicable() {
		codes = append(codes, string(p.Code()))
	}
	slices.Sort(codes)
	return codes
}
