// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ProviderConfig is one tier→provider binding from ai-routing.yaml — the only
// place vendor names appear. It carries NO secret: a cloud provider's BYOK key
// is read at boot from the provider's conventional environment variable (see
// cloudKeyEnv), so secrets live in the environment, never a config file. The
// parser rejects unknown keys, so a stray `api_key:` here is a loud error that
// points the operator at the env var (ADR-0020: BYOK, we provide no inference).
type ProviderConfig struct {
	Provider string `yaml:"provider" json:"provider"` // one of knownProviders
	Model    string `yaml:"model" json:"model"`       // provider-native model id, resolved from the logical tier
	BaseURL  string `yaml:"base_url" json:"base_url"` // endpoint override; empty means the provider default
	// Input is what the bound model can be GIVEN, in the acceptedModalities
	// vocabulary (inputmodality.go). It does two jobs: on openai_compatible and
	// vllm it IS the carriage, because only there is the answer a property of the
	// bound model rather than of the adapter; everywhere else it NARROWS the
	// carriage that adapter's wire already has, and can never widen it.
	//
	// Nil — the common case — means the provider's own answer: text-only on the
	// two OpenAI-wire providers, whatever the wire carries on the rest.
	Input []string `yaml:"input" json:"input,omitempty"`
}

// Provider defaults. The Anthropic URL is the vendor's public API; a
// hosting-partner or proxy deployment overrides BaseURL in config.
const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultOllamaBaseURL    = "http://localhost:11434"
	defaultVLLMBaseURL      = "http://localhost:8000"
	// defaultOpenAIBaseURL is the host root WITHOUT a version segment: the
	// OpenAI-wire transport appends "/v1/responses" / "/v1/embeddings", so a base
	// that already carried "/v1" would double it (…/v1/v1/responses → 404). Same
	// version-less convention as Anthropic and vLLM.
	defaultOpenAIBaseURL = "https://api.openai.com"
	// defaultGeminiBaseURL keeps the /v1beta version segment: Gemini paths are
	// version-relative (":generateContent" under "/models/…"), the mirror of the
	// OpenAI-wire convention above.
	defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// Local default models are Gemma-class per ADR-0012/A23: the unbound
// local path must land on a non-Chinese model (Gemma default, Mistral
// as the EU alternative an operator picks explicitly in config).
const (
	defaultOllamaModel = "gemma3"
	defaultVLLMModel   = "google/gemma-3-12b-it"
)

// jsonSchemaFormatType is the structured-output format discriminator the
// adapters send for a schema-constrained completion (the OpenAI-wire
// response_format, Anthropic's output_config.format, and the OpenAI Responses
// text.format all share this "json_schema" value).
const jsonSchemaFormatType = "json_schema"

// Provider names — the vocabulary shared by the SelectBrain switch,
// knownProviders, and the sovereign-eligible localProviders set. One spelling
// each so a typo can't silently split "the switch accepts it" from "the config
// enum offers it". ProviderFake is exported because the composition layer's
// dev tooling names the offline fake in the routing configs it assembles.
const (
	ProviderFake             = "fake"
	providerAnthropic        = "anthropic"
	providerOllama           = "ollama"
	providerVLLM             = "vllm"
	providerOpenAICompatible = "openai_compatible"
	providerOpenAI           = "openai"
	providerGemini           = "gemini"
)

// knownProviders is the single source of truth for the provider names
// SelectBrain accepts — read by the default error below and by the config
// JSON-schema drift test. Add a provider here when you add its case.
var knownProviders = []string{ProviderFake, providerAnthropic, providerOllama, providerVLLM, providerOpenAICompatible, providerOpenAI, providerGemini}

// KnownProviders lists the adapter names knownProviders holds, which is the
// same slice SelectBrain's switch and the config enum read.
//
// Exported for the gate that holds onboarding's provider list to this one: the
// screen offers a first-time admin a vendor, and a name this switch does not
// accept is a write refused with a message about an adapter they never chose.
// Returned as a copy — a caller that sorted the package's own slice in place
// would reorder the config enum's source.
func KnownProviders() []string {
	out := make([]string, len(knownProviders))
	copy(out, knownProviders)
	return out
}

