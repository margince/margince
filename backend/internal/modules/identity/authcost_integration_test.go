// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

// What one sign-in costs once the session stamp is fresh: begin, session, the
// batch of roles and teams, both settings, and commit. Fewer means a read was skipped.
const (
	authenticateStatementCeiling = 6
	authenticateRoundTrips       = 5
)

// meteredIdentityPool is a pool on the same database whose connections count
// protocol round trips and leave nothing running between requests.
func meteredIdentityPool(t *testing.T, base *pgxpool.Pool) (*pgxpool.Pool, func() int64) {
	t.Helper()
	cfg := base.Config()
	cfg.ShouldPing = func(context.Context, pgxpool.ShouldPingParams) bool { return false }
	cfg.MaxConnLifetime = 0
	cfg.MaxConnIdleTime = time.Hour
	cfg.HealthCheckPeriod = time.Hour
	cfg.ConnConfig.TLSConfig = nil
	cfg.ConnConfig.Fallbacks = nil
	trips := testdb.CountRoundTrips(t, cfg)
	pool, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, trips.Load
}

func TestAuthenticateStaysWithinItsStatementAndRoundTripBudgets(t *testing.T) {
	e := setupRevocationEnv(t, "auth-cost")
	// Every real seat holds a role, and a role with a mask is the costliest.
	role := e.customRole(t, "Masked sales", "rep", nil)
	e.maskRole(t, role)
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, role); err != nil {
		t.Fatal(err)
	}
	provider, err := e.svc.LoginViaFederatedIdentity(e.wsOnlyCtx(), "google",
		OIDCClaims{Subject: "auth-cost-member", Email: e.member.Email})
	if err != nil {
		t.Fatal(err)
	}
	sessions := map[string]string{"password": e.signIn(t, "device"), "provider": provider}
	pool, trips := meteredIdentityPool(t, e.pool)
	db := database.BindTo(pool, e.ws)
	cfg := unlockedConfig(t, pool)
	e.svc.db = db
	e.svc.WithConfig(cfg).WithVault(keyvault.NewMemory())
	e.svc.WithAdmissionSettings(NewInstallationSettings(db, settings.New(pool, settings.NewRegistry(append(Definitions(), cfg.StoredDefinitions()...)...)), cfg).AdmissionSettings)

	for kind, token := range sessions {
		t.Run(kind, func(t *testing.T) {
			// The first request warms the connection and, with the stamp fresh
			// from sign-in, writes nothing; the measured one is the steady state.
			if _, err := e.svc.Authenticate(e.wsOnlyCtx(), token); err != nil {
				t.Fatal(err)
			}
			ctx, meter := database.WithStatementMeter(e.wsOnlyCtx())
			before := trips()
			id, err := e.svc.Authenticate(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			if len(id.Permissions.FieldMasks) == 0 {
				t.Fatal("the measured seat carries no field mask, so the mask read went uncounted")
			}
			seen, roundTrips := meter.Cost(), trips()-before
			t.Logf("authenticate statements=%d exchanges=%d round trips=%d transactions=%d", seen.Statements, seen.Exchanges, roundTrips, seen.Transactions)
			if seen.Transactions != 1 {
				t.Errorf("authenticate opened %d transactions, want 1", seen.Transactions)
			}
			if seen.Statements > authenticateStatementCeiling {
				t.Errorf("authenticate sent %d statements, budget %d", seen.Statements, authenticateStatementCeiling)
			}
			if roundTrips < seen.Exchanges || roundTrips != authenticateRoundTrips {
				t.Errorf("authenticate used %d round trips for %d pgx calls, want %d", roundTrips, seen.Exchanges, authenticateRoundTrips)
			}
		})
	}
}

func TestAuthenticateWritesTheSessionStampOncePerInterval(t *testing.T) {
	e := setupRevocationEnv(t, "stamp-interval")
	token := e.signIn(t, "device")
	stampAge := func(age string) {
		t.Helper()
		if _, err := e.owner.Exec(t.Context(),
			`UPDATE session SET last_seen_at = now() - @age::interval WHERE token_hash = @token`,
			pgx.StrictNamedArgs{"age": age, "token": hashToken(token)}); err != nil {
			t.Fatal(err)
		}
	}
	lastSeen := func() time.Time {
		t.Helper()
		var seen time.Time
		if err := e.owner.QueryRow(t.Context(), `SELECT last_seen_at FROM session WHERE token_hash = @token`,
			pgx.StrictNamedArgs{"token": hashToken(token)}).Scan(&seen); err != nil {
			t.Fatal(err)
		}
		return seen
	}
	for _, c := range []struct {
		age    string
		writes bool
	}{{"30 seconds", false}, {"90 seconds", true}} {
		stampAge(c.age)
		before := lastSeen()
		if _, err := e.svc.Authenticate(e.wsOnlyCtx(), token); err != nil {
			t.Fatal(err)
		}
		if wrote := !lastSeen().Equal(before); wrote != c.writes {
			t.Errorf("a stamp %s old: wrote = %v, want %v", c.age, wrote, c.writes)
		}
	}
}
