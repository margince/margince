// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package configtest loads settings the way a program does, from a
// margince.yaml body and variables, so a test reads them through config.Val.
package configtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// Load writes body as margince.yaml in a temp dir and loads settings over it
// and environ, leaving the bootstrap roots to deployconfig. faults, when given, is the program's cross-setting check.
func Load(t testing.TB, body string, environ []string, faults func(*config.Config) []error, settings ...config.Any) (*config.Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("configtest: writing %s: %v", path, err)
	}
	reg, err := config.NewRegistry(settings)
	if err != nil {
		t.Fatalf("configtest: %v", err)
	}
	return config.Load(reg, config.Sources{
		Args: []string{"--config", path}, Environ: environ, Faults: faults, BootstrapRoots: deployconfig.Roots(),
	})
}

// MustLoad is Load for a test whose configuration is valid by construction.
func MustLoad(t testing.TB, body string, settings ...config.Any) *config.Config {
	t.Helper()
	cfg, err := Load(t, body, nil, nil, settings...)
	if err != nil {
		t.Fatalf("configtest: loading %q: %v", body, err)
	}
	return cfg
}
