// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import "testing"

// The template opens every generated example, which `make dev` copies as the
// first config of a stack, so it must bootstrap as written.
func TestTheBootstrapTemplateLoads(t *testing.T) {
	boot, err := LoadBootstrap(writeTemp(t, BootstrapTemplate))
	if err != nil {
		t.Fatalf("the bootstrap template does not load: %v", err)
	}
	if boot.BootstrapAdmin == nil || boot.Workspace.Name == "" {
		t.Fatalf("the template names no workspace or admin, so a first boot cannot bootstrap: %+v", boot)
	}
}
