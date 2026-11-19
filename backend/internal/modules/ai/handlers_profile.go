// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PublicProfile is the deliberately small pre-auth view of the process's AI
// posture. It describes the bound configuration, never provider health.
type PublicProfile struct {
	State            crmcontracts.AssistantProfileState
	InferenceMode    crmcontracts.AssistantProfileInferenceMode
	Providers        []crmcontracts.AssistantProfileProviders
	ConfiguredModels []crmcontracts.AssistantConfiguredModel
}

// NewPublicProfile derives the anonymous view from the same validated routing
// config the process used to construct its model path. Fake is a development
// mechanism, not a provider identity, so it never crosses this boundary.
func NewPublicProfile(runtimeState string, cfg RoutingConfig) PublicProfile {
	switch runtimeState {
	case "fake":
		return PublicProfile{
			State:            crmcontracts.AssistantProfileStateDevelopment,
			InferenceMode:    crmcontracts.AssistantProfileInferenceModeDevelopment,
			Providers:        []crmcontracts.AssistantProfileProviders{},
			ConfiguredModels: []crmcontracts.AssistantConfiguredModel{},
		}
	case "configured":
		providers := publicProviders(cfg)
		return PublicProfile{
			State:            crmcontracts.AssistantProfileStateConfigured,
			InferenceMode:    publicInferenceMode(providers),
			Providers:        providers,
			ConfiguredModels: publicConfiguredModels(cfg),
		}
	default:
		return PublicProfile{
			State:            crmcontracts.AssistantProfileStateUnconfigured,
			InferenceMode:    crmcontracts.AssistantProfileInferenceModeNone,
			Providers:        []crmcontracts.AssistantProfileProviders{},
			ConfiguredModels: []crmcontracts.AssistantConfiguredModel{},
		}
	}
}

func publicConfiguredModels(cfg RoutingConfig) []crmcontracts.AssistantConfiguredModel {
	tiers := make([]Tier, 0, len(cfg.Tiers))
	for tier := range cfg.Tiers {
		tiers = append(tiers, tier)
	}
	slices.Sort(tiers)
	models := make([]crmcontracts.AssistantConfiguredModel, 0, len(tiers))
	for _, tier := range tiers {
		binding := cfg.Tiers[tier]
		provider, ok := publicProvider(binding.Provider)
		if !ok {
			continue
		}
		modelID := binding.Model
		if fallback := providerDefaultModel(binding.Provider); fallback != "" {
			modelID = defaulted(modelID, fallback)
		}
		models = append(models, crmcontracts.AssistantConfiguredModel{
			Tier:     crmcontracts.AssistantConfiguredModelTier(tier),
			Provider: crmcontracts.AssistantConfiguredModelProvider(provider),
			Model:    modelID,
		})
	}
	return models
}

func publicProviders(cfg RoutingConfig) []crmcontracts.AssistantProfileProviders {
	set := make(map[crmcontracts.AssistantProfileProviders]struct{}, len(cfg.Tiers)+1)
	for _, binding := range cfg.Tiers {
		if provider, ok := publicProvider(binding.Provider); ok {
			set[provider] = struct{}{}
		}
	}
	if provider, ok := publicProvider(cfg.Embeddings.Provider); ok {
		set[provider] = struct{}{}
	}
	providers := make([]crmcontracts.AssistantProfileProviders, 0, len(set))
	for provider := range set {
		providers = append(providers, provider)
	}
	slices.Sort(providers)
	return providers
}

func publicProvider(provider string) (crmcontracts.AssistantProfileProviders, bool) {
	d, ok := providerByName(provider)
	if !ok || !d.public {
		return "", false
	}
	return crmcontracts.AssistantProfileProviders(d.name), true
}

func publicInferenceMode(providers []crmcontracts.AssistantProfileProviders) crmcontracts.AssistantProfileInferenceMode {
	if len(providers) == 0 {
		return crmcontracts.AssistantProfileInferenceModeDevelopment
	}
	local, cloud := false, false
	for _, provider := range providers {
		if ProviderIsLocal(string(provider)) {
			local = true
		} else {
			cloud = true
		}
	}
	switch {
	case local && cloud:
		return crmcontracts.AssistantProfileInferenceModeHybrid
	case local:
		return crmcontracts.AssistantProfileInferenceModeLocal
	default:
		return crmcontracts.AssistantProfileInferenceModeCloud
	}
}

// WithPublicProfile binds the view to the transport surface. It is read per
// request, so a rebind reaches it.
func (h Handlers) WithPublicProfile(profile func() PublicProfile) Handlers {
	h.publicProfile = profile
	return h
}

// GetAssistantProfile implements (GET /assistant/profile).
func (h Handlers) GetAssistantProfile(w http.ResponseWriter, _ *http.Request) {
	profile := h.publicProfile()
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AssistantProfile{
		Name: crmcontracts.AssistantProfileNameMargince, Kind: crmcontracts.AssistantProfileKindAi,
		State: profile.State, InferenceMode: profile.InferenceMode,
		Providers: profile.Providers,
	})
}

// GetAiProfile implements (GET /ai/profile). The detailed deployment posture
// shares the operational-config grant used by AI calls and usage; the public
// assistant profile remains the deliberately smaller anonymous surface.
func (h Handlers) GetAiProfile(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r.Context(), "automation", principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	profile := h.publicProfile()
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AiProfile{
		Name: crmcontracts.AiProfileNameMargince, Kind: crmcontracts.AiProfileKindAi,
		State:            crmcontracts.AiProfileState(profile.State),
		InferenceMode:    crmcontracts.AiProfileInferenceMode(profile.InferenceMode),
		Providers:        makeAiProfileProviders(profile.Providers),
		ConfiguredModels: profile.ConfiguredModels,
	})
}

func makeAiProfileProviders(in []crmcontracts.AssistantProfileProviders) []crmcontracts.AiProfileProviders {
	out := make([]crmcontracts.AiProfileProviders, len(in))
	for i, provider := range in {
		out[i] = crmcontracts.AiProfileProviders(provider)
	}
	return out
}
