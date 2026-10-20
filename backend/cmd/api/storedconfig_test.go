// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
)

// The api's own load hands compose what it loaded, so a stored setting a boot
// source sets is locked in every store compose builds. Not parallel: it reads
// the process's one binding, which every other load here also writes.
func TestTheAPILoadLocksAStoredSettingItsEnvironmentSets(t *testing.T) {
	t.Cleanup(func() { compose.UseConfig(nil) })
	_, loaded, err := loadAPIConfig([]string{"--dsn", testDSN}, []string{"MARGINCE_INSTALLATION_SEND_RATE_LIMIT=7"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Bound() != loaded {
		t.Fatal("compose holds a configuration other than the one the api loaded")
	}
	origin, locked := compose.StoredConfig(nil).Locked(identity.SendRateLimit.Key())
	if !locked || origin.Name != "MARGINCE_INSTALLATION_SEND_RATE_LIMIT" {
		t.Fatalf("the environment's send rate is not a lock compose sees: %+v %v", origin, locked)
	}
}