// SelectBrain turns one binding into a Client (interfaces.md §4):
// "offline fake ↔ API key ↔ local, one line" — swapping providers is a
// config change, never a code change.
//
//nolint:ireturn // one call returns whichever of seven adapters the binding names; the port interface IS the return type
func SelectBrain(cfg ProviderConfig, keys config.Lookup) (model.Client, error) {
	switch cfg.Provider {
	case ProviderFake:
		// The stub narrows like any other adapter: `input:` on a fake binding
		// that silently did nothing is the failure this field exists to remove,
		// and an offline test of the narrowing needs a client that honours it.
		return NewFakeClient().carrying(narrowedCarriage(carriesImagesAndPDF, cfg.Input)), nil
	case providerAnthropic:
		key := cloudKey(providerAnthropic, keys)
		if key == "" {
			return nil, byokKeyRequired(providerAnthropic)
		}
		return &anthropicClient{
			http:            newOutboundClient(),
			baseURL:         defaulted(cfg.BaseURL, defaultAnthropicBaseURL),
			apiKey:          key,
			defaultModel:    cfg.Model,
			attachmentMIMEs: narrowedCarriage(anthropicCarries, cfg.Input),
		}, nil
	case providerOllama:
		return &ollamaClient{
			http:            newOutboundClient(),
			baseURL:         defaulted(cfg.BaseURL, defaultOllamaBaseURL),
			defaultModel:    defaulted(cfg.Model, defaultOllamaModel),
			attachmentMIMEs: narrowedCarriage(carriesImages, cfg.Input),
		}, nil
	case providerVLLM:
		return &openAICompatClient{
			http:            newOutboundClient(),
			baseURL:         defaulted(cfg.BaseURL, defaultVLLMBaseURL),
			apiKey:          "", // local vLLM: no auth
			localOnly:       true,
			defaultModel:    defaulted(cfg.Model, defaultVLLMModel),
			attachmentMIMEs: carriageFor(cfg.Input),
		}, nil
	case providerOpenAICompatible:
		key := cloudKey(providerOpenAICompatible, keys)
		if key == "" {
			return nil, byokKeyRequired(providerOpenAICompatible)
		}
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("%w: openai_compatible (the vendor host root — no version segment, the adapter adds /v1; e.g. https://api.mistral.ai)", errNoBaseURL)
		}
		return &openAICompatClient{
			http:            newOutboundClient(),
			baseURL:         cfg.BaseURL,
			apiKey:          key,
			localOnly:       false,
			defaultModel:    cfg.Model,
			attachmentMIMEs: carriageFor(cfg.Input),
		}, nil
	case providerOpenAI:
		key := cloudKey(providerOpenAI, keys)
		if key == "" {
			return nil, byokKeyRequired(providerOpenAI)
		}
		return &openaiClient{
			http:            newOutboundClient(),
			baseURL:         defaulted(cfg.BaseURL, defaultOpenAIBaseURL),
			apiKey:          key,
			defaultModel:    cfg.Model,
			attachmentMIMEs: narrowedCarriage(openAICarries, cfg.Input),
		}, nil
	case providerGemini:
		key := cloudKey(providerGemini, keys)
		if key == "" {
			return nil, byokKeyRequired(providerGemini)
		}
		return &geminiClient{
			http:            newOutboundClient(),
			baseURL:         defaulted(cfg.BaseURL, defaultGeminiBaseURL),
			apiKey:          key,
			defaultModel:    cfg.Model,
			attachmentMIMEs: narrowedCarriage(geminiCarries, cfg.Input),
		}, nil
	case "":
		return nil, fmt.Errorf("ai: binding has no provider")
	default:
		return nil, fmt.Errorf("ai: unknown provider %q (have: %s)", cfg.Provider, strings.Join(knownProviders, ", "))
	}
}

