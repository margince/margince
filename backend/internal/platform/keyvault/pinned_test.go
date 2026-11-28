// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package keyvault

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAPinnedVaultOpensARefMintedUnderTheStoredIdForAnyCallerId(t *testing.T) {
	ctx := context.Background()
	stored := ids.From[ids.WorkspaceKind](ids.NewV7())
	inner := NewMemory()
	sealedBefore, err := inner.Put(ctx, stored, []byte("sealed-before"))
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	vault := pin(inner, func(context.Context) (ids.WorkspaceID, error) {
		reads++
		return stored, nil
	})
	otherCaller := ids.From[ids.WorkspaceKind](ids.NewV7())

	got, err := vault.Get(ctx, otherCaller, sealedBefore)
	if err != nil || string(got) != "sealed-before" {
		t.Fatalf("a ref sealed under the stored id was not opened for another caller id: %q, %v", got, err)
	}
	fresh, err := vault.Put(ctx, otherCaller, []byte("sealed-after"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := inner.Get(ctx, stored, fresh); err != nil || string(got) != "sealed-after" {
		t.Fatalf("a new secret was not scoped to the stored id: %q, %v", got, err)
	}
	if err := vault.Delete(ctx, otherCaller, fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := inner.Get(ctx, stored, fresh); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete left the secret: %v", err)
	}
	if reads != 1 {
		t.Fatalf("the stored id was read %d times, want once", reads)
	}
}

func TestAPinnedVaultRefusesWhileTheInstallationHasNoStoredId(t *testing.T) {
	missing := errors.New("no stored id")
	vault := pin(NewMemory(), func(context.Context) (ids.WorkspaceID, error) { return ids.WorkspaceID{}, missing })
	if _, err := vault.Put(context.Background(), ids.From[ids.WorkspaceKind](ids.NewV7()), []byte("x")); !errors.Is(err, missing) {
		t.Fatalf("Put = %v, want the resolver's refusal", err)
	}
}
