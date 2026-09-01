// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// What one vendor says it serves today, for the screen that binds a lane to it.
//
// The picker used to offer the price sheet and nothing else, so it could only
// ever show what somebody had last hand-entered — a model released last month
// was simply absent, and a reader concluded the product could not reach it. The
// vendor is the authority on what it serves, so the vendor is asked.
//
// The sheet stays the authority on COST. A model listed here that the sheet
// cannot price is bindable and reports UNPRICED, which is the honest reading:
// the call will work and we cannot yet say what it charged.

// listTimeout bounds one vendor round-trip. A picker is opened interactively
// and a reader is waiting, so a vendor that is slow must degrade to the sheet's
// suggestions rather than hold the field — and a local ollama that is not
// running must fail in a moment rather than at the adapter's call ceiling,
// which is measured in minutes.
const listTimeout = 5 * time.Second

// ModelAvailability says why a vendor's list is empty, for the cases that are
// a STATE of the installation rather than a fault.
//
// A closed vocabulary rather than an error, because none of these should break
// the form: the field still binds any id the reader types, and the screen
// degrades to the price sheet's suggestions while saying which of these is why.
// An error here would turn "your local ollama is not running" into a settings
// page that cannot be read.
type ModelAvailability string

const (
	// AvailabilityOK means the vendor answered.
	AvailabilityOK ModelAvailability = ""
	// AvailabilityNoKey means this vendor takes a credential and holds none, so
	// there is nothing to ask with.
	AvailabilityNoKey ModelAvailability = "no_key"
	// AvailabilityProfileForbids means the deployment profile does not permit
	// reaching this vendor at all — asking would be the egress the profile
	// exists to prevent.
	AvailabilityProfileForbids ModelAvailability = "profile_forbids"
	// AvailabilityNotPublished means the adapter has no list endpoint to call.
	AvailabilityNotPublished ModelAvailability = "not_published"
	// AvailabilityUnreachable means the vendor was asked and did not answer.
	AvailabilityUnreachable ModelAvailability = "unreachable"
	// AvailabilityNoEndpoint means an OpenAI-wire binding names no host, so
	// there is no address to ask. Only openai_compatible can be in this state:
	// every other adapter has a compiled default.
	AvailabilityNoEndpoint ModelAvailability = "no_endpoint"
)

// AvailableModels is one vendor's answer.
type AvailableModels struct {
	Provider string
	Models   []model.Info
	// Unavailable is empty when the vendor answered, and names the state when
	// it did not. Models is empty whenever this is set.
	Unavailable ModelAvailability
}

// ListAvailableModels asks one vendor what it serves.
//
// The provider and the LANE are named; the endpoint is not. Both are closed
// vocabularies — the adapter names this build accepts, and the tiers the stored
// document already binds — and the host is read from that document or from the
// adapter's compiled default, never from the request. The AI outbound client
// carries no egress allowlist, so accepting a URL here would let anyone holding
// this grant point the server at an arbitrary host.
//
// The lane matters because one vendor may be bound at two hosts: a broker on one
// tier and a self-hosted gateway on another is a configuration the routing
// validator permits. Asked without it, this read answered for whichever host it
// happened to find, which is not necessarily the one the lane being edited
// points at.
func (s *RoutingStore) ListAvailableModels(
	ctx context.Context,
	provider, tier string,
) (AvailableModels, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return AvailableModels{}, err
	}
	cfg, err := s.Get(ctx)
	if err != nil {
		return AvailableModels{}, err
	}
	out := AvailableModels{Provider: provider}
	// The profile decides where inference may happen, and a list call is egress
	// like any other. Refused here for the same reason a binding is refused at
	// save time: a sovereign installation must not reach a cloud vendor, and
	// discovering that at the first call is too late.
	if cfg.Profile == ProfileSovereign && !ProviderIsLocal(provider) {
		out.Unavailable = AvailabilityProfileForbids
		return out, nil
	}
	client, err := SelectBrain(boundProviderConfig(cfg, provider, tier), s.resolvedKeys(ctx))
	if err != nil {
		out.Unavailable = unavailableFor(err)
		return out, nil
	}
	lister, ok := client.(model.Lister)
	if !ok {
		out.Unavailable = AvailabilityNotPublished
		return out, nil
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	models, err := lister.ListModels(asked)
	if err != nil {
		// The vendor's own words stay out of the answer. A reader is choosing a
		// model, not debugging our HTTP, and the vendor's message on this
		// endpoint is as often a proxy's HTML as it is a sentence.
		out.Unavailable = AvailabilityUnreachable
		return out, nil
	}
	out.Models = models
	return out, nil
}

