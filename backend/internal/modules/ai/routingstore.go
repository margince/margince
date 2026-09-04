// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Reading and replacing the installation's tier→model binding.
//
// The store owns what the transport must not: the RBAC gate, the validation the
// routing file was always held to, and telling whoever is serving that the
// binding changed. A handler that skipped any of the three would produce a
// binding nobody vetted, or one stored and never served.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RoutingStore reads and replaces the stored binding.
type RoutingStore struct {
	settings *settings.Store
	// keys resolves a provider's BYOK secret. It travels with the store for
	// the same reason it travels with a parsed config: a routing document names
	// providers and never their credentials, so the two meet only where a
	// binding is turned into something that can serve.
	keys config.Lookup
	// vault resolves a sealed BYOK key, for the one read that has to CALL a
	// vendor rather than describe it. Optional: an installation with no vault
	// keeps its credentials in the environment, which is where every
	// installation had them before the vault existed, and `keys` still answers.
	vault keyvault.Vault
	// catalogue serves the one vendor (OpenRouter) whose list this store asks
	// unauthenticated and unbound, for the ONE read that ranks by a published
	// benchmark rather than by a stored binding. Optional: absent it, that
	// vendor answers not_published like any adapter this build does not carry.
	catalogue *ModelCatalogue
}

// NewRoutingStore builds the store over the settings catalog.
func NewRoutingStore(s *settings.Store, keys config.Lookup) *RoutingStore {
	return &RoutingStore{settings: s, keys: keys}
}

// WithVault returns a store that can resolve a sealed credential.
//
// A separate constructor rather than a fourth argument on NewRoutingStore: the
// vault is needed by exactly one method, every other caller of this store has
// no vault to give, and widening the constructor would make all of them say so.
func (s *RoutingStore) WithVault(vault keyvault.Vault) *RoutingStore {
	next := *s
	next.vault = vault
	return &next
}

// resolvedKeys is the credential lookup a vendor call uses: the vault's sealed
// keys for this request's workspace, falling back to the environment for a
// vendor that has none sealed yet.
//
// Per request, because the workspace is the request's. A store-wide lookup
// would either be one tenant's credentials serving another's call, or the
// environment only — and the environment is exactly what an installation that
// pasted its key into the UI does not have.
func (s *RoutingStore) resolvedKeys(ctx context.Context) config.Lookup {
	if s.vault == nil {
		return s.keys
	}
	workspace, err := credentialWorkspace(ctx)
	if err != nil {
		return s.keys
	}
	refs, err := settings.Get(ctx, s.settings, ProviderKeys)
	if err != nil {
		// The environment still answers. A settings read that fails is not a
		// reason to report every vendor as unkeyed — that would tell a reader
		// their credentials are gone when what failed was one query.
		return s.keys
	}
	return SealedKeys(ctx, s.vault, workspace, refs, s.keys)
}

// Get reads the stored binding. An installation that has bound nothing reads as
// the zero config rather than an error — that is a state, not a fault.
func (s *RoutingStore) Get(ctx context.Context) (RoutingConfig, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return RoutingConfig{}, err
	}
	return settings.Get(ctx, s.settings, Routing)
}

// Replace stores a whole binding, having held it to the bar the file loader
// applies. The write is audit-only (EVT-NOEVT-3): the settings store stamps the
// audit row, and the closed event catalog defines no routing verb.
//
// It returns the FINALIZED config — defaults applied, version computed — rather
// than what the caller sent, because that is what will be served, and because
// the version is what a caller re-pointing a lane needs to see change.
func (s *RoutingStore) Replace(ctx context.Context, next RoutingConfig) (RoutingConfig, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return RoutingConfig{}, err
	}
	// Unconfigured is a legitimate destination: an operator unbinding every
	// model is choosing to run without AI, and it is the state a fresh
	// installation is already in.
	if !next.Unconfigured() {
		var err error
		if next, err = next.finalize(); err != nil {
			return RoutingConfig{}, settings.InvalidValue{
				Setting: RoutingKey, Code: settings.CodeInvalidValue, Reason: err.Error(),
			}
		}
	}
	if err := settings.Set(ctx, s.settings, Routing, next); err != nil {
		return RoutingConfig{}, err
	}
	return next, nil
}
