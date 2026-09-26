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
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	return s.ReplaceIfVersion(ctx, next, "")
}

// Revision identifies the editable binding independently of credentials.
func (cfg RoutingConfig) Revision() string { return cfg.bindingDigest() }

// ReplaceIfVersion checks a supplied version under the same lock as the write.
// An empty version preserves the existing unconditional API for legacy clients.
//
// A lane that arrives with no upstream preferences keeps the ones stored for
// the same binding (see keepingStoredUpstream), read under that lock too, so a
// concurrent write cannot hand it another binding's pins.
func (s *RoutingStore) ReplaceIfVersion(ctx context.Context, next RoutingConfig, expected string) (RoutingConfig, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return RoutingConfig{}, err
	}
	var refused error
	if err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, RoutingKey); err != nil {
			return err
		}
		current, err := settings.GetTx(ctx, tx, Routing)
		if err != nil {
			return err
		}
		if expected != "" && current.Revision() != expected {
			return apperrors.ErrVersionSkew
		}
		// Unconfigured is a legitimate destination: an operator unbinding every
		// model is choosing to run without AI, and it is the state a fresh
		// installation is already in.
		if !next.Unconfigured() {
			if next, err = next.keepingStoredUpstream(current).finalize(); err != nil {
				refused = settings.InvalidValue{Setting: RoutingKey, Code: settings.CodeInvalidValue, Reason: err.Error()}
				return refused
			}
		}
		// The residency bar is the entry's own validator (validateStoredRouting),
		// which SetTx runs and answers with the same InvalidValue.
		return settings.SetTx(ctx, s.settings, tx, Routing, next)
	}); err != nil {
		if refused != nil {
			return RoutingConfig{}, refused
		}
		return RoutingConfig{}, err
	}
	return next, nil
}

// keepingStoredUpstream carries each stored lane's upstream preferences and
// thinking level onto the same lane of next when next declares none and binds
// the same provider, host and model. A value next declares always wins.
//
// A write that omits them — a client that predates the contract's `routing` or
// `thinking_level` field, or a settings seed — would otherwise drop an `only:`
// residency pin, and the broker would go back to serving that lane from any
// region. The routing editor applies the same rule from its side (rebind in
// frontend/src/screens/ai-routing-fields.tsx). Keyed on the model as well as
// the host because a pin names hosts that serve ONE model, and a level is
// refused on a model that predates it — carried onto another, either would
// fail the lane with nothing in the form able to lift it.
//
// A thinking level of thinkingLevelDefault is the explicit clear, as an empty
// `routing` object is for upstream preferences: it is stored as no level.
func (next RoutingConfig) keepingStoredUpstream(stored RoutingConfig) RoutingConfig {
	carry := func(lane, kept ProviderConfig) ProviderConfig {
		cleared := lane.ThinkingLevel == thinkingLevelDefault
		if cleared {
			lane.ThinkingLevel = ""
		}
		if lane.Provider != kept.Provider || !sameEndpoint(lane.BaseURL, kept.BaseURL) || lane.Model != kept.Model {
			return lane
		}
		if lane.Routing == nil {
			lane.Routing = kept.Routing
		}
		if lane.ThinkingLevel == "" && !cleared {
			lane.ThinkingLevel = kept.ThinkingLevel
		}
		return lane
	}
	tiers := make(map[Tier]ProviderConfig, len(next.Tiers))
	for tier, binding := range next.Tiers {
		tiers[tier] = carry(binding, stored.Tiers[tier])
	}
	next.Tiers = tiers
	next.Embeddings.ProviderConfig = carry(next.Embeddings.ProviderConfig, stored.Embeddings.ProviderConfig)
	return next
}

// sameEndpoint reports whether two base URLs name one endpoint, ignoring the
// spellings a form or a hand edit varies without meaning to: surrounding
// space, a trailing slash, and the case of scheme and host (URLs are
// case-insensitive in both). Compared byte for byte, re-saving
// "https://openrouter.ai/api/" over a stored "https://openrouter.ai/api" would
// silently drop the lane's residency pin.
func sameEndpoint(a, b string) bool {
	return canonicalEndpoint(a) == canonicalEndpoint(b)
}

func canonicalEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		// Unparseable is compared as written: it cannot be dialled, so the
		// only question left is whether it is literally the stored value.
		return raw
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return parsed.String()
}
