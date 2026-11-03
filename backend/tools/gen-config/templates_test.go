// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// Every default the defaults file writes loads in every program and reads
// back as the default it shows. Secrets are left out: their file references
// name files no test has.
func TestEveryDefaultTheDefaultsFileWritesLoads(t *testing.T) {
	regs := compose.ProgramRegistries()
	settings, err := collect(regs)
	if err != nil {
		t.Fatal(err)
	}
	var plain []setting
	for _, s := range settings {
		if !s.Secret() {
			plain = append(plain, s)
		}
	}
	defaults, err := defaultsYAML(plain)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(defaults), 0o600); err != nil {
		t.Fatal(err)
	}
	compared := 0
	for _, role := range slices.Sorted(maps.Keys(regs)) {
		cfg, err := config.Load(regs[role], config.Sources{
			Args: []string{"--config", path}, Environ: []string{},
			Known: compose.AllSettingNames(), BootstrapRoots: deployconfig.Roots(),
		})
		if err != nil {
			t.Errorf("%s: the defaults file does not load: %v", role, err)
			continue
		}
		for _, e := range cfg.Entries() {
			if e.Default == nil {
				continue
			}
			compared++
			if e.Value == nil || *e.Value != *e.Default {
				t.Errorf("%s: %s reads back as %v from the defaults file, not its default %q", role, e.Key, e.Value, *e.Default)
			}
		}
	}
	if compared == 0 {
		t.Error("no setting with a default was read back; this test compares nothing")
	}
}
