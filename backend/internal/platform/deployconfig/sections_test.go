// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

func TestSectionsAreTheFileRootKeys(t *testing.T) {
	t.Parallel()
	got := deployconfig.Sections()
	for _, want := range []string{"version", "workspace", "bootstrap_admin", "seeds", "auth", "web"} {
		if !got[want] {
			t.Errorf("Sections() lacks %q: %v", want, got)
		}
	}
	if got["Workspace"] || got[""] {
		t.Errorf("Sections() holds a Go field name or an empty key: %v", got)
	}
}
