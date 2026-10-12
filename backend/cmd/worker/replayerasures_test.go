// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

func TestReplayErasuresRejectsAStrayArgument(t *testing.T) {
	err := runReplayErasures(t.Context(), nil, refuseVault(t), config.Lookup(func(string) string { return "" }),
		[]string{"now"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage: worker replay-erasures") {
		t.Fatalf("err = %v, want a usage error for the stray argument", err)
	}
}

// The other database subcommands ran without a key vault before the replay
// existed, and still do.
func TestADatabaseSubcommandThatNeedsNoVaultNeverResolvesOne(t *testing.T) {
	handled, err := runDatabaseSubcommand(t.Context(), nil, refuseVault(t), []string{"deal-key-names"}, &bytes.Buffer{})
	if !handled || err == nil {
		t.Fatalf("handled = %v, err = %v; want deal-key-names to refuse its missing flags", handled, err)
	}
}

func refuseVault(t *testing.T) func() (keyvault.Vault, error) {
	t.Helper()
	return func() (keyvault.Vault, error) {
		t.Fatal("the key vault was resolved for a subcommand that does not need it")
		return nil, nil
	}
}
