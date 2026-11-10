// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"slices"
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
	names := compose.AllSettingNames()
	if !slices.Contains(names.Keys, key) || !slices.Contains(names.Envs, programsettings.JobDrainWindow.Env()) {
		t.Fatal("the api would refuse a shared margince.yaml")
	}
}

// Data tasks read immutable originals through the same object-store settings
// as the worker; unrelated runtime settings cannot change schema application.
func TestMigrateReadsOnlyItsDatabaseKeyAndOriginalStoreSettings(t *testing.T) {
	t.Parallel()
	reg, err := compose.ProgramRegistry(config.RoleMigrate)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range reg.All() {
		keys = append(keys, s.Key())
	}
	want := []string{"blobstore_access_key", "blobstore_bucket", "blobstore_endpoint", "blobstore_path", "blobstore_region", "blobstore_secret_key", "blobstore_use_ssl", "dsn", "keyvault_root_key", "owner_dsn"}
	if !slices.Equal(keys, want) {
		t.Fatalf("migrate reads %v, want %v", keys, want)
	}
}
