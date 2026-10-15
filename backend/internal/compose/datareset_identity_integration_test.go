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

// A data reset clears records, not the installation's own decisions. An
// identity setting in the configuration registry keeps its row, and an
// operating value goes back to its default.
func TestADataResetKeepsTheIdentitySettingsOfTheRegistry(t *testing.T) {
	e := integration.Setup(t)
	kept := []string{identity.RequireMFA.Key(), identity.DateFormat.Key(), identity.BaseLanguage.Key()}
	e.WsExec(t, `INSERT INTO setting (key, value) VALUES
		($1, 'true'), ($2, '"dmy"'), ($3, '"de"'), ($4, '5')
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		kept[0], kept[1], kept[2], identity.SendRateLimit.Key())

	h := dataResetHandlers{
		pool:             e.Pool,
		seeds:            deployconfig.Seeds{},
		dataResetAllowed: true,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if _, err := h.run(e.Admin(), "Authz"); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, key := range kept {
		if got := e.WsCount(t, `SELECT count(*) FROM setting WHERE key = $1`, key); got != 1 {
			t.Errorf("%s has %d rows after the reset, want its row kept", key, got)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM setting WHERE key = $1`, identity.SendRateLimit.Key()); got != 0 {
		t.Errorf("%s kept its row through the reset, want it back to the default", identity.SendRateLimit.Key())
	}
}
