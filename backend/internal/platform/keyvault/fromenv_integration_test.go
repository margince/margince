// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package keyvault_test

// A root key that goes missing while the installation is holding sealed
// ciphertext.
//
// This needs a real database because the whole question is what is in
// vault_secret, and it is the one failure the vault has that nothing else can
// report honestly: every reader downstream discovers it separately and
// describes it in its own dialect, the worst of which calls an installation
// that has a license unlicensed. Refusing here is what lets those readers keep
// treating a nil vault as "nothing was ever sealed".

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An installation that has sealed nothing is the ordinary case — every
// development and CI process in this repository — and it must keep booting with
// no vault at all. Asserted first, because a refusal that fired here would stop
// every one of them.
func TestNoRootKeyAndNothingSealedIsNotAnError(t *testing.T) {
	pool := singleConnPool(t)

	vault, configured, err := keyvault.FromEnv(t.Context(), pool, config.Static(nil))
	if err != nil {
		t.Fatalf("an installation with no vault and nothing sealed was refused: %v", err)
	}
	if configured || vault != nil {
		t.Errorf("configured=%v vault!=nil=%v, want an absent vault", configured, vault != nil)
	}
}

func TestSealedSecretsWithNoRootKeyRefuseTheBoot(t *testing.T) {
	pool := singleConnPool(t)
	ctx := t.Context()

	sealing, err := keyvault.New(keyvault.Config{RootKey: rootKey(t), Pool: pool})
	if err != nil {
		t.Fatalf("building the sealing vault: %v", err)
	}
	if _, err := sealing.Put(ctx, ids.From[ids.WorkspaceKind](ids.NewV7()), []byte("a credential")); err != nil {
		t.Fatalf("sealing the credential under test: %v", err)
	}

	// The redeploy that dropped MARGINCE_KEYVAULT_ROOT_KEY: same database, same
	// ciphertext, no key.
	_, _, err = keyvault.FromEnv(ctx, pool, config.Static(nil))
	if err == nil {
		t.Fatal("a boot with sealed secrets and no root key was allowed; every credential it holds is unreachable and nothing said so")
	}
	// The variable is named because it is the only thing that fixes this, and
	// "sealed" is named because an operator has to understand that the data is
	// still there and still theirs — the key is what is missing, and it is not
	// recoverable from the ciphertext.
	for _, want := range []string{keyvault.EnvRootKey, "sealed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
}

// Every role now reads "this deployment has no vault" off the vault value
// alone — ForRole hands back nil and the boolean stops there. That rests on
// FromEnv's two answers agreeing, so the equivalence is asserted rather than
// assumed, in both directions: a nil vault reported configured
// wires a lane onto nothing, and a real vault reported unconfigured leaves
// every sealed credential unreachable on a deployment that has one.
//
// Here rather than beside the unit tests because the configured direction needs
// a pool — the local provider refuses to build without one.
func TestForRoleHandsBackNilForExactlyAnUnconfiguredVault(t *testing.T) {
	pool := singleConnPool(t)
	configured := config.Static(map[string]string{
		keyvault.EnvRootKey: base64.StdEncoding.EncodeToString(rootKey(t)),
	})

	for _, tc := range []struct {
		name       string
		env        config.Lookup
		wantAVault bool
	}{
		{"no root key", config.Static(nil), false},
		{"a root key", configured, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vault, flag, err := keyvault.FromEnv(t.Context(), pool, tc.env)
			if err != nil {
				t.Fatalf("FromEnv: %v", err)
			}
			if flag != tc.wantAVault || (vault != nil) != tc.wantAVault {
				t.Fatalf("FromEnv reported configured=%v vault!=nil=%v, want both %v",
					flag, vault != nil, tc.wantAVault)
			}
			forRole, err := keyvault.ForRole(t.Context(), "role", pool, tc.env)
			if err != nil {
				t.Fatalf("ForRole: %v", err)
			}
			if (forRole != nil) != tc.wantAVault {
				t.Fatalf("ForRole handed back vault!=nil=%v, want %v", forRole != nil, tc.wantAVault)
			}
		})
	}
}
