// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The OpenRouter read behind `/ai/available-models/openrouter`: a public
// vendor's own list of served models, optionally ranked by a published
// benchmark and priced for a screen that PROPOSES a rate rather than records
// one. It sits in first run, so it must never block onboarding on another
// company's uptime: any failure degrades to Unavailable, never an error, and
// a successful read is cached in-process for 15 minutes so a busy screen does
// not re-ask the vendor on every render.
//
// Every other vendor answers the same route through SelectBrain and a stored
// binding; OpenRouter alone is asked unauthenticated and unbound, because its
// public list is also the only one carrying a benchmark score and a price.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
)

const (
	// catalogueRankedBy names the measure a ranked list's order came from,
	// verbatim on the wire: a screen prints it beside the list so "top" carries
	// a reason.
	catalogueRankedBy = "Artificial Analysis intelligence index"
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
	// contract's own path-param value.
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

// CatalogueFetcher is the vendor read this module depends on; production wires
// an unauthenticated GET, tests a fake.
type CatalogueFetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// ModelCatalogue serves OpenRouter's published list from a 15-minute cache. The
// full payload is cached; each request derives its ranked-or-full view from it.
type ModelCatalogue struct {
	cache *catalogueCache[[]openRouterModel]
}

// NewModelCatalogue wires the production catalogue over OpenRouter's public read.
func NewModelCatalogue(clock Clock) *ModelCatalogue {
	return NewModelCatalogueOver(vendorCatalogueFetcher{
		vendor: openRouterProvider, url: openRouterModelsURL, maxBytes: catalogueMaxResponseBytes,
	}, clock)
}

// NewModelCatalogueOver is NewModelCatalogue over another fetcher: a test's fake.
func NewModelCatalogueOver(fetcher CatalogueFetcher, clock Clock) *ModelCatalogue {
	return &ModelCatalogue{cache: &catalogueCache[[]openRouterModel]{
		fetcher: fetcher, clock: clock, parse: parseNonEmptyOpenRouter,
	}}
}

func parseNonEmptyOpenRouter(body []byte) ([]openRouterModel, error) {
	models, err := parseOpenRouterCatalogue(body)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("ai model catalogue: openrouter answered with no models")
	}
	return models, nil
}

// WithCatalogue wires the OpenRouter read into the store. Absent it, the
// store answers openrouter as not_published rather than panicking, the same
// posture as an adapter that does not exist.
func (s *RoutingStore) WithCatalogue(catalogue *ModelCatalogue) *RoutingStore {
	next := *s
	next.catalogue = catalogue
	return &next
}

// List serves OpenRouter's catalogue from cache when fresh, else re-reads the
// vendor. It never returns an error: any failure resolves to the honest
// Unavailable response, logged as a warning for an operator to notice without
// ever blocking the caller on it.
//
// top > 0 asks for the best `top` models by the vendor's published benchmark,
// which is the only measure this adapter can honour (`ranked_by` is set on
// the result). top == 0 answers the vendor's full list in the vendor's own
// order: priced where the vendor prices it, and unranked.
func (c *ModelCatalogue) List(ctx context.Context, top int) AvailableModels {
	models, err := c.cache.fresh(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ai model catalogue unusable; serving unavailable",
			"provider", openRouterProvider, "error", err)
		return AvailableModels{Provider: openRouterProvider, Unavailable: AvailabilityUnreachable}
	}
	if top > 0 {
		entries, rankedBy := rankedAvailableModels(models, top)
		return AvailableModels{Provider: openRouterProvider, Models: entries, RankedBy: rankedBy}
	}
	return AvailableModels{Provider: openRouterProvider, Models: fullAvailableModels(models)}
}

// vendorCatalogueFetcher is the production CatalogueFetcher: an unauthenticated
// GET of one vendor's public list, refused once it runs past maxBytes.
type vendorCatalogueFetcher struct {
	vendor, url string
	maxBytes    int64
}

var catalogueHTTPClient = &http.Client{Timeout: catalogueFetchTimeout}

func (f vendorCatalogueFetcher) Fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("ai model catalogue: building the %s request: %w", f.vendor, err)
	}
	// How this product names itself to a server it calls is `outbound`'s to say.
	req.Header.Set("User-Agent", outbound.ModelCatalogueHeader)
	resp, err := catalogueHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai model catalogue: %s unreachable: %w", f.vendor, err)
	}
	//craft:ignore swallowed-errors best-effort close: the capped read below may leave the body mid-stream, so a close error carries no signal for the fetch result
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai model catalogue: %s answered %d", f.vendor, resp.StatusCode)
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("ai model catalogue: reading the %s response: %w", f.vendor, err)
	}
	if int64(len(out)) > f.maxBytes {
		return nil, fmt.Errorf("ai model catalogue: %s answered more than %d bytes", f.vendor, f.maxBytes)
	}
	return out, nil
}
