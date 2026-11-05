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
	"cmp"
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RoutingStore reads and replaces the stored binding.
type RoutingStore struct {
	settings *settings.Store
	// cfg reads and saves the binding under the locks the boot sources set.
	cfg *config.Config
	// keys overrides how a provider's BYOK secret is resolved, for a test that
	// hands a client its credentials directly. Nil reads each key's setting
	// through cfg.
	keys config.Lookup
	// catalogue serves the one vendor (OpenRouter) whose list this store asks
	// unauthenticated and unbound, for the ONE read that ranks by a published
	// benchmark rather than by a stored binding. Optional: absent it, that
	// vendor answers not_published like any adapter this build does not carry.
	catalogue *ModelCatalogue
	// selectBrain builds the client every vendor read and save-time probe
	// calls through; the zero value is SelectBrain.
	selectBrain brainSelector
	// log hears a save admitted unchecked and a key that could not be read;
	// nil is slog.Default.
	log *slog.Logger
	// now dates what a Vertex location was found to serve; nil is time.Now.
	now func() time.Time
	// served holds, per Vertex location, which models it was found to serve.
	// Shared by the copies the With* builders make; nil asks every time.
	served *servedAtLocation
}

// NewRoutingStore builds the store over the settings catalog, reading the
// binding, the task overrides and the provider keys through cfg.
func NewRoutingStore(s *settings.Store, cfg *config.Config) *RoutingStore {
	return &RoutingStore{settings: s, cfg: cfg, served: &servedAtLocation{}}
}

// WithSealer returns a store that can open a sealed provider key, for the
// reads that call a vendor rather than describe it.
func (s *RoutingStore) WithSealer(sealer config.Sealer) *RoutingStore {
	next := *s
	next.cfg = s.cfg.WithSealer(sealer)
	return &next
}

// resolvedKeys is the credential lookup a vendor call uses. Per request,
// because a sealed key opens only under the request's workspace.
func (s *RoutingStore) resolvedKeys(ctx context.Context) config.Lookup {
	if s.keys != nil {
		return s.keys
	}
	return KeyLookup(ctx, s.cfg, s.logger())
}

// Get reads the stored binding as canonical, so a row stored in the per-lane
// shape every document had before providers held hosts reads as its lifted
// twin. An installation that has bound nothing reads as the zero config rather
// than an error — that is a state, not a fault.
func (s *RoutingStore) Get(ctx context.Context) (RoutingConfig, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return RoutingConfig{}, err
	}
	stored, err := config.StoredVal(ctx, s.cfg, Routing)
	if err != nil {
		return RoutingConfig{}, err
	}
	return stored.canonical(), nil
}

// Replace stores a whole binding, having held it to the bar the file loader
// applies. The write is audit-only (EVT-NOEVT-3): the settings store stamps the
// audit row, and the closed event catalog defines no routing verb.
//
// It returns the document as stored, as Get reads it (see write).
func (s *RoutingStore) Replace(ctx context.Context, next RoutingConfig) (RoutingConfig, error) {
	return s.ReplaceIfVersion(ctx, next, "")
}

// probedWrite runs a write that must not store a Vertex binding Google was not
// asked about. The probe runs before the lock, because a network call must not
// hold it, and the write stores only if the document under the lock is the one
// probed: one another write moved meanwhile is probed again, and a document
// that keeps moving is refused as stale.
func (s *RoutingStore) probedWrite(ctx context.Context, probe func(stored RoutingConfig) error, settle func(current RoutingConfig) (stored, served RoutingConfig, err error)) (RoutingConfig, error) {
	// A boot-set binding is kept or refused as it stands; Google is not asked.
	if _, locked := s.cfg.Locked(RoutingKey); locked {
		return s.write(ctx, settle)
	}
	for range probeAttempts {
		stored, err := config.StoredVal(ctx, s.cfg, Routing)
		if err != nil {
			return RoutingConfig{}, err
		}
		if err := probe(stored); err != nil {
			return RoutingConfig{}, err
		}
		probed := stored.Revision()
		written, err := s.write(ctx, func(current RoutingConfig) (RoutingConfig, RoutingConfig, error) {
			if current.Revision() != probed {
				return RoutingConfig{}, RoutingConfig{}, errProbedStale
			}
			return settle(current)
		})
		if !errors.Is(err, errProbedStale) {
			return written, err
		}
	}
	return RoutingConfig{}, apperrors.ErrVersionSkew
}

// probeAttempts bounds how often a save re-probes a document other writes keep
// moving before it gives up as stale.
const probeAttempts = 3

var errProbedStale = errors.New("ai: routing: the document changed while its bindings were being probed")

// probeProviderSettings asks Google about each Vertex binding that one
// provider's new settings would move, as probeCandidate does for a whole
// document.
func (s *RoutingStore) probeProviderSettings(ctx context.Context, stored RoutingConfig, provider string, next ProviderSettings) error {
	_, candidate, err := stored.withProviderSettings(provider, next)
	if err != nil {
		return err
	}
	if err := s.probeVertexBindings(ctx, stored, candidate); err != nil {
		return invalidRouting(err)
	}
	return nil
}

