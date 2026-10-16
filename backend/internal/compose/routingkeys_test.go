// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The routing path opens the vault the boot opened. So its root key comes from
// the loaded settings, never the process environment. A provider key comes
// from the loaded settings too, through its own setting and never through this
// lookup or the process environment.
func TestRoutingKeysTakeTheRootKeyFromSettingsAndProviderKeysFromTheRegistry(t *testing.T) {
	t.Setenv(keyvault.EnvRootKey, "from-the-process")
	t.Setenv("GEMINI_API_KEY", "from-the-process")
	settings, err := configtest.Load(t, "", []string{keyvault.EnvRootKey + "=from-the-settings", "GEMINI_API_KEY=a-provider-key"}, nil,
		append(keyvault.Settings(), ai.Settings()...)...)
	if err != nil {
		t.Fatalf("loading the root key: %v", err)
	}
	keys := RoutingKeys(settings)
	if got := keys(keyvault.EnvRootKey); got != "from-the-settings" {
		t.Errorf("root key %q, want the loaded setting's", got)
	}
	if got := keys("GEMINI_API_KEY"); got != "" {
		t.Errorf("provider key %q through the root-key lookup, want none", got)
	}
	read := ai.KeyLookup(principal.SystemActing(context.Background(), "test"), settings, slog.New(slog.DiscardHandler))
	if got := read("GEMINI_API_KEY"); got != "a-provider-key" {
		t.Errorf("provider key %q, want the loaded setting's", got)
	}
}
