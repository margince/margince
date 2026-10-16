// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"crypto/rand"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// SetupGoogleConnect mounts the Google connect flow over a vault, with the
// environment's app given by clientID and secret. Empty means it composed none,
// and only a stored app can serve.
func SetupGoogleConnect(t *testing.T, clientID, secret string, opts ...compose.Option) (*AppEnv, keyvault.Vault) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: EarlyPool(t)})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	e := SetupAppWithOptions(t, append([]compose.Option{
		compose.WithKeyvault(vault),
		compose.WithGmailCapture(compose.GmailConfig{
			ClientID:      clientID,
			ClientSecret:  secret,
			StateKey:      "0123456789abcdef0123456789abcdef",
			PublicBaseURL: "https://app.example",
		}, compose.CaptureConfig{}),
	}, opts...)...)
	e.BootstrapWorkspace(t)
	return e, vault
}
