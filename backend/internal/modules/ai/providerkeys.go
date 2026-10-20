// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Where a provider's BYOK key comes from.
//
// Each key is one stored secret setting. A boot source, usually the vendor's
// own environment variable, wins and locks it. Otherwise an administrator sets
// it under Settings -> AI, and the row holds only the key vault's ref to it.
// A boot value is read where it is and never copied into the vault.

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/platform/config"
)

// providerKeysObject is the RBAC object gating the BYOK credentials.
//
// The same object routing carries, deliberately: both are admin/ops-only on
// both verbs, and no seeded role holds one without the other. A second object
// would be a distinction the role matrix does not make — and every object costs
// a backfill migration that has to reach installations that already exist.
const providerKeysObject = routingSettingsObject

// providerKeyStore is how every BYOK key is stored: sealed, and kept through a
// data reset. A reset wipes an installation's records, not the credentials it
// reaches its vendors with.
var providerKeyStore = config.Store{
	RBACObject: providerKeysObject, AuditVerb: settingsAuditVerb, SecretReference: true, SurvivesDataReset: true,
}

// The BYOK keys, under each vendor's own variable name, so an operator already
// exporting one needs no extra wiring. None is required: a cloud binding
// without its key fails at construction, naming the variable (byokKeyRequired).
var (
	keyRoles = []config.Role{config.RoleAPI, config.RoleWorker}

	AnthropicKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_anthropic", Env: "ANTHROPIC_API_KEY", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "BYOK API key for the Anthropic provider; required only when a binding names it.",
	}, providerKeyStore)
	OpenAICompatibleKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_openai_compatible", Env: "OPENAI_COMPATIBLE_API_KEY", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "BYOK API key for the openai_compatible provider; required only when a binding names it.",
	}, providerKeyStore)
	OpenAIKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_openai", Env: "OPENAI_API_KEY", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "BYOK API key for the OpenAI provider; required only when a binding names it.",
	}, providerKeyStore)
	GeminiKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_gemini", Env: "GEMINI_API_KEY", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "BYOK API key for the Gemini provider; required only when a binding names it.",
	}, providerKeyStore)
	GeminiVertexKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_gemini_vertex", Env: "GEMINI_VERTEX_SA_JSON", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "BYOK service-account key (the JSON key file's contents) for the gemini_vertex provider; required only when a binding names it.",
	}, providerKeyStore)
	JevKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_jev", Env: "TYPESAFE_API_KEY", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "API key for the TypeSafe decision provider; required only when the decisions lane binds it.",
	}, providerKeyStore)
	JevCompatibleKey = config.DefineStored(config.Spec[string]{
		Key: "ai.provider_keys_jev_compatible", Env: "JEV_COMPATIBLE_API_KEY", Type: config.String, Secret: true, Roles: keyRoles,
		Doc: "API key for a jev_compatible decision server; sent when set and never required.",
	}, providerKeyStore)
)

// Settings are the BYOK keys, one per provider that takes one.
func Settings() []config.Any {
	keys := KeySettings()
	out := make([]config.Any, 0, len(keys))
	for _, s := range keys {
		out = append(out, s)
	}
	return out
}

// KeySettings are the BYOK keys in the routing table's order.
func KeySettings() []*config.Setting[string] {
	out := make([]*config.Setting[string], 0, len(keySettings))
	for _, provider := range CloudProvidersNeedingKeys() {
		out = append(out, keySettings[provider])
	}
	return out
}

// keySettings maps a provider to the setting its key is read through.
var keySettings = projectProviders(
	func(d providerDescriptor) *config.Setting[string] { return d.key },
	func(d providerDescriptor) bool { return d.key != nil })

// KeyLookup resolves each provider's key through its setting: the boot value
// where a boot source sets one, else the sealed row. It satisfies config.Lookup,
// so a client still asks for the variable a key arrives in and byokKeyRequired
// still names that variable.
//
// A key that cannot be read answers empty, and the binding then fails naming
// the variable to set. The read's error goes to log; it never holds the key.
func KeyLookup(ctx context.Context, cfg *config.Config, log *slog.Logger) config.Lookup {
	byEnv := make(map[string]*config.Setting[string], len(keySettings))
	for _, s := range keySettings {
		byEnv[s.Env()] = s
	}
	return func(name string) string {
		s, ok := byEnv[name]
		if !ok {
			return ""
		}
		key, err := config.StoredVal(ctx, cfg, s)
		if err != nil {
			log.WarnContext(ctx, "cannot read a provider key; the provider is treated as unkeyed", "setting", s.Key(), "error", err)
			return ""
		}
		return key
	}
}

// keyHeld reports whether a provider holds a key: one a boot source sets, or a
// sealed row. Neither is opened.
func keyHeld(ctx context.Context, cfg *config.Config, s *config.Setting[string]) (bool, error) {
	if _, locked := cfg.Locked(s.Key()); locked {
		return true, nil
	}
	_, stored, err := config.SealedRef(ctx, cfg, s)
	return stored, err
}

// CloudProvidersBound names the cloud vendors THIS binding actually calls, in a
// stable order.
//
// The intersection of what the binding names and what needs a credential, which
// is the only set worth asking about: a caller checking whether an installation
// can serve its own binding must not demand a key for a vendor it never uses,
// and must not miss the embeddings lane, which is bound separately from the
// chat tiers and is the half a per-tier walk forgets.
//
// Local providers (ollama, vllm, the offline fake) are absent by construction —
// they are not in cloudKeyEnv — so a fully local binding returns nothing and a
// sovereign installation is complete with no keys at all.
func (cfg RoutingConfig) CloudProvidersBound() []string {
	named := make(map[string]bool, len(cfg.Tiers)+1)
	for _, tier := range cfg.Tiers {
		named[tier.Provider] = true
	}
	named[cfg.Embeddings.Provider] = true
	// The decisions lane is bound apart from the tiers, so its key is named
	// here or nowhere. A key the adapter only sends when held is never demanded.
	if cfg.Decisions != nil && !keyIsOptional(cfg.Decisions.Provider) {
		named[cfg.Decisions.Provider] = true
	}

	out := make([]string, 0, len(named))
	for _, provider := range CloudProvidersNeedingKeys() {
		if named[provider] {
			out = append(out, provider)
		}
	}
	return out
}

// CloudProvidersNeedingKeys names the providers whose credential this module
// holds, in a stable order so a log line reads the same on every boot.
//
// Derived from cloudKeyEnv rather than restated, so a provider added there is
// covered by the key store the day it appears.
func CloudProvidersNeedingKeys() []string {
	out := make([]string, 0, len(cloudKeyEnv))
	for _, provider := range providerNames() {
		if _, cloud := cloudKeyEnv[provider]; cloud {
			out = append(out, provider)
		}
	}
	return out
}

// KeyEnvVarFor is the variable a provider's key arrives in, or "" for one that
// takes no key. The names match each vendor's own convention, which is why they
// carry no MARGINCE_ prefix.
func KeyEnvVarFor(provider string) string { return cloudKeyEnv[provider] }

// keyIsOptional reports whether provider calls without its key when none is
// held — a server on the operator's own host needs none — so a missing one is
// a state to show, never a gap that blocks setup.
func keyIsOptional(provider string) bool {
	d, _ := providerByName(provider)
	return d.keyOptional
}