// probeCandidate holds next to the bar the write will, then asks Google about
// what it adds or changes over stored.
func (s *RoutingStore) probeCandidate(ctx context.Context, stored, next RoutingConfig) error {
	_, candidate, err := next.replacing(stored)
	if err != nil {
		return err
	}
	if err := s.probeVertexBindings(ctx, stored, candidate); err != nil {
		return invalidRouting(err)
	}
	return nil
}

func invalidRouting(err error) error {
	if faults, ok := errors.AsType[routingFaults](err); ok {
		return faults
	}
	return settings.InvalidValue{Setting: RoutingKey, Code: settings.CodeInvalidValue, Reason: err.Error()}
}

func (s *RoutingStore) logger() *slog.Logger {
	if s.log == nil {
		return slog.Default()
	}
	return s.log
}

// LockFirst answers a refusal of a value sent for key, one the store has not
// seen, with the lock when a boot source sets key. The update gate comes
// first, as on a save, so a caller without it learns nothing of the lock.
func (s *RoutingStore) LockFirst(ctx context.Context, key string, err error) error {
	if _, locked := s.cfg.Locked(key); !locked || err == nil {
		return err
	}
	if gate := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); gate != nil {
		return gate
	}
	return s.cfg.LockFirst(key, err)
}

// Revision identifies the editable document independently of credentials. It
// digests canonical(), providers included, so editing a provider entry no lane
// binds still moves the ETag while the routing version stays put.
func (cfg RoutingConfig) Revision() string { return digestJSON(cfg.canonical()) }

// ReplaceIfVersion checks a supplied version under the same lock as the write.
// An empty version preserves the existing unconditional API for legacy clients.
//
// The document is settled against the stored one under that lock too, so a
// concurrent write cannot hand a lane another binding's serving preferences or
// another provider entry: see replacing.
func (s *RoutingStore) ReplaceIfVersion(ctx context.Context, next RoutingConfig, expected string) (RoutingConfig, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return RoutingConfig{}, err
	}
	settle := func(current RoutingConfig) (RoutingConfig, RoutingConfig, error) {
		if expected != "" && current.Revision() != expected {
			return RoutingConfig{}, RoutingConfig{}, apperrors.ErrVersionSkew
		}
		return next.replacing(current)
	}
	// A save that names no gemini_vertex lane reads and asks nothing, and a
	// stale one is refused as stale before Google is asked.
	if next.Unconfigured() || len(vertexProbesOf(next)) == 0 {
		return s.write(ctx, settle)
	}
	return s.probedWrite(ctx, func(stored RoutingConfig) error {
		if expected != "" && stored.Revision() != expected {
			return apperrors.ErrVersionSkew
		}
		return s.probeCandidate(ctx, stored, next)
	}, settle)
}

// SetProviderSettings replaces one provider's entry and re-validates the whole
// document under the routing lock, so no If-Match is needed: nothing else in
// the document changes. A zero entry removes it. A provider this build does not
// know is not found.
//
// It returns the document as stored, as Replace does.
func (s *RoutingStore) SetProviderSettings(ctx context.Context, provider string, next ProviderSettings) (RoutingConfig, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return RoutingConfig{}, err
	}
	if !knownProvider(provider) {
		return RoutingConfig{}, apperrors.ErrNotFound
	}
	settle := func(current RoutingConfig) (RoutingConfig, RoutingConfig, error) {
		return current.withProviderSettings(provider, next)
	}
	// Only a Vertex provider moves a Vertex binding.
	if provider != providerGeminiVertex {
		return s.write(ctx, settle)
	}
	return s.probedWrite(ctx, func(stored RoutingConfig) error {
		return s.probeProviderSettings(ctx, stored, provider, next)
	}, settle)
}

// write runs one routing write under the setting's row lock: settle derives the
// document to store from the current one, having held the binding it serves to
// the bar. The write is audit-only (EVT-NOEVT-3) — the settings store stamps the
// audit row — and SetTx re-runs the entry's validator, residency included.
//
// It answers with the document as stored, which is what Get reads back: its
// Revision is the next If-Match, and its tier routing is what was written. The
// served binding carries the product default, which a client writing the
// answer back would freeze into every tier.
func (s *RoutingStore) write(ctx context.Context, settle func(current RoutingConfig) (stored, served RoutingConfig, err error)) (RoutingConfig, error) {
	var written, served RoutingConfig
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, RoutingKey); err != nil {
			return err
		}
		current, err := config.StoredValTx(ctx, tx, s.cfg, Routing)
		if err != nil {
			return err
		}
		// A stale If-Match is answered as stale before the lock, as the budget
		// and the overrides answer it; any other refusal waits on the lock.
		stored, serving, err := settle(current)
		if errors.Is(err, apperrors.ErrVersionSkew) {
			return err
		}
		if err != nil {
			return s.cfg.LockFirst(RoutingKey, err)
		}
		written, served = stored, serving
		// Compared by revision, as If-Match is: a resent boot binding settles to
		// its canonical form, which need not be the decoded value field for field.
		sameBinding := func(boot RoutingConfig) bool { return boot.Revision() == stored.Revision() }
		if locked, err := config.LockedChange(s.cfg, Routing, sameBinding); locked || err != nil {
			return err
		}
		return config.SaveTx(ctx, s.cfg, tx, Routing, stored)
	})
	if err != nil {
		return RoutingConfig{}, err
	}
	LogSingleRungLadders(s.logger(), served)
	return written, nil
}