// defaulted returns val, or fallback when val is empty — the base-url / model
// defaulting every provider case shares.
func defaulted(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// cloudKeyEnv maps a cloud provider to the environment variable its BYOK key is
// read from. Secrets live in the environment; the routing file names only the
// provider (12-factor). The names match the vendor SDK conventions so an
// operator who already exports OPENAI_API_KEY / GEMINI_API_KEY needs no extra
// wiring. openai_compatible has no vendor convention, so it gets a namespaced one.
var cloudKeyEnv = map[string]string{
	providerAnthropic:        "ANTHROPIC_API_KEY",
	providerOpenAI:           "OPENAI_API_KEY",
	providerGemini:           "GEMINI_API_KEY",
	providerOpenAICompatible: "OPENAI_COMPATIBLE_API_KEY",
}

// cloudKey returns the BYOK key for a cloud provider from its conventional
// environment variable, or "" when unset (the caller fails closed).
func cloudKey(provider string, keys config.Lookup) string {
	if keys == nil {
		return ""
	}
	if env := cloudKeyEnv[provider]; env != "" {
		return keys(env)
	}
	return ""
}

// byokKeyRequired is the fail-closed error for a cloud provider bound without a
// key in the environment: Margince provides no inference, so the customer's key
// is mandatory (ADR-0020). It names the env var to set so the fix is obvious.
func byokKeyRequired(provider string) error {
	if env := cloudKeyEnv[provider]; env != "" {
		return fmt.Errorf("%w: provider %s — set %s in the environment (BYOK — we provide no inference)", errNoProviderKey, provider, env)
	}
	return fmt.Errorf("%w: provider %s (BYOK — we provide no inference)", errNoProviderKey, provider)
}

// The two refusals a CALLER can tell a reader how to fix: a vendor with no
// credential is a key to paste, and an OpenAI-wire binding with no host is an
// address to fill in. Sentinels rather than message matching, because the
// availability read reports them as states of the installation rather than as
// faults, and a screen that decided which by reading prose would break the
// first time the prose improved.
//
// Package-local, not `apperrors`: that registry is the HTTP error contract, and
// neither of these reaches a client as an error — they arrive as a named state
// on a 200.
var (
	errNoProviderKey = errors.New("ai: provider needs an api key")
	errNoBaseURL     = errors.New("ai: provider needs a base_url")
)

// ConfigItems declares the BYOK keys. They carry no MARGINCE_ prefix — the
// names match each vendor's own convention so an operator already exporting one
// needs no extra wiring — which is exactly why they have to be declared: the
// documentation gate that sweeps the tree exempts non-namespaced names, so this
// registry is the only artefact that can describe them at all.
//
// None is Required. A deployment binds whichever providers it uses, and a cloud
// binding without its key fails loudly at construction naming the variable
// (byokKeyRequired) rather than being pre-empted here, because which keys an
// installation needs is a property of its routing file, not of the product.
func ConfigItems() []config.Item {
	both := []string{config.RoleAPI, config.RoleWorker}
	items := make([]config.Item, 0, len(cloudKeyEnv))
	for _, provider := range knownProviders {
		env, cloud := cloudKeyEnv[provider]
		if !cloud {
			continue
		}
		items = append(items, config.Item{
			Name: env, Kind: config.KindString, Secret: true, Roles: both,
			Doc: "BYOK API key for the " + provider + " provider; required only when the routing file binds it (ADR-0020 — we provide no inference)",
		})
	}
	return items
}

// ParseBinding reads a "provider:model" spec and the endpoint that goes with it.
//
// It lives beside ProviderConfig because that is the only domain type it names,
// and because its callers cannot share it anywhere lower: aicert sits under
// compose, so a compose test cannot import it without a cycle, and the alternative
// is two spellings of the first-colon rule below — the one thing here that must
// not be copied.
//
// Exported with no production caller today, deliberately. Both callers are
// by-hand lanes (`e2e_llm`, `voicelive`) that hand it an operator-supplied spec;
// the rule is about how a binding is WRITTEN, so it belongs with the type
// whichever lane reads one next.
//
// The split cuts at the FIRST colon and leaves the rest of the slug whole:
// OpenRouter marks a served variant with its own colon suffix (":free",
// ":batch", ":thinking"), and cutting at the last one would silently certify a
// different variant from the one asked for.
//
// baseURL is carried rather than inferred. openai_compatible fails closed
// without one — the endpoint belongs to the vendor, not the model — so a broker
// run supplies it and a native vendor leaves it empty for the provider default.
func ParseBinding(spec, baseURL string) (ProviderConfig, error) {
	provider, modelName, found := strings.Cut(spec, ":")
	if !found || provider == "" || modelName == "" {
		return ProviderConfig{}, fmt.Errorf("ai: a model binding wants provider:model, got %q", spec)
	}
	return ProviderConfig{Provider: provider, Model: modelName, BaseURL: baseURL}, nil
}
