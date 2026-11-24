// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

func TestReplayErasuresRejectsAStrayArgument(t *testing.T) {
	err := runReplayErasures(t.Context(), nil, refuseVault(t), nil,
		[]string{"now"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage: worker replay-erasures") {
		t.Fatalf("err = %v, want a usage error for the stray argument", err)
	}
}

// The other database subcommands ran without a key vault before the replay
// existed, and still do.
func TestADatabaseSubcommandThatNeedsNoVaultNeverResolvesOne(t *testing.T) {
	handled, err := runDatabaseSubcommand(t.Context(), nil, refuseVault(t), nil, []string{"deal-key-names"}, &bytes.Buffer{})
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

func TestReplayErasuresWarnsWhenTheJournalSharesTheRestoredStore(t *testing.T) {
	settings, err := configtest.Load(t, "", nil, nil, blobstore.Settings()...)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	var out bytes.Buffer
	err = runReplayErasures(t.Context(), nil, refuseVault(t), settings, nil, &out)
	if !errors.Is(err, compose.ErrReplayNeedsObjectStore) {
		t.Fatalf("err = %v, want the replay refused for want of a store", err)
	}
	if !strings.Contains(out.String(), "WARNING: no separate erasure journal store is set") {
		t.Fatalf("output = %q, want the shared-journal warning", out.String())
	}
}

func TestTheJournalStoreIsTheSeparateOneWhenSet(t *testing.T) {
	main := blobstore.NewMemory()
	dir := t.TempDir()
	settings, err := configtest.Load(t, "", []string{blobstore.EnvJournalPath + "=" + dir}, nil, blobstore.Settings()...)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	journal, separate, err := blobstore.ResolveJournal(t.Context(), settings, main)
	if err != nil || !separate || journal == main {
		t.Fatalf("got separate=%v err=%v, want a store other than the main one", separate, err)
	}

	unset, err := configtest.Load(t, "", nil, nil, blobstore.Settings()...)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	fallback, separate, err := blobstore.ResolveJournal(t.Context(), unset, main)
	if err != nil || separate || fallback != main {
		t.Fatalf("got separate=%v err=%v, want the main store when none is set", separate, err)
	}
}
