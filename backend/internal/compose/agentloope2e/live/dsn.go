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
	hosts := []string{cfg.Host}
	for _, fallback := range cfg.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	for _, host := range hosts {
		if !isLocalHost(host) {
			return fmt.Errorf("live: the database host %q is not this machine; the lane writes workspaces and runs only on a local database", host)
		}
	}
	return nil
}

// isLocalHost is true for a socket directory, localhost or a loopback address.
func isLocalHost(host string) bool {
	if host == "" {
		return false
	}
	if host[0] == '/' || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
