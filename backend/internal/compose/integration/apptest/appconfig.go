// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"io/fs"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
)

// SetupAppWithEnv is SetupAppWithOptions for an app whose program read environ
// at boot, so a suite sees what a setting set there locks. It loads the api's
// configuration the way cmd/api does. Compose holds it from before the app is
// composed until the test ends.
func SetupAppWithEnv(t *testing.T, environ []string, opts ...compose.Option) *AppEnv {
	t.Helper()
	compose.UseConfig(withEnv(t, environ))
	t.Cleanup(func() { compose.UseConfig(nil) })
	return SetupAppWithOptions(t, opts...)
}

// withEnv loads the api's registry from environ alone: no flag, and no file,
// so a margince.yaml beside the test cannot reach it.
func withEnv(t *testing.T, environ []string) *config.Config {
	t.Helper()
	reg, err := compose.ProgramRegistry(config.RoleAPI)
	if err != nil {
		t.Fatalf("the api's settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		KnownKey: compose.AllSettingKeys,
		KnownEnv: compose.AllSettingEnvs,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the api's configuration from %v: %v", environ, err)
	}
	return cfg
}
