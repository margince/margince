// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The AI model catalogue read (GET /ai-model-catalogue): a public vendor's
// own list of served models, ranked by a published benchmark and priced for
// a screen that PROPOSES a rate rather than records one. It sits in first
// run, so it must never block onboarding on another company's uptime: any
// failure degrades to Unavailable at HTTP 200, never an error status, and a
// successful read is cached in-process for 15 minutes so a busy screen does
// not re-ask the vendor on every render.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/outbound"
)

const (
	// catalogueRankedBy names the measure the order came from, verbatim on
	// the wire: a screen prints it beside the list so "top" carries a reason.
	catalogueRankedBy = "Artificial Analysis intelligence index"
	// catalogueTop bounds how many priced, ranked models the screen is shown.
	catalogueTop = 10
	// catalogueCacheTTL bounds how long a successful read is trusted before
	// the next request re-asks the vendor.
	catalogueCacheTTL = 15 * time.Minute
	// catalogueFetchTimeout matches webread's fetchTimeout: the outbound
	// budget this product gives any one read of the open web.
	catalogueFetchTimeout = 10 * time.Second
	// catalogueMaxResponseBytes caps the vendor body a fetch will read. The
	// full OpenRouter list runs to a few hundred models with benchmark
	// blocks attached, comfortably under this.
	catalogueMaxResponseBytes = 8 << 20
	// openRouterProvider is the one vendor this read serves, matching the
	// contract's own enum value.
	openRouterProvider = "openrouter"
	// openRouterModelsURL is OpenRouter's public, unauthenticated model list.
	openRouterModelsURL = "https://openrouter.ai/api/v1/models"
	// usdPerTokenToMicroUSDPerMTok scales a USD-per-single-token price into
	// microUSD-per-million-tokens: 1e6 tokens/MTok times 1e6 microUSD/USD.
	usdPerTokenToMicroUSDPerMTok = 1_000_000_000_000
)

// Clock is this module's own time source for the catalogue cache, kept
// tiny and local rather than importing one: it lets the TTL be proven
// without sleeping a real 15 minutes in a test. Compose injects the real
// wall clock.
type Clock interface{ Now() time.Time }

// catalogueFetcher is the vendor read this module depends on. Production
// wires an unauthenticated OpenRouter GET; tests inject a fake and never
// touch the network.
type catalogueFetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// ModelCatalogue serves the ranked, priced vendor catalogue behind a
// mutex-guarded, TTL cache keyed by provider. Only a successful read is
// ever cached; a failure is retried on the very next request rather than
// pinned for 15 minutes.
type ModelCatalogue struct {
	fetcher catalogueFetcher
	clock   Clock

	mu    sync.Mutex
	cache map[string]crmcontracts.AiModelCatalogueResponse
}

// NewModelCatalogue wires the production catalogue over OpenRouter's public
// read. clock is compose's real wall clock in production and a fake in tests.
func NewModelCatalogue(clock Clock) *ModelCatalogue {
	return &ModelCatalogue{
		fetcher: openRouterFetcher{},
		clock:   clock,
		cache:   map[string]crmcontracts.AiModelCatalogueResponse{},
	}
}

// WithCatalogue wires the model catalogue read. Absent it the route answers
// Unavailable rather than panicking, the same posture as WithProviderKeys.
func (h Handlers) WithCatalogue(catalogue *ModelCatalogue) Handlers {
	h.catalogue = catalogue
	return h
}

// ListAiModelCatalogue implements (GET /ai-model-catalogue).
func (h Handlers) ListAiModelCatalogue(w http.ResponseWriter, r *http.Request, params crmcontracts.ListAiModelCatalogueParams) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if !params.Provider.Valid() {
		httperr.Write(w, r, httperr.Validation("provider", "catalogue_provider_unknown",
			"provider must be a vendor whose catalogue this installation can read"))
		return
	}
	if h.catalogue == nil {
		httperr.WriteJSON(w, http.StatusOK, unavailableCatalogue())
		return
	}
	httperr.WriteJSON(w, http.StatusOK, h.catalogue.List(r.Context(), string(params.Provider)))
}

// List serves the given provider's catalogue from cache when fresh, else
// re-reads the vendor. It never returns an error: any failure resolves to
// the honest Unavailable response, logged as a warning for an operator to
// notice without ever blocking the caller on it.
func (c *ModelCatalogue) List(ctx context.Context, provider string) crmcontracts.AiModelCatalogueResponse {
	// The fetcher reads OpenRouter and only OpenRouter, so a provider it does
	// not serve must stop HERE. The contract's enum admits one vendor today,
	// which is exactly why this is worth writing: widening that enum without
	// giving the new vendor a fetcher would otherwise hand its name back with
	// OpenRouter's models under it, and a wrong catalogue reads as a right one.
	if provider != openRouterProvider {
		slog.WarnContext(ctx, "ai model catalogue asked for a vendor with no reader; serving unavailable",
			"provider", provider)
		return unavailableCatalogue()
	}
	c.mu.Lock()
	cached, ok := c.cache[provider]
	c.mu.Unlock()
	if ok && cached.FetchedAt != nil && c.clock.Now().Sub(*cached.FetchedAt) < catalogueCacheTTL {
		return cached
	}

	body, err := c.fetcher.Fetch(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ai model catalogue fetch failed; serving unavailable",
			"provider", provider, "error", err)
		return unavailableCatalogue()
	}
	resp, err := parseOpenRouterCatalogue(body, c.clock.Now())
	if err != nil || len(resp.Data) == 0 {
		slog.WarnContext(ctx, "ai model catalogue response unusable; serving unavailable",
			"provider", provider, "error", err)
		return unavailableCatalogue()
	}

	c.mu.Lock()
	c.cache[provider] = resp
	c.mu.Unlock()
	return resp
}

func unavailableCatalogue() crmcontracts.AiModelCatalogueResponse {
	return crmcontracts.AiModelCatalogueResponse{
		Data:        []crmcontracts.AiModelCatalogueEntry{},
		RankedBy:    catalogueRankedBy,
		Unavailable: true,
	}
}

// openRouterFetcher is the production catalogueFetcher: a plain,
// unauthenticated GET of OpenRouter's public model list.
type openRouterFetcher struct{}

var catalogueHTTPClient = &http.Client{Timeout: catalogueFetchTimeout}

func (openRouterFetcher) Fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ai model catalogue: building the openrouter request: %w", err)
	}
	// How this product names itself to a server it calls is `outbound`'s to
	// say, for every call this product makes. A token minted here would be a
	// second answer to that, carrying its own copy of the version.
	req.Header.Set("User-Agent", outbound.ModelCatalogueHeader)
	resp, err := catalogueHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai model catalogue: openrouter unreachable: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close: the capped read below may leave the body mid-stream, so a close error carries no signal for the fetch result
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai model catalogue: openrouter answered %d", resp.StatusCode)
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, catalogueMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("ai model catalogue: reading the openrouter response: %w", err)
	}
	return out, nil
}
