// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// serverSecretPrefix marks a secret this installation minted, so one pasted
// into the wrong settings field is recognisable.
const serverSecretPrefix = "mwf_"

// mintFormKey answers 192 random bits as 32 base64url characters. The key is
// unguessable, so a paused or archived form cannot be found by walking keys.
func mintFormKey() (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("webforms: minting a form key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func mintServerSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("webforms: minting a signing secret: %w", err)
	}
	return serverSecretPrefix + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func workspaceOf(ctx context.Context) (ids.WorkspaceID, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.WorkspaceID{}, fmt.Errorf("webforms: no workspace bound: %w", apperrors.ErrPermissionDenied)
	}
	return ids.From[ids.WorkspaceKind](ws), nil
}

// storeSecret seals the secret a signed sender needs and answers its ref. For
// a server form it also answers the secret it minted, to be shown once; a
// browser form stores nothing.
func (s *Store) storeSecret(ctx context.Context, sender string, pasted *string) (ref *string, minted string, err error) {
	if !signedSender(sender) {
		return nil, "", nil
	}
	if s.vault == nil {
		return nil, "", badInput("sender", "this installation has no secret store, so only a browser form can be set up")
	}
	secret := ""
	if sender == SenderServer {
		if secret, err = mintServerSecret(); err != nil {
			return nil, "", err
		}
	} else {
		secret = *pasted
	}
	ws, err := workspaceOf(ctx)
	if err != nil {
		return nil, "", err
	}
	stored, err := s.vault.Put(ctx, ws, []byte(secret))
	if err != nil {
		return nil, "", fmt.Errorf("webforms: sealing the form's secret: %w", err)
	}
	out := string(stored)
	if sender == SenderServer {
		minted = secret
	}
	return &out, minted, nil
}

// openSecret answers the secret a signed form verifies against.
func (s *Store) openSecret(ctx context.Context, f Form) ([]byte, error) {
	if f.SecretRef == nil || s.vault == nil {
		return nil, errUnverified
	}
	ws, err := workspaceOf(ctx)
	if err != nil {
		return nil, err
	}
	return s.vault.Get(ctx, ws, keyvault.Ref(*f.SecretRef))
}

// dropSecret destroys a secret no row points at any more. It runs after the
// change that orphaned it is settled, and never fails that change.
func (s *Store) dropSecret(ctx context.Context, ref *string, lifecycle string) {
	if ref == nil {
		return
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return
	}
	keyvault.DeleteDetached(ctx, s.vault, s.log, ws, keyvault.Ref(*ref), lifecycle)
}
