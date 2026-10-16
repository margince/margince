// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// Uncommented, every default the example writes loads in every program, so a
// line an operator copies reads back as the default it shows. Secrets are left
// out: their file references name files no test has.
func TestEveryDefaultTheExampleWritesLoads(t *testing.T) {
	regs := compose.ProgramRegistries()
	settings, err := collect(regs)
	if err != nil {
		t.Fatal(err)
	}
	example, err := exampleYAML(settings, "version: 1\n")
	if err != nil {
		t.Fatal(err)
	}
	_, body, found := strings.Cut(example, "\n# ---- ")
	if !found {
		t.Fatal("the example has no setting group")
	}
	lines := []string{"version: 1"}
	for line := range strings.SplitSeq(body, "\n") {
		text, commented := strings.CutPrefix(line, "# ")
		if commented && !strings.HasPrefix(text, "---- ") && !strings.Contains(text, "${file:") {
			lines = append(lines, text)
		}
	}
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, role := range slices.Sorted(maps.Keys(regs)) {
		_, err := config.Load(regs[role], config.Sources{
			Args: []string{"--config", path}, Environ: []string{},
			KnownKey: compose.AllSettingKeys, KnownEnv: compose.AllSettingEnvs, BootstrapRoots: deployconfig.Roots(),
		})
		if err != nil {
			t.Errorf("%s: the example's defaults do not load: %v", role, err)
		}
	}
}
