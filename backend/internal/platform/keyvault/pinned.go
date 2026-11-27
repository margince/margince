// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package keyvault

import (
	"context"
	"sync/atomic"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pinnedVault scopes every call to the one id the installation's refs were
// minted under, whatever workspace id the caller passes. An installation holds
// one company, so the caller's id carries no information, and the refs already
// stored name the stored-key id. Resolving it here keeps every sealer that
// still passes an id from reaching a ref it cannot open.
type pinnedVault struct {
	inner   Vault
	resolve func(context.Context) (ids.WorkspaceID, error)
	pinned  atomic.Pointer[ids.WorkspaceID]
}

// pin wraps vault so each call is scoped to the id resolve answers. The answer
// is read until it succeeds once, then kept: the stored-key id never changes.
func pin(vault Vault, resolve func(context.Context) (ids.WorkspaceID, error)) *pinnedVault {
	return &pinnedVault{inner: vault, resolve: resolve}
}

// prime resolves the id before any caller holds a connection. A vault used
// inside an open transaction must not need a second connection to learn it.
func (p *pinnedVault) prime(ctx context.Context) error {
	_, err := p.scope(ctx)
	return err
}

func (p *pinnedVault) scope(ctx context.Context) (ids.WorkspaceID, error) {
	if cached := p.pinned.Load(); cached != nil {
		return *cached, nil
	}
	ws, err := p.resolve(ctx)
	if err != nil {
		return ids.WorkspaceID{}, err
	}
	p.pinned.Store(&ws)
	return ws, nil
}

func (p *pinnedVault) Put(ctx context.Context, _ ids.WorkspaceID, secret []byte) (Ref, error) {
	ws, err := p.scope(ctx)
	if err != nil {
		return "", err
	}
	return p.inner.Put(ctx, ws, secret)
}

func (p *pinnedVault) Get(ctx context.Context, _ ids.WorkspaceID, ref Ref) ([]byte, error) {
	ws, err := p.scope(ctx)
	if err != nil {
		return nil, err
	}
	return p.inner.Get(ctx, ws, ref)
}

func (p *pinnedVault) GetOn(ctx context.Context, q Querier, _ ids.WorkspaceID, ref Ref) ([]byte, error) {
	ws, err := p.scope(ctx)
	if err != nil {
		return nil, err
	}
	return p.inner.GetOn(ctx, q, ws, ref)
}

// Delete removes a secret under the id its own ref names. A retention or
// erasure pass thus reaches an archived company's ciphertext. A read does not
// widen like this: that credential stays out of reach of every live caller, as
// it did before the id was pinned.
func (p *pinnedVault) Delete(ctx context.Context, _ ids.WorkspaceID, ref Ref) error {
	if parsed, err := ref.parse(); err == nil {
		return p.inner.Delete(ctx, parsed.workspace, ref)
	}
	ws, err := p.scope(ctx)
	if err != nil {
		return err
	}
	return p.inner.Delete(ctx, ws, ref)
}

func (p *pinnedVault) Health(ctx context.Context) error { return p.inner.Health(ctx) }
