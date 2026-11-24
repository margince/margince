// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// RequireLocalDSN refuses a database that is not on this machine. The lane
// seeds and writes workspaces, and it must never do that on a shared server.
func RequireLocalDSN(dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return errors.New("live: no database address is set, so the lane cannot tell where it would write")
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("live: the database address is unreadable: %w", err)
	}
	if cfg.Host == "" {
		return errors.New("live: the database address names no host")
	}
	if cfg.Host[0] == '/' || cfg.Host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(cfg.Host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("live: the database host %q is not this machine; the lane writes workspaces and runs only on a local database", cfg.Host)
}