// unavailableFor reads why a binding could not be turned into a client.
//
// The two states a reader can act on are told apart: a vendor with no
// credential is a key to paste, and an OpenAI-wire binding with no host is an
// address to fill in.
//
// Everything else is a name this surface cannot ask AT ALL — an adapter that
// does not exist, or a binding with no provider on it — and that is
// `not_published` rather than `unreachable`. The difference is not cosmetic:
// `unreachable` says the vendor was asked and did not answer, which would have
// a reader chasing a network fault for a provider nothing ever called.
func unavailableFor(err error) ModelAvailability {
	if errors.Is(err, errNoProviderKey) {
		return AvailabilityNoKey
	}
	if errors.Is(err, errNoBaseURL) {
		return AvailabilityNoEndpoint
	}
	return AvailabilityNotPublished
}

// boundProviderConfig is the binding this installation would call `provider`
// with, as far as an availability read needs it.
//
// The LANE decides, where the caller named one: a vendor bound at two hosts is
// asked at the one the lane being edited points at, rather than at whichever the
// map yielded. A lane that names a different vendor — the reader has just
// re-pointed it and has not saved — falls through to the rest, because the
// question is about the vendor.
//
// Failing that, the tiers are walked in a FIXED order. Which of two hosts wins
// is then arbitrary; that the same one wins every time is not, and Go randomises
// map iteration, so ranging the map directly changed the picker's list under a
// reader who had touched nothing.
//
// A vendor the routing does not name at all falls back to the adapter's compiled
// default, which is what makes the picker useful before anything is bound. No
// model id: this asks what the vendor serves, not what one lane names.
func boundProviderConfig(cfg RoutingConfig, provider, tier string) ProviderConfig {
	if binding, ok := cfg.Tiers[Tier(tier)]; ok &&
		binding.Provider == provider && binding.BaseURL != "" {
		return ProviderConfig{Provider: provider, BaseURL: binding.BaseURL}
	}
	if tier == string(LaneEmbeddings) &&
		cfg.Embeddings.Provider == provider && cfg.Embeddings.BaseURL != "" {
		return ProviderConfig{Provider: provider, BaseURL: cfg.Embeddings.BaseURL}
	}
	for _, t := range sortedTiers(cfg.Tiers) {
		binding := cfg.Tiers[t]
		if binding.Provider == provider && binding.BaseURL != "" {
			return ProviderConfig{Provider: provider, BaseURL: binding.BaseURL}
		}
	}
	if cfg.Embeddings.Provider == provider && cfg.Embeddings.BaseURL != "" {
		return ProviderConfig{Provider: provider, BaseURL: cfg.Embeddings.BaseURL}
	}
	return ProviderConfig{Provider: provider}
}

// sortedTiers is the tier names in a stable order.
//
// Sorted rather than ranked by the cost ladder: this picks WHICH HOST to ask a
// vendor about, and the ladder ranks how capable a lane is, which says nothing
// about that. A rank borrowed here would read as a preference the code does not
// actually hold, and would be a second copy of an order that lives in the task
// contract.
func sortedTiers(tiers map[Tier]ProviderConfig) []Tier {
	out := make([]Tier, 0, len(tiers))
	for tier := range tiers {
		out = append(out, tier)
	}
	slices.Sort(out)
	return out
}
