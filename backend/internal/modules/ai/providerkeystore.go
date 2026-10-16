// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Putting a BYOK key in, taking one out, and saying which vendors have one.
//
// It owns what a transport must not: the RBAC gate and the lock a boot-set key
// holds. It refuses a provider this build cannot serve, and a key it cannot
// use. Sealing, rotation and retiring the old blob are config.Save's.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProviderKeyStatus is what a reader may learn about one vendor's credential:
// that this build can serve it, and whether a key is held. Never the key.
type ProviderKeyStatus struct {
	// Provider is the routing name — the same string a binding uses.
	Provider string
	// Configured reports whether a credential is sealed for it.
	Configured bool
	// EnvVar is the variable the key may also arrive in, so a screen can tell
	// an operator which export seeded it.
	EnvVar string
	// Optional reports that the adapter calls without a key when none is held,
	// so an absent one is no gap.
	Optional bool
	// CredentialKind is which ProviderCredential field this vendor takes.
	CredentialKind string
	// PricedBy is the provider whose prices this one's unpriced models take,
	// so its sheet can show them; empty for one that borrows none.
	PricedBy string
}

// Usable reports whether this vendor can be called: a key is held, the adapter
// needs none, or it takes no key. The sync's scope and the badge both read it.
func (s ProviderKeyStatus) Usable() bool { return s.Configured || s.Optional || s.EnvVar == "" }

// The two credential kinds. A property of the vendor, so List answers it
// without opening the vault.
const (
	CredentialKindAPIKey         = "api_key"
	CredentialKindServiceAccount = "service_account"
)

func credentialKindFor(provider string) string {
	if d, _ := providerByName(provider); d.serviceAccountKey {
		return CredentialKindServiceAccount
	}
	return CredentialKindAPIKey
}

// ProviderCredential is what a caller sends to key one vendor: exactly the
// field credentialKindFor names.
type ProviderCredential struct {
	APIKey             string
	ServiceAccountJSON string
}

// ProviderKeyStore reads and changes the BYOK credentials.
//
// The workspace is not held here: the vault takes it off each request's
// context, the way every other tenant read resolves it.
type ProviderKeyStore struct {
	// cfg reads and saves each key's setting, and seals through the vault
	// compose gave it. One with no sealer is a role that seals nothing.
	cfg *config.Config
	// selectBrain builds the client a service-account key is checked through;
	// the zero value is SelectBrain.
	selectBrain brainSelector
	// profile reads the installation's stored profile, which decides whether a
	// key may be checked with its vendor at all.
	profile func(context.Context) (Profile, error)
}

// NewProviderKeyStore builds the store over cfg, which carries the database
// and, where the role has a vault, the sealer.
func NewProviderKeyStore(cfg *config.Config) *ProviderKeyStore {
	return &ProviderKeyStore{cfg: cfg, profile: func(ctx context.Context) (Profile, error) {
		binding, err := config.StoredVal(ctx, cfg, Routing)
		return binding.Profile, err
	}}
}

// ErrVaultUnavailable is the refusal when this installation seals nothing. It
// is apart from a validation error because the fix is the operator's vault root
// key, and not anything the caller sent.
var ErrVaultUnavailable = errors.New("this installation has no key vault configured, so a provider key cannot be stored: set the vault root key on the api and worker, then try again")

// List reports every cloud provider this build can serve and whether each holds
// a key, in the routing table's own order.
//
// It lists the providers rather than the stored keys, so a vendor with no
// credential is visible as something an admin could configure. A list of what
// happens to be stored would show an empty screen on the installation that most
// needs the screen.
func (s *ProviderKeyStore) List(ctx context.Context) ([]ProviderKeyStatus, error) {
	if err := auth.Require(ctx, providerKeysObject, principal.ActionRead); err != nil {
		return nil, err
	}
	providers := CloudProvidersNeedingKeys()
	out := make([]ProviderKeyStatus, 0, len(providers))
	for _, provider := range providers {
		held, err := keyHeld(ctx, s.cfg, keySettings[provider])
		if err != nil {
			return nil, err
		}
		out = append(out, ProviderKeyStatus{
			Provider:       provider,
			Configured:     held,
			EnvVar:         KeyEnvVarFor(provider),
			Optional:       keyIsOptional(provider),
			CredentialKind: credentialKindFor(provider),
			PricedBy:       pricedByFor(provider),
		})
	}
	return out, nil
}