// replacing settles a whole document written over current. Lane fields an old
// client still writes are reconciled with the providers (see
// reconcileLaneProviderFields), and a lane that arrives with no serving
// preferences keeps the ones stored for the same binding (keepingStoredUpstream).
func (cfg RoutingConfig) replacing(current RoutingConfig) (stored, served RoutingConfig, err error) {
	reconciled, err := cfg.reconcileLaneProviderFields(current)
	if err != nil {
		return RoutingConfig{}, RoutingConfig{}, err
	}
	return reconciled.keepingStoredUpstream(current).settle()
}

// withProviderSettings settles the document with one provider entry replaced.
func (cfg RoutingConfig) withProviderSettings(provider string, next ProviderSettings) (stored, served RoutingConfig, err error) {
	cfg = cfg.canonical()
	providers := maps.Clone(cfg.Providers)
	if providers == nil {
		providers = map[string]ProviderSettings{}
	}
	if next == (ProviderSettings{}) {
		delete(providers, provider)
	} else {
		providers[provider] = ProviderSettings{BaseURL: next.BaseURL, Upstream: next.Upstream.clone(), Location: next.Location}
	}
	if len(providers) == 0 {
		providers = nil
	}
	cfg.Providers = providers
	return cfg.settle()
}

// settle is what a write stores — the canonical document, so what a lane does
// not state stays unstated (an absent `routing` is not frozen into the product
// default) — and the finalized binding it serves.
//
// Unconfigured is a legitimate destination: an operator unbinding every model
// is choosing to run without AI, and it is the state a fresh installation is
// already in. Nothing is finalized for it; the entry's validator decides.
func (cfg RoutingConfig) settle() (stored, served RoutingConfig, err error) {
	stored = cfg.canonical()
	if cfg.Unconfigured() {
		return stored, stored, nil
	}
	if served, err = cfg.finalize(); err != nil {
		return RoutingConfig{}, RoutingConfig{}, cfg.routingRefusal(err)
	}
	return stored, served, nil
}

// keepingStoredUpstream carries each stored lane's serving preferences and
// thinking level onto the same lane of next when next declares none and binds
// the same provider and model. A value next declares always wins.
//
// A write that omits them would otherwise reset how the lane's model is served.
// Such a write comes from a settings seed, or from a client older than the
// `routing` or `thinking_level` field. The rebind in
// frontend/src/features/ai/ai-routing-fields.tsx applies the same rule from the
// routing editor's side. The pins are keyed on the model: a model older than a
// preference or a level refuses it. Carried onto another model, it would fail
// the lane with nothing in the form able to lift it.
//
// Both documents are read lifted, so the pins — the provider's — are never
// carried onto a lane, and the host is the provider's rather than part of the
// key. Preferences are carried only where next still serves the lane at the
// broker, and the embeddings lane, the one with a server of its own, keeps
// them only on the same server.
//
// A thinking level of thinkingLevelDefault is the explicit clear, as an empty
// `routing` object is for upstream preferences: it is stored as no level.
func (cfg RoutingConfig) keepingStoredUpstream(stored RoutingConfig) RoutingConfig {
	next, kept := cfg.canonical(), stored.canonical()
	carry := func(lane, keptLane ProviderConfig, prefsApply bool) ProviderConfig {
		cleared := lane.ThinkingLevel == thinkingLevelDefault
		if cleared {
			lane.ThinkingLevel = ""
		}
		if lane.Provider != keptLane.Provider || lane.Model != keptLane.Model {
			return lane
		}
		if lane.Routing == nil && prefsApply {
			lane.Routing = keptLane.Routing.clone()
		}
		if lane.ThinkingLevel == "" && !cleared {
			lane.ThinkingLevel = keptLane.ThinkingLevel
		}
		return lane
	}
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for tier, binding := range cfg.Tiers {
		tiers[tier] = carry(binding, kept.Tiers[tier], next.servedAtBroker(next.Tiers[tier]))
	}
	cfg.Tiers = tiers
	embeddings := next.Embeddings.ProviderConfig
	sameServer := sameEndpoint(embeddings.BaseURL, kept.Embeddings.BaseURL)
	cfg.Embeddings.ProviderConfig = carry(cfg.Embeddings.ProviderConfig, kept.Embeddings.ProviderConfig,
		sameServer && next.servedAtBroker(embeddings))
	return cfg
}

// servedAtBroker is whether a lane of this lifted document is served at
// OpenRouter: at its own server when it names one, else at its provider's.
func (cfg RoutingConfig) servedAtBroker(lane ProviderConfig) bool {
	host := cmp.Or(lane.BaseURL, cfg.Providers[lane.Provider].BaseURL)
	return UpstreamPreferencesApply(ProviderConfig{Provider: lane.Provider, BaseURL: host})
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
