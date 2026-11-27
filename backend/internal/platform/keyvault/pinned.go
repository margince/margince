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
//
//nolint:ireturn // the seam has two providers behind one Vault; returning the interface is the design.
func pin(vault Vault, resolve func(context.Context) (ids.WorkspaceID, error)) Vault {
	return &pinnedVault{inner: vault, resolve: resolve}
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

func (p *pinnedVault) Delete(ctx context.Context, _ ids.WorkspaceID, ref Ref) error {
	ws, err := p.scope(ctx)
	if err != nil {
		return err
	}
	return p.inner.Delete(ctx, ws, ref)
}

func (p *pinnedVault) Health(ctx context.Context) error { return p.inner.Health(ctx) }
