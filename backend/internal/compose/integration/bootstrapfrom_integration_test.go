// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// bootstrapFrom loads a margince.yaml body's bootstrap roots the way a boot
// does, through LoadBootstrap.
func bootstrapFrom(t *testing.T, doc string) (deployconfig.Bootstrap, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return deployconfig.LoadBootstrap(path)
}
