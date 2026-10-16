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
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// A posture the environment sets is the one every decision reads, without the
// database. Not parallel: the binding is the process's own.
func TestABootSetPostureIsTheOneEveryDecisionReads(t *testing.T) {
	boot := enforceEveryCategory()
	var entries []string
	for _, category := range slices.Sorted(maps.Keys(boot)) {
		boot[category] = "observe"
		entries = append(entries, category+":observe")
	}
	configtest.Bind(t, configtest.MustLoadEnv(t, ConfigSettings(), "MARGINCE_CONSENT_AUTHORIZATION_MODES="+strings.Join(entries, ",")))

	modes, err := authorizationModesTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("reading the boot-set posture: %v", err)
	}
	if !maps.Equal(modes, boot) {
		t.Fatalf("the decisions read %v, want the %v the environment sets", modes, boot)
	}
}

// A posture that names only some categories fails the boot, as its save would.
// No admin could save the map it would lock.
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
