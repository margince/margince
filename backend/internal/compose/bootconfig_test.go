// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io/fs"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// bootConfigForTest loads role's registry from environ alone, as the program
// would, without binding it: a test passes it where it is needed instead.
func bootConfigForTest(t *testing.T, role config.Role, environ ...string) *config.Config {
	t.Helper()
	reg, err := ProgramRegistry(role)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		KnownKey: AllSettingKeys,
		KnownEnv: AllSettingEnvs,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the %s configuration from %v: %v", role, environ, err)
	}
	return cfg
}
