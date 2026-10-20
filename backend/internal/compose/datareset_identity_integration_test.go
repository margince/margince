// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// A data reset clears records, not the installation's own decisions. Every
// identity setting in the configuration registry declared to survive a reset
// keeps its row, and every other one goes back to its default.
func TestADataResetKeepsTheIdentitySettingsOfTheRegistry(t *testing.T) {
	e := integration.Setup(t)
	identityKeys := map[string]bool{}
	for _, s := range identity.Settings() {
		identityKeys[s.Key()] = true
	}
	survives := map[string]bool{}
	for _, def := range storedDefinitions() {
		if !identityKeys[def.Key()] {
			continue
		}
		survives[def.Key()] = def.SurvivesDataReset()
		if def.Key() == identity.Name.Key() {
			// The harness seeded it, and the reset is confirmed by it.
			continue
		}
		raw, err := def.DefaultJSON()
		if err != nil {
			t.Fatal(err)
		}
		e.WsExec(t, `INSERT INTO setting (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, def.Key(), raw)
	}
	if len(survives) != len(identityKeys) {
		t.Fatalf("found rows for %d of identity's %d settings", len(survives), len(identityKeys))
	}

	h := dataResetHandlers{
		pool:             e.Pool,
		seeds:            deployconfig.Seeds{},
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if _, err := h.run(e.Admin(), "Authz"); err != nil {
		t.Fatalf("run: %v", err)
	}
	for key, kept := range survives {
		want := 0
		if kept {
			want = 1
		}
		if got := e.WsCount(t, `SELECT count(*) FROM setting WHERE key = $1`, key); got != want {
			t.Errorf("%s has %d rows after the reset, want %d (survives a reset: %t)", key, got, want, kept)
		}
	}
}
