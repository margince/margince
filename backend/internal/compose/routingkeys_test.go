// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// The routing path opens the vault the boot opened, so its root key comes
// from the loaded settings and not the process environment; a provider key
// still comes from the environment.
func TestRoutingKeysTakeTheRootKeyFromSettingsAndProviderKeysFromTheEnvironment(t *testing.T) {
	t.Setenv(keyvault.EnvRootKey, "from-the-process")
	t.Setenv("GEMINI_API_KEY", "a-provider-key")
	settings, err := configtest.Load(t, "", []string{keyvault.EnvRootKey + "=from-the-settings"}, nil, keyvault.Settings()...)
	if err != nil {
		t.Fatalf("loading the root key: %v", err)
	}
	keys := RoutingKeys(settings)
	if got := keys(keyvault.EnvRootKey); got != "from-the-settings" {
		t.Errorf("root key %q, want the loaded setting's", got)
	}
	if got := keys("GEMINI_API_KEY"); got != "a-provider-key" {
		t.Errorf("provider key %q, want the environment's", got)
	}
}
