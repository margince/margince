// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

const (
	shippedExample    = "../../../config/margince.example.yaml"
	shippedDevPosture = "../../../config/margince.dev.yaml"
)

// loadShipped loads the settings of one program from shipped files, the way
// its boot does, with an empty environment.
func loadShipped(t *testing.T, role config.Role, args ...string) *config.Config {
	t.Helper()
	reg, err := compose.ProgramRegistry(role)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Args: args, Known: compose.AllSettingNames(), BootstrapRoots: deployconfig.Roots(),
	})
	if err != nil {
		t.Fatalf("%s: %v", role, err)
	}
	return cfg
}

// The generated example is what `make dev` copies and every deployment starts
// from, so both serving programs load it as written. It arms nothing. The dev
// file turns the MCP connector on.
func TestTheShippedExampleLoadsForBothServingPrograms(t *testing.T) {
	t.Parallel()
	for _, role := range []config.Role{config.RoleAPI, config.RoleWorker} {
		example := loadShipped(t, role, "--config", shippedExample)
		dev := loadShipped(t, role, "--config", shippedExample, "--config-file", shippedDevPosture)
		if role == config.RoleAPI && (config.Val(example, ps.MCPConnector) || !config.Val(dev, ps.MCPConnector)) {
			t.Error("the dev file turns the MCP connector on and the example leaves it at its default, off")
		}
	}
}

// A dev stack reads the tracked dev file as --config-file, which arms the
// reset; the example on its own, which every deployment copies, arms nothing.
func TestTheShippedDevFileArmsTheResetAndTheExampleDoesNot(t *testing.T) {
	t.Parallel()
	for _, role := range []config.Role{config.RoleAPI, config.RoleWorker} {
		dev := loadShipped(t, role, "--config", shippedExample, "--config-file", shippedDevPosture)
		if !config.Val(dev, ps.AllowDataReset) || !config.Val(dev, ps.AllowTestMailbox) {
			t.Errorf("%s: the dev file must arm the reset and the test mailbox; a dev stack without them is a silent regression", role)
		}
		if config.Val(loadShipped(t, role, "--config", shippedExample), ps.AllowDataReset) {
			t.Errorf("%s: the shipped example arms the tenant-data purge", role)
		}
	}
}
