// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// consentConfig is this module's settings as a program loads them from
// environ alone: no flag, and no file beside the test.
func consentConfig(t *testing.T, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatalf("the consent settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the consent settings from %v: %v", environ, err)
	}
	return cfg
}

// bindForTest binds cfg as the program's configuration until the test ends.
func bindForTest(t *testing.T, cfg *config.Config) {
	t.Helper()
	before := config.Bound()
	config.Bind(cfg)
	t.Cleanup(func() { config.Bind(before) })
}

// A posture the environment sets is the one every decision reads, without the
// database. Not parallel: the binding is the process's own.
func TestABootSetPostureIsTheOneEveryDecisionReads(t *testing.T) {
	boot := enforceEveryCategory()
	var entries []string
	for _, category := range slices.Sorted(maps.Keys(boot)) {
		boot[category] = "observe"
		entries = append(entries, category+":observe")
	}
	bindForTest(t, consentConfig(t, "MARGINCE_CONSENT_AUTHORIZATION_MODES="+strings.Join(entries, ",")))

	modes, err := authorizationModesTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("reading the boot-set posture: %v", err)
	}
	if !maps.Equal(modes, boot) {
		t.Fatalf("the decisions read %v, want the %v the environment sets", modes, boot)
	}
}

// A posture that names only some categories is refused at boot, as a save of
// it is, rather than locking a map no admin could save.
func TestAPartialBootPostureFailsTheLoad(t *testing.T) {
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatal(err)
	}
	_, err = config.Load(reg, config.Sources{
		Environ:  []string{"MARGINCE_CONSENT_AUTHORIZATION_MODES=marketing:observe"},
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err == nil {
		t.Fatal("a posture naming one category loaded; a save of it is refused")
	}
}
