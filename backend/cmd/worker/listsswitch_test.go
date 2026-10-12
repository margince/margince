// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The worker reads lists.enabled from the same file the api does, so an
// operator who hides Lists also stops the Live List check.
func TestTheWorkerReadsTheListsSwitchFromTheDeploymentFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want bool
	}{
		{name: "unstated", yaml: "version: 1\n", want: true},
		{name: "stated off", yaml: "version: 1\nlists:\n  enabled: false\n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "margince.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := workerConfig{configPath: path}
			if _, err := loadDeployment(&cfg); err != nil {
				t.Fatalf("loadDeployment: %v", err)
			}
			if cfg.listsEnabled != tc.want {
				t.Errorf("listsEnabled = %v, want %v", cfg.listsEnabled, tc.want)
			}
		})
	}
}
