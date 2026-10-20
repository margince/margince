// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"io/fs"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// environConfig loads identity's settings from environ alone, as a program
// whose environment holds it would.
func environConfig(t *testing.T, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(Settings())
	if err != nil {
		t.Fatalf("identity's settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading identity's settings from %v: %v", environ, err)
	}
	return cfg
}
