// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Whether a stored credential works, asked of the vendor that issued it.
//
// The probe is the same model-list call the picker makes, because that is the
// cheapest authenticated request every listing vendor serves and it spends no
// tokens. What differs from ListAvailableModels is the answer: the picker folds
// every vendor failure into `unreachable`, since a reader choosing a model has
// nothing to do about one, while a reader testing a key needs "refused" told
// apart from "down" — that distinction is the whole reason to press Test.

// KeyTestReason is why a key test did not pass, as a closed vocabulary: the
// vendor's own words stay out of the answer, since they are as often a proxy's
// HTML as they are a sentence.
type KeyTestReason string

const (
	// KeyTestNoKey means the vendor takes a credential and holds none.
	KeyTestNoKey KeyTestReason = "no_key"
	// KeyTestProfileForbids means the profile forbids reaching the vendor.
	KeyTestProfileForbids KeyTestReason = "profile_forbids"
	// KeyTestNotPublished means the vendor has no list endpoint to test against.
	KeyTestNotPublished KeyTestReason = "not_published"
	// KeyTestNoEndpoint means an OpenAI-wire vendor no binding gives a host yet.
	KeyTestNoEndpoint KeyTestReason = "no_endpoint"
	// KeyTestAuthFailed means the vendor refused the credential.
	KeyTestAuthFailed KeyTestReason = "auth_failed"
	// KeyTestPermissionDenied means the vendor accepted the credential and
	// refused the call: a missing role or an API the account has not enabled.
	KeyTestPermissionDenied KeyTestReason = "permission_denied"
	// KeyTestRateLimited means the vendor is throttling the credential, which
	// says nothing about whether it is valid.
	KeyTestRateLimited KeyTestReason = "rate_limited"
	// KeyTestUnreachable means the vendor did not answer, or answered with
	// something this reading does not recognise.
	KeyTestUnreachable KeyTestReason = "unreachable"
)

// KeyTest is one vendor's answer to the stored credential. Reason is empty
// exactly when OK is true. Counted says ModelCount is a real list's length: a
// vendor tested without listing (a broker's key endpoint, the decision wire)
// passes with no count rather than with a zero. Unconfirmed marks a pass that
// proves the server answered but not that it checked the key: the decision
// wire's empty request, which a server may refuse before it reads the key.
type KeyTest struct {
	Provider    string
	OK          bool
	ModelCount  int
	Counted     bool
	Unconfirmed bool
	Reason      KeyTestReason
}

// clientBuilder turns a binding into a client; SelectBrain in production, and
// a transport-injected twin in a test, whose stub listens where the egress
// guard refuses to dial. deciderBuilder is the same seam for a decision lane.
type (
	clientBuilder  func(ProviderConfig, config.Lookup) (model.Client, error)
	deciderBuilder func(DecisionsConfig, config.Lookup) (*decisionClient, error)
)

// keyProbes is the pair of builders a key test may use.
type keyProbes struct {
	brain   clientBuilder
	decider deciderBuilder
}

// TestProviderKey asks provider's vendor whether the stored credential works.
//
// READ on ai_routing, the grant ListAvailableModels makes the identical vendor
// call under: a stricter grant here would be a wall with a door beside it.
func (s *RoutingStore) TestProviderKey(ctx context.Context, provider string) (KeyTest, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return KeyTest{}, err
	}
	cfg, err := s.Get(ctx)
	if err != nil {
		return KeyTest{}, err
	}
	tested := probeProviderKey(ctx, cfg, provider, s.resolvedKeys(ctx), keyProbes{SelectBrain, selectDecider})
	if tested.OK && !tested.Unconfirmed {
		// The vendor just answered with this key: whatever the tracker holds
		// against the provider is out of date, and waiting for the next call to
		// find that out would keep a fixed provider reported down.
		sharedProviderHealth.forget(provider)
	}
	return tested, nil
}

