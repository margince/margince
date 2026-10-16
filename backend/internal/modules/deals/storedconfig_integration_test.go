// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// dealsConfig is this module's configuration as a program loads it from
// environ alone, reading and saving through pool.
func dealsConfig(t *testing.T, pool *pgxpool.Pool, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatalf("the deals settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the deals settings from %v: %v", environ, err)
	}
	return cfg.WithDatabase(settings.New(pool, settings.NewRegistry(cfg.StoredDefinitions()...)))
}

// bindForTest binds cfg as the program's configuration until the test ends.
func bindForTest(t *testing.T, cfg *config.Config) {
	t.Helper()
	before := config.Bound()
	config.Bind(cfg)
	t.Cleanup(func() { config.Bind(before) })
}

// The kill switch the environment sets stops an automatic apply over a saved
// row that allows it, and the settings surface reads it. Not parallel: the
// binding is the process's own.
func TestABootSetKillSwitchStopsAnAutoApply(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, toID := twoStagePipeline(t, e)
	ref := transitionOf(t, e, fromID, toID)
	autopilotOn(t, e, true)
	ruleOn(t, e, ref)
	aGoodRecord(t, e, dealID, ref)
	if got := verdictFor(t, e, ref); got.Mode != ModeAuto {
		t.Fatalf("the fixture does not reach auto on its own (%s: %s)", got.Mode, got.Why)
	}
	bindForTest(t, dealsConfig(t, e.pool, "MARGINCE_DEALS_STAGE_AUTOPILOT_ENABLED=false"))

	if got := verdictFor(t, e, ref); got.Mode != ModePropose {
		t.Fatalf("a transition applied automatically with the kill switch off at boot: %s", got.Mode)
	}
	if on, err := e.store.StageAutomationEnabled(e.as()); err != nil || on {
		t.Fatalf("the settings surface reads automation %v (%v), want the off the environment sets", on, err)
	}
}

// Legal review switched off at boot releases the gate over a saved row that
// keeps it on.
func TestABootSetLegalReviewOffReleasesTheGate(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	bindForTest(t, dealsConfig(t, e.pool, "MARGINCE_DEALS_LEGAL_REVIEW_ENABLED=false"))

	if err := e.advance(dealID, toID); err != nil {
		t.Fatalf("the gate held a deal with legal review off at boot: %v", err)
	}
}
