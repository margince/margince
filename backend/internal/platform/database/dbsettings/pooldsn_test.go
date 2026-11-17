// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbsettings_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
)

// The pool the api and the worker open retires its connections at the
// configured ages, and an age the DSN names itself stays the DSN's.
func TestThePoolRetiresConnectionsAtTheConfiguredAges(t *testing.T) {
	configured := []string{"MARGINCE_DATABASE_CONNECTION_LIFETIME=30m", "MARGINCE_DATABASE_CONNECTION_IDLE_TIME=2m"}
	for _, tc := range []struct {
		name, dsn              string
		env                    []string
		lifetime, idle, jitter time.Duration
	}{
		{"the defaults", "postgres://h/db", nil, 10 * time.Minute, 5 * time.Minute, 2 * time.Minute},
		{"a configured lifetime and idle time", "postgres://h/db",
			configured, 30 * time.Minute, 2 * time.Minute, 6 * time.Minute},
		{"the DSN's own values win", "host=h pool_max_conn_lifetime=1h pool_max_conn_idle_time=90s",
			configured, time.Hour, 90 * time.Second, 12 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configtest.MustLoadEnv(t, dbsettings.Settings(), tc.env...)
			pool, err := database.PoolConfig(dbsettings.PoolDSN(tc.dsn, cfg))
			if err != nil {
				t.Fatal(err)
			}
			if pool.MaxConnLifetime != tc.lifetime || pool.MaxConnIdleTime != tc.idle || pool.MaxConnLifetimeJitter != tc.jitter {
				t.Fatalf("lifetime %v idle %v jitter %v, want %v, %v and %v",
					pool.MaxConnLifetime, pool.MaxConnIdleTime, pool.MaxConnLifetimeJitter, tc.lifetime, tc.idle, tc.jitter)
			}
		})
	}
}

func TestAConnectionLifetimeUnderAMinuteIsRefused(t *testing.T) {
	if _, err := configtest.Load(t, "database:\n  connection_lifetime: 10s\n", nil, nil, dbsettings.Settings()...); err == nil {
		t.Fatal("accepted a connection lifetime of ten seconds, which would replace every backend several times a minute")
	}
}
