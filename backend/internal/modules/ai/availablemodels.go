// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	// AvailabilityNoEndpoint means there is no address to ask: an OpenAI-wire
	// binding with no host, or a gemini_vertex one with no location. A probe
	// also answers it for a location that does not serve the model.
	AvailabilityNoEndpoint ModelAvailability = "no_endpoint"
)

// AvailableModel is one model this read reports, widened past the
// three-field model.Info with what only a broker publishes on the same
// endpoint: a price and, for the one vendor with a published benchmark, a
// rank score. Every vendor's answer is carried in this shape; model.Info
// itself stays three fields, because that is what every OTHER adapter's list
// endpoint can honestly promise.
type AvailableModel struct {
	model.Info
	// ContextLength is absent where the vendor publishes none.
	ContextLength *int
	// InputPerMtok and OutputPerMtok are the vendor's own asking price, in
	// USD-per-million-tokens decimal strings, absent where the vendor
	// publishes no price.
	InputPerMtok, OutputPerMtok *string
	// CacheReadPerMtok and CacheWritePerMtok are the vendor's prompt-cache
	// prices in the same unit, absent where it publishes none. Only the full
	// view carries them: they exist for the rate refresh, not for a picker.
	CacheReadPerMtok, CacheWritePerMtok *string
	// RankScore is this model's score under AvailableModels.RankedBy, absent
	// where the list is not ranked.
	RankScore *string
}

// AvailableModels is one vendor's answer.
type AvailableModels struct {
	Provider string
	Models   []AvailableModel
	// RankedBy names the measure Models is sorted by, and is empty when the
	// list is in the vendor's own order — only a vendor that publishes a
	// benchmark (OpenRouter) can ever set this.
	RankedBy string
	// Unavailable is empty when the vendor answered, and names the state when
	// it did not. Models is empty whenever this is set.
	Unavailable ModelAvailability
	// Complete is true when Models is exactly what the asked place serves, so
	// a picker offers nothing beside it.
	Complete bool
}

// ListAvailableModels asks one vendor what it serves.
//
// The provider and the lane are named; the endpoint is not. Both are closed
// vocabularies — the adapter names this build accepts, and the lanes the stored
// document already binds — and the host is read from that document or from the
// adapter's compiled default, never from the request. A URL accepted here would
// be a destination chosen by a caller holding only READ on this setting, while
// the stored one had to be written by someone holding update and passed the
// endpoint rule on the way in (outboundegress.go). A gemini_vertex location
// is no exception: it picks one of vertexHost's three templates, never a host.
//
// The host is the provider's, so the lane matters only for the embeddings
// lane, which may sit on a server of its own (providerConfigFor).
//
// top asks for a shortlist: honoured only for the one vendor (OpenRouter) that
// publishes the benchmark it ranks by. Every other vendor answers its full
// list regardless, per the contract's own description of `top` — a caller
// that needs the distinction reads AvailableModels.RankedBy, never top itself.
func (s *RoutingStore) ListAvailableModels(ctx context.Context, q AvailableModelsQuery) (AvailableModels, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return AvailableModels{}, err
	}
	vertex := q.Provider == providerGeminiVertex
	if vertex && q.Location != "" && !vertexLocationShape.MatchString(q.Location) {
		return AvailableModels{}, fmt.Errorf("%w: location must be eu, us, global, or a region such as europe-west4", apperrors.ErrInvalidArgument)
	}
	if vertex && q.Model != "" && !vertexModelShape.MatchString(q.Model) {
		return AvailableModels{}, fmt.Errorf("%w: model must be a publisher model id such as gemini-3.5-flash", apperrors.ErrInvalidArgument)
	}
	cfg, err := s.Get(ctx)
	if err != nil {
		return AvailableModels{}, err
	}
	return s.availableModels(ctx, cfg, q), nil
}

// AvailableModelsQuery names what ListAvailableModels is asked. Location and
// Model apply to gemini_vertex alone: Location replaces the lane's stored one,
// and Model turns the list into a probe of that one id.
type AvailableModelsQuery struct {
	Provider, Tier  string
	Top             int
	Location, Model string
}

