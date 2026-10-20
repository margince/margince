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

// Uncommented, every default the example writes loads in every program and
// reads back as the default it shows. Secrets are left out: their file
// references name files no test has.
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
	compared := 0
	for _, role := range slices.Sorted(maps.Keys(regs)) {
		cfg, err := config.Load(regs[role], config.Sources{
			Args: []string{"--config", path}, Environ: []string{},
			Known: compose.AllSettingNames(), BootstrapRoots: deployconfig.Roots(),
		})
		if err != nil {
			t.Errorf("%s: the example's defaults do not load: %v", role, err)
			continue
		}
		for _, e := range cfg.Entries() {
			if e.Default == nil {
				continue
			}
			compared++
			if e.Value == nil || *e.Value != *e.Default {
				t.Errorf("%s: %s reads back as %v from the example, not its default %q", role, e.Key, e.Value, *e.Default)
			}
		}
	}
	if compared == 0 {
		t.Error("no setting with a default was read back; this test compares nothing")
	}
}