// probeProviderKey is the test itself, over a routing document already read.
//
// The host is the provider's: a key belongs to the vendor, not to one lane, so
// it is tried where the provider is configured, or at the adapter's default.
func probeProviderKey(
	ctx context.Context,
	cfg RoutingConfig,
	provider string,
	keys config.Lookup,
	build keyProbes,
) KeyTest {
	out := KeyTest{Provider: provider}
	if refused := listRefusal(cfg.Profile, provider); refused != AvailabilityOK {
		out.Reason = keyTestRefusal(refused)
		return out
	}
	if isDecisionProvider(provider) {
		return probeDecisionKey(ctx, cfg, provider, keys, build.decider)
	}
	binding := providerConfigFor(cfg, provider, "")
	// A Vertex key is tested on Google's global host, which the model list
	// reaches whatever location a lane names: a key is the project's, not one
	// location's.
	if provider == providerGeminiVertex {
		binding.Location = vertexMetadataLocation
	}
	client, err := build.brain(binding, keys)
	if err != nil {
		out.Reason = keyTestRefusal(unavailableFor(err))
		return out
	}
	lister, ok := client.(model.Lister)
	if !ok {
		out.Reason = KeyTestNotPublished
		return out
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	models, err := lister.ListModels(asked)
	if err != nil {
		out.Reason = vendorKeyTestFailure(provider, err)
		return out
	}
	out.OK, out.ModelCount, out.Counted = true, len(models), true
	return out
}

// probeDecisionKey tests a decision adapter's key at the lane it would serve:
// the stored binding when it names this adapter, else the adapter's default.
func probeDecisionKey(
	ctx context.Context,
	cfg RoutingConfig,
	provider string,
	keys config.Lookup,
	build deciderBuilder,
) KeyTest {
	out := KeyTest{Provider: provider}
	lane := boundDecisionLane(cfg, provider)
	if decisionLaneForbidden(cfg.Profile, lane) {
		out.Reason = KeyTestProfileForbids
		return out
	}
	client, err := build(lane, keys)
	if err != nil {
		out.Reason = keyTestRefusal(unavailableFor(err))
		return out
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	count, counted, err := client.probeKey(asked, provider)
	if err != nil {
		out.Reason = vendorKeyTestFailure(provider, err)
		return out
	}
	out.OK, out.ModelCount, out.Counted = true, count, counted
	out.Unconfirmed = decisionHostFor(provider, client.url) == decisionHostWire
	return out
}

// keyTestRefusal carries a state the picker already names across to this
// vocabulary. Spelled out rather than converted, so renaming either side is a
// compile-visible change here and not a silent new wire value.
func keyTestRefusal(state ModelAvailability) KeyTestReason {
	switch state {
	case AvailabilityNoKey:
		return KeyTestNoKey
	case AvailabilityProfileForbids:
		return KeyTestProfileForbids
	case AvailabilityNoEndpoint:
		return KeyTestNoEndpoint
	case AvailabilityUnreachable:
		return KeyTestUnreachable
	default:
		return KeyTestNotPublished
	}
}

// geminiKeyInvalid is the ErrorInfo reason Google APIs give a bad API key.
const geminiKeyInvalid = "API_KEY_INVALID"

// keyTestFailure reads a vendor's refusal. Only the status is trusted: 401 is
// the credential, 403 the account's permissions, 429 the throttle, and else —
// a timeout, a 5xx, a 404 from a host that is not the vendor — is unreachable.
//
// Gemini is the exception: it answers an invalid key with 400 and names it
// API_KEY_INVALID in the error's structured details. The code, not the status,
// decides — a 400 from a proxy in front of it is not a refused key.
// vendorKeyTestFailure keeps a 403 a refused key at an endpoint an operator runs:
// only a named vendor's 403 reliably means the key was accepted.
func vendorKeyTestFailure(provider string, err error) KeyTestReason {
	reason := keyTestFailure(err)
	if d, _ := providerByName(provider); reason == KeyTestPermissionDenied && d.egress == egressOperatorEndpoint {
		return KeyTestAuthFailed
	}
	return reason
}

func keyTestFailure(err error) KeyTestReason {
	var refused *listStatusError
	if !errors.As(err, &refused) {
		return KeyTestUnreachable
	}
	if refused.vendor == geminiListVendor && refused.status == http.StatusBadRequest &&
		refused.reason == geminiKeyInvalid {
		return KeyTestAuthFailed
	}
	switch refused.status {
	case http.StatusUnauthorized:
		return KeyTestAuthFailed
	case http.StatusForbidden:
		return KeyTestPermissionDenied
	case http.StatusTooManyRequests:
		return KeyTestRateLimited
	default:
		return KeyTestUnreachable
	}
}