func (s *RoutingStore) availableModels(ctx context.Context, cfg RoutingConfig, q AvailableModelsQuery) AvailableModels {
	provider := q.Provider
	out := AvailableModels{Provider: provider}
	bound := providerConfigFor(cfg, provider, q.Tier)
	if provider == providerGeminiVertex && q.Location != "" {
		bound.Location = q.Location
	}
	if out.Unavailable = boundListRefusal(cfg.Profile, bound); out.Unavailable != AvailabilityOK {
		return out
	}
	if isDecisionProvider(provider) {
		return s.listDecisionModels(ctx, cfg, provider)
	}
	if q.Model != "" {
		return s.probeAvailability(ctx, bound, q)
	}
	// OpenRouter publishes its list unauthenticated and unbound: there is no
	// stored binding to resolve a host from, and SelectBrain knows no adapter
	// by this name, so it is asked directly rather than through the bound
	// path below.
	if provider == openRouterProvider {
		if s.catalogue == nil {
			out.Unavailable = AvailabilityNotPublished
			return out
		}
		return s.catalogue.List(ctx, q.Top)
	}
	client, err := s.selectBrain.build(bound, s.resolvedKeys(ctx))
	if err != nil {
		out.Unavailable = unavailableFor(err)
		return out
	}
	lister, ok := client.(model.Lister)
	if !ok {
		out.Unavailable = AvailabilityNotPublished
		return out
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	models, err := lister.ListModels(asked)
	if err != nil {
		// The vendor's own words stay out of the answer. A reader is choosing a
		// model, not debugging our HTTP, and the vendor's message on this
		// endpoint is as often a proxy's HTML as it is a sentence.
		out.Unavailable = AvailabilityUnreachable
		return out
	}
	if gemini, vertex := client.(*geminiClient); vertex && provider == providerGeminiVertex {
		models, out.Complete = s.servedOnly(ctx, gemini, bound.Location, models)
	}
	out.Models = make([]AvailableModel, len(models))
	for i, m := range models {
		out.Models[i] = AvailableModel{Info: m}
	}
	return out
}

// isKeyFault reports whether the held credential, not the vendor, is why a
// provider cannot be asked: no key, or a key that cannot be used.
func isKeyFault(err error) bool {
	return errors.Is(err, errNoProviderKey) || errors.Is(err, errInvalidServiceAccount)
}

// listRefusal is why provider's list is not asked for at all, or
// AvailabilityOK when it may be.
//
// The profile decides where inference may happen, and a list call is egress
// like any other. Refused for the same reason a binding is refused at save
// time: a sovereign installation must not reach a cloud vendor, and
// discovering that at the first call is too late. OpenRouter is cloud egress
// like any other broker, so it is refused too rather than falling through to
// the unauthenticated read. The registry's own local flag decides, so a local
// decision adapter is not mistaken for a cloud one.
func listRefusal(profile Profile, provider string) ModelAvailability {
	d, _ := providerByName(provider)
	if profile == ProfileSovereign && !d.local && !d.localByEndpoint {
		return AvailabilityProfileForbids
	}
	return AvailabilityOK
}

// boundListRefusal is listRefusal asked of one binding: a Vertex binding with
// no location has no host to ask.
func boundListRefusal(profile Profile, bound ProviderConfig) ModelAvailability {
	if bound.Provider == providerGeminiVertex && bound.Location == "" {
		return AvailabilityNoEndpoint
	}
	return listRefusal(profile, bound.Provider)
}

// isDecisionProvider is whether provider answers the decision wire rather than
// a chat one, which decides how it is listed and tested.
func isDecisionProvider(provider string) bool {
	d, _ := providerByName(provider)
	return d.caps.has(capDecision)
}

// listDecisionModels asks a decision endpoint what it serves, at the lane it
// would serve. A host that publishes no list answers not_published, and the
// price sheet's rows stay its suggestions.
func (s *RoutingStore) listDecisionModels(ctx context.Context, cfg RoutingConfig, provider string) AvailableModels {
	out := AvailableModels{Provider: provider}
	lane := boundDecisionLane(cfg, provider)
	client, err := selectDecider(lane, s.resolvedKeys(ctx))
	if err != nil {
		out.Unavailable = unavailableFor(err)
		return out
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	models, listed, err := client.decisionModels(asked, provider)
	switch {
	case !listed:
		out.Unavailable = AvailabilityNotPublished
	case err != nil:
		out.Unavailable = AvailabilityUnreachable
	default:
		out.Models = make([]AvailableModel, len(models))
		for i, m := range models {
			out.Models[i] = AvailableModel{Info: m}
		}
	}
	return out
}

// unavailableFor reads why a binding could not be turned into a client.
//
// The two states a reader can act on are told apart: a vendor with no
// usable credential is a key to paste, and an OpenAI-wire binding with no host
// is an address to fill in.
//
// Everything else is a name this surface cannot ask AT ALL — an adapter that
// does not exist, or a binding with no provider on it — and that is
// `not_published` rather than `unreachable`. The difference is not cosmetic:
// `unreachable` says the vendor was asked and did not answer, which would have
// a reader chasing a network fault for a provider nothing ever called.
func unavailableFor(err error) ModelAvailability {
	if isKeyFault(err) {
		return AvailabilityNoKey
	}
	if errors.Is(err, errNoBaseURL) {
		return AvailabilityNoEndpoint
	}
	return AvailabilityNotPublished
}

// providerConfigFor is where an availability read asks `provider`: its own
// host and Vertex location, read through the lift so a document still in the
// per-lane shape gives the same answer every time. No lane decides it, with one
// exception: the embeddings lane may sit on a server or at a location of its
// own, and a picker opened on that lane asks there. No model id: this asks what
// the vendor serves.
func providerConfigFor(cfg RoutingConfig, provider, lane string) ProviderConfig {
	lifted := cfg.canonical()
	settings := lifted.Providers[provider]
	out := ProviderConfig{Provider: provider, BaseURL: settings.BaseURL, Location: settings.Location}
	if embeddings := lifted.Embeddings; lane == string(LaneEmbeddings) && embeddings.Provider == provider {
		out.BaseURL = cmp.Or(embeddings.BaseURL, out.BaseURL)
		out.Location = cmp.Or(embeddings.Location, out.Location)
	}
	return out
}
