// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// dsnSettings are the two credentials, on a flag set that also holds
// migrate's own flags, which Load does not know.
type dsnSettings struct {
	reg   *config.Registry
	given *config.CommandFlags
}

// addDSNFlags registers --dsn without a default: one read from the
// environment would reach the usage text, which on CI is a public log.
func addDSNFlags(fs *flag.FlagSet) (*dsnSettings, error) {
	reg, err := compose.ProgramRegistry(config.RoleMigrate)
	if err != nil {
		return nil, err
	}
	return &dsnSettings{reg: reg, given: config.AddFlags(fs, reg)}, nil
}

// resolve runs after the flag set parsed, so --dsn still beats environ.
func (d *dsnSettings) resolve(environ []string) (string, error) {
	loaded, err := config.Load(d.reg, config.Sources{
		Args:           d.given.Args(),
		Environ:        environ,
		KnownKey:       compose.AllSettingKeys,
		KnownEnv:       compose.AllSettingEnvs,
		BootstrapRoots: deployconfig.Roots(),
	})
	if err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}
	return ownerDSN(loaded)
}

// ownerDSN resolves which credential migrates: an explicit --dsn, else
// MARGINCE_OWNER_DSN, else MARGINCE_DSN.
//
// The order is the invariant. Every verb here runs DDL and so needs the owner
// role; MARGINCE_DSN names the APP role everywhere else in the product
// (NOSUPERUSER, NOBYPASSRLS), which holds no DDL rights. It remains the last
// resort because an installation may run everything under one
// sufficiently-privileged credential.
func ownerDSN(loaded *config.Config) (string, error) {
	dsn, origin := config.ValWithSource(loaded, dbsettings.DSN)
	if !origin.FromFlag() {
		dsn = cmp.Or(config.Val(loaded, dbsettings.OwnerDSN), dsn)
	} else if dsn == "" {
		// An explicit `--dsn ""` is refused, not read as absent. A wrapper passing
		// --dsn "$UNSET_VAR" would otherwise run `drop-db` on the ambient DSN.
		return "", errors.New("migrate: --dsn was given but empty; pass a DSN or omit the flag to use MARGINCE_OWNER_DSN")
	}
	if dsn == "" {
		return "", errors.New("migrate: --dsn, MARGINCE_OWNER_DSN or MARGINCE_DSN required")
	}
	return dsn, nil
}
