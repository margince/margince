// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
)

func TestEveryProgramRegistryAssembles(t *testing.T) {
	t.Parallel()
	for role, reg := range compose.ProgramRegistries() {
		if reg == nil || len(reg.All()) == 0 {
			t.Fatalf("%s: empty registry", role)
		}
	}
}

func TestAWorkerOnlyYAMLKeyIsKnownToTheAPI(t *testing.T) {
	t.Parallel()
	key := programsettings.JobDrainWindow.Key()
	api, err := compose.ProgramRegistry(config.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	if _, inAPI := api.Lookup(key); inAPI {
		t.Fatalf("%s is the api's own setting, so it proves nothing here", key)
	}
	if !compose.AllSettingKeys(key) || !compose.AllSettingEnvs(programsettings.JobDrainWindow.Env()) {
		t.Fatal("the api would refuse a shared margince.yaml")
	}
}

// migrate reads the credentials only: a worker or api setting in its registry
// would let a stray variable change how the schema is applied.
func TestMigrateReadsOnlyTheDatabaseCredentials(t *testing.T) {
	t.Parallel()
	reg, err := compose.ProgramRegistry(config.RoleMigrate)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range reg.All() {
		keys = append(keys, s.Key())
	}
	if len(keys) != 2 || keys[0] != "dsn" || keys[1] != "owner_dsn" {
		t.Fatalf("migrate reads %v, want [dsn owner_dsn]", keys)
	}
}