// Set seals a key for one provider, replacing whatever was there. A key a boot
// source sets is refused as locked before the one sent is judged, so a caller
// learns the lock whatever they sent.
func (s *ProviderKeyStore) Set(ctx context.Context, provider string, sent ProviderCredential) error {
	if err := auth.Require(ctx, providerKeysObject, principal.ActionUpdate); err != nil {
		return err
	}
	setting, cloud := keySettings[provider]
	if !cloud {
		return keyRefused(provider, fmt.Sprintf("%q takes no credential — it is served without one", provider))
	}
	if _, err := config.LockedChange(s.cfg, setting, func(string) bool { return false }); err != nil {
		return err
	}
	secret, err := s.acceptedCredential(ctx, provider, sent)
	if err != nil {
		return err
	}
	if !s.cfg.Seals() {
		return ErrVaultUnavailable
	}
	return config.Save(ctx, s.cfg, setting, secret)
}

// LockFirst answers the lock of a boot-set key in place of err, a refusal of
// what was sent for it. The update gate comes first, as on a save, so a caller
// without the grant learns nothing of the lock.
func (s *ProviderKeyStore) LockFirst(ctx context.Context, provider string, err error) error {
	setting, cloud := keySettings[provider]
	if err == nil || !cloud {
		return err
	}
	if gateErr := auth.Require(ctx, providerKeysObject, principal.ActionUpdate); gateErr != nil {
		return gateErr
	}
	return s.cfg.LockFirst(setting.Key(), err)
}

// acceptedCredential is the one field this provider takes, trimmed, or a
// refusal naming what was wrong without repeating any of it.
//
// Trimmed because a pasted credential arrives with whatever whitespace the
// clipboard carried, and a key with a trailing newline authenticates nothing
// while looking exactly like one that would.
func (s *ProviderKeyStore) acceptedCredential(ctx context.Context, provider string, sent ProviderCredential) (string, error) {
	apiKey, keyFile := strings.TrimSpace(sent.APIKey), strings.TrimSpace(sent.ServiceAccountJSON)
	serviceAccount := credentialKindFor(provider) == CredentialKindServiceAccount
	switch {
	case apiKey != "" && keyFile != "":
		return "", keyRefused(provider, "send api_key or service_account_json, not both")
	case serviceAccount && apiKey != "":
		return "", keyRefused(provider, fmt.Sprintf("%q takes a service_account_json — the service-account key file — not an api_key", provider))
	case !serviceAccount && keyFile != "":
		return "", keyRefused(provider, fmt.Sprintf("%q takes an api_key, not a service_account_json", provider))
	case serviceAccount && keyFile == "":
		return "", keyRefused(provider, "the service-account key is empty — remove the credential instead of storing nothing")
	case !serviceAccount && apiKey == "":
		return "", keyRefused(provider, "the api key is empty — remove the credential instead of storing nothing")
	case !serviceAccount:
		return apiKey, nil
	}
	// Checking the key is a call to Google, which sovereign forbids like any
	// other; and under it no gemini_vertex binding could use the key anyway.
	profile, err := s.profile(ctx)
	if err != nil {
		return "", err
	}
	if profile == ProfileSovereign {
		return "", keyRefused(provider, fmt.Sprintf("profile sovereign forbids reaching Google, so a %q key cannot be checked or used — change the profile first", provider))
	}
	if err := s.verifyServiceAccount(ctx, keyFile); err != nil {
		return "", keyRefused(provider, err.Error())
	}
	return keyFile, nil
}

// verifyServiceAccount parses the key file and has Google exchange it for an
// access token, through a client SelectBrain builds over the candidate alone.
// A key that cannot mint a token would only fail later, on a model call.
func (s *ProviderKeyStore) verifyServiceAccount(ctx context.Context, keyFile string) error {
	candidate := config.Static(map[string]string{cloudKeyEnv[providerGeminiVertex]: keyFile})
	client, err := s.selectBrain.build(ProviderConfig{Provider: providerGeminiVertex, Location: vertexMetadataLocation}, candidate)
	if err != nil {
		return err
	}
	vertex, ok := vertexOf(client)
	if !ok {
		return errors.New("ai: gemini_vertex: the key could not be checked")
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	if err := vertex.verifyCredential(asked); err != nil {
		return fmt.Errorf("the service-account key was not accepted by Google: %w", err)
	}
	return nil
}

// keyRefused names the provider's own setting as the field at fault, or the
// provider word itself for one that takes no key.
func keyRefused(provider, reason string) error {
	field := provider
	if setting, cloud := keySettings[provider]; cloud {
		field = setting.Key()
	}
	return settings.InvalidValue{Setting: field, Code: settings.CodeInvalidValue, Reason: reason}
}

// Remove retires a provider's credential. Removing one that was never there
// succeeds: the caller asked for a state, and that state already holds. A key
// a boot source sets is locked, and its removal refused.
func (s *ProviderKeyStore) Remove(ctx context.Context, provider string) error {
	if err := auth.Require(ctx, providerKeysObject, principal.ActionUpdate); err != nil {
		return err
	}
	setting, cloud := keySettings[provider]
	if !cloud {
		return nil
	}
	if !s.cfg.Seals() {
		return ErrVaultUnavailable
	}
	return config.RemoveSecret(ctx, s.cfg, setting)
}
