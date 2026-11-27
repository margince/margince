// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"io/fs"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
)

// SetupAppWithEnv is SetupAppWithOptions for an app whose program read environ
// at boot, so a suite sees what a setting set there locks. It loads the api's
// configuration the way cmd/api does and composes the app over it.
func SetupAppWithEnv(t *testing.T, environ []string, opts ...compose.Option) *AppEnv {
	t.Helper()
	cfg := LoadEnv(t, environ)
	e := SetupAppWithOptions(t, append(opts, compose.WithConfig(cfg))...)
	e.Config = cfg
	return e
}

// Ctx is ctx serving the program this app was composed for.
func (e *AppEnv) Ctx(ctx context.Context) context.Context {
	if e.Config == nil {
		return ctx
	}
	return compose.ConfigContext(ctx, e.Config)
}

// LoadEnv loads the api's registry from environ alone: no flag, and no file,
// so a margince.yaml beside the test cannot reach it. A suite that composes its
// stores itself serves them through compose.ConfigContext.
func LoadEnv(t *testing.T, environ []string) *config.Config {
	t.Helper()
	reg, err := compose.ProgramRegistry(config.RoleAPI)
	if err != nil {
		t.Fatalf("the api's settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		Known:    compose.AllSettingNames(),
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the api's configuration from %v: %v", environ, err)
	}
	return cfg
}
