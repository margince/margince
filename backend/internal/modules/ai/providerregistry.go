// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// providerDescriptor is every fact this package knows about one provider word.
// Each per-provider table the package reads (the accepted names, sovereign
// eligibility, egress lane, key variable, served-identity source, local
// endpoint default, carriage, public naming, default model, vendor hosting) is
// a projection of providerRegistry, so a provider cannot be known to one
// question and missing from the next. Adding a chat provider is adding one row
// here plus its construction recipe in selectBrainOn; a decision provider needs
// the row alone, since every decision adapter speaks one wire.
//
// Held by: TestProviderFactsLiveOnlyInTheRegistry (backend/internal/modules/ai/providerregistry_census_test.go)
type providerDescriptor struct {
	name string
	// caps is what the adapter answers. A chat tier and the embeddings lane
	// bind only a capChat provider; the decisions lane binds only a
	// capDecision one, so a word cannot be accepted by a lane whose wire it
	// does not speak.
	caps capSet
	// local is same-host inference: sovereign-eligible, and on the operator
	// egress lane for that reason.
	local bool
	// egress is the one place a provider's reach is decided, read by both the
	// dialer and the write-time rule. The zero value is the strict lane.
	egress egressClass
	// keyEnv is the environment variable a BYOK key is read from; empty means
	// the adapter takes no key.
	keyEnv       string
	servedSource string
	// defaultBaseURL is the endpoint an omitted base_url resolves to, for the
	// local adapters whose endpoint the sovereign rule checks.
	defaultBaseURL string
	// vendorHosted marks an adapter whose omitted base_url is the vendor's own
	// public API, which is what the routing preview calls cloud processing.
	vendorHosted bool
	// public is whether the anonymous profile may name the adapter. The fake is
	// a development mechanism, not a provider identity.
	public bool
	// defaultModel is the model an omitted model resolves to, for the adapters
	// that have one.
	defaultModel string
	carriage     []string
	// wildcardReason is why this adapter's carriage keeps a wildcard; empty
	// means the adapter names its decoders.
	wildcardReason string
	// keyOwner is the provider whose credential this adapter sends, for an
	// adapter that reaches a vendor already holding one. Empty means its own
	// keyEnv (or none): one vendor account is one vault ref, however many
	// wires reach it.
	keyOwner string
	// decisionPath is the decision wire's path under the binding's base_url.
	decisionPath string
}

// capSet is the set of wires an adapter answers on.
type capSet uint8

const (
	capChat capSet = 1 << iota
	capDecision
)

func (c capSet) has(want capSet) bool { return c&want != 0 }

// The decision-wire provider words. openrouter_decision is OpenRouter's
// decisions endpoint, reached with the openai_compatible key; laya is a
// same-host encoder answering the same wire, keyless and sovereign-eligible.
const (
	providerOpenRouterDecision = "openrouter_decision"
	providerLaya               = "laya"
	defaultLayaBaseURL         = "http://127.0.0.1:8765"
)

// providerRegistry's row order is the order knownProviders reports, which
// SelectBrain's refusal message and the config schema's enum both show.
var providerRegistry = []providerDescriptor{
	{
		// The fake opens no socket, so its egress class is inert; it still
		// declares one because every provider answers every question.
		name: ProviderFake, caps: capChat, local: true, egress: egressPublicOnly,
		servedSource: servedIdentitySourceResponse, carriage: carriesImagesAndPDF,
		wildcardReason: "stands in for whichever binding named it, so it claims the wire's shape rather than a decoder",
	},
	{
		name: providerAnthropic, caps: capChat, egress: egressPublicOnly, keyEnv: "ANTHROPIC_API_KEY",
		servedSource: servedIdentitySourceResponse, vendorHosted: true, public: true,
		carriage: anthropicCarries,
	},
	{
		name: providerOllama, caps: capChat, local: true, egress: egressOperatorEndpoint,
		servedSource: servedIdentitySourceResponse, defaultBaseURL: defaultOllamaBaseURL,
		public: true, defaultModel: defaultOllamaModel, carriage: carriesImages,
		wildcardReason: "serves whichever vision model the operator pulled",
	},
	{
		// vllm and openai_compatible are ONE adapter, and it has no document
		// part: openAICompatMessages builds `image_url` parts and nothing else,
		// so no client either word selects carries a PDF in any configuration.
		// The wire's shape is not the ambition of the wire's vendor: it is what
		// this adapter can put on it.
		name: providerVLLM, caps: capChat, local: true, egress: egressOperatorEndpoint,
		servedSource: servedIdentitySourceEcho, defaultBaseURL: defaultVLLMBaseURL,
		public: true, defaultModel: defaultVLLMModel, carriage: carriesImages,
		wildcardReason: "serves whichever model the operator loaded",
	},
	{
		// The one BYOK provider on the operator lane, and the only row whose
		// egress does not follow from local. `openai_compatible` is the adapter
		// for "any vendor on the OpenAI wire", and a self-hosted gateway on the
		// operator's own network is a documented one — so a private address is
		// a binding this lane must serve, and the key travelling there travels
		// to the operator's own infrastructure.
		// TestEveryLocalProviderTakesTheOperatorLane names it as the sole
		// exception, so a future adapter cannot join it quietly. Its key
		// variable is namespaced because it has no vendor convention.
		name: providerOpenAICompatible, caps: capChat, egress: egressOperatorEndpoint,
		keyEnv: "OPENAI_COMPATIBLE_API_KEY", servedSource: servedIdentitySourceEcho,
		public: true, carriage: carriesImages,
		wildcardReason: "serves whichever vendor the operator pointed base_url at",
	},
	{
		name: providerOpenAI, caps: capChat, egress: egressPublicOnly, keyEnv: "OPENAI_API_KEY",
		servedSource: servedIdentitySourceResponse, vendorHosted: true, public: true,
		carriage: openAICarries,
	},
	{
		name: providerGemini, caps: capChat, egress: egressPublicOnly, keyEnv: "GEMINI_API_KEY",
		servedSource: servedIdentitySourceResponse, vendorHosted: true, public: true,
		carriage: geminiCarries,
	},
	{
		name: providerOpenRouterDecision, caps: capDecision, egress: egressPublicOnly,
		keyOwner: providerOpenAICompatible, servedSource: servedIdentitySourceResponse,
		decisionPath: "/alpha/decisions",
	},
	{
		// Keyless and same-host: the operator lane, sovereign-eligible, and its
		// omitted base_url is loopback, so the sovereign rule checks it.
		name: providerLaya, caps: capDecision, local: true, egress: egressOperatorEndpoint,
		servedSource: servedIdentitySourceEcho, defaultBaseURL: defaultLayaBaseURL,
		decisionPath: "/v1/systemone",
	},
}

// providerByName answers for a provider word that may not be one this build
// knows; the zero descriptor is the strict answer to every question.
func providerByName(name string) (providerDescriptor, bool) {
	for _, d := range providerRegistry {
		if d.name == name {
			return d, true
		}
	}
	return providerDescriptor{}, false
}

func providerIsVendorHosted(name string) bool {
	d, _ := providerByName(name)
	return d.vendorHosted
}

func providerDefaultModel(name string) string {
	d, _ := providerByName(name)
	return d.defaultModel
}

// projectProviders builds one per-provider table from the registry, keeping
// only the rows keep accepts.
func projectProviders[V any](value func(providerDescriptor) V, keep func(providerDescriptor) bool) map[string]V {
	out := make(map[string]V, len(providerRegistry))
	for _, d := range providerRegistry {
		if keep(d) {
			out[d.name] = value(d)
		}
	}
	return out
}

func everyProvider(providerDescriptor) bool { return true }

func speaksChat(d providerDescriptor) bool { return d.caps.has(capChat) }

func providerNames() []string { return providerNamesWhere(everyProvider) }

// providerNamesWhere is the registry's names, in its order, for the rows keep
// accepts.
func providerNamesWhere(keep func(providerDescriptor) bool) []string {
	out := make([]string, 0, len(providerRegistry))
	for _, d := range providerRegistry {
		if keep(d) {
			out = append(out, d.name)
		}
	}
	return out
}

// DecisionProviders lists the provider words the decisions lane accepts, in
// registry order. Exported for the config schema's enum and the routing
// form, which offer exactly these; a fresh slice, so no caller can reorder
// the source.
func DecisionProviders() []string {
	return providerNamesWhere(func(d providerDescriptor) bool { return d.caps.has(capDecision) })
}

// keyOwnerOf is the provider whose credential a binding of provider sends.
func keyOwnerOf(provider string) string {
	if d, _ := providerByName(provider); d.keyOwner != "" {
		return d.keyOwner
	}
	return provider
}
