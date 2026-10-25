// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package geocode turns a postal address into a point on the earth.
//
// It exists because "which customers are near Stuttgart" is a question the
// query surface could parse and not answer: within_radius was in the grammar
// and every plan carrying it came back unavailable, because no row had
// coordinates to compare.
//
// ONE PROVIDER TODAY, and the interface is what keeps that from hardening.
// Nominatim is OpenStreetMap's free service: no key, no contract, and a usage
// policy this package takes seriously rather than works around. A self-hosted
// instance is the same client with a different base URL.
package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// Point is a resolved location.
type Point struct {
	Lat, Lon float64
}

// Client resolves free text or a structured address to a point.
//
// An address that resolves to nothing is NOT an error: the geocoder did its
// job and the answer is "no such place", which is a fact about the address and
// must be recorded as one rather than retried forever. ok=false says that;
// err is reserved for a lookup that did not complete.
type Client interface {
	Resolve(ctx context.Context, query string) (Point, bool, error)
	// Countries answers the countries of the places a query names. An empty
	// answer is no place; an error is a lookup that did not complete.
	Countries(ctx context.Context, query string) ([]string, error)
}

// The public Nominatim service, and the rate it may be asked at.
const (
	// PublicBaseURL is OSM's own instance. POC-only: a deployment that geocodes
	// at any volume should point at its own.
	PublicBaseURL = "https://nominatim.openstreetmap.org"

	// RecurringInterval is the floor between requests, and it is 15 SECONDS
	// rather than the 1s the policy allows an interactive client.
	//
	// OSMF's usage policy caps the public service at roughly 1 request/second
	// in general, but a script that runs regularly or for longer than a day is
	// held to 4 requests per minute, single-threaded, with caching. Geocoding
	// on ingestion is exactly such a job — it runs whenever a company is read,
	// indefinitely — so 15s is the rate this package is entitled to, not a
	// conservative reading of a faster one.
	RecurringInterval = 15 * time.Second
)

// UserAgent names this software to the geocoding service.
const UserAgent = outbound.GeocodeHeader

// ErrNotConfigured says the operator turned geocoding off. It is a REFUSAL, not
// a failure: the caller records that rather than retrying.
var ErrNotConfigured = errors.New("geocode: geocoding is switched off")

// OffSetting is the value of MARGINCE_GEOCODE_BASE_URL that switches geocoding
// off; "public" and an unset value both mean the public service.
const OffSetting = "off"

// Resolve is the single reading of MARGINCE_GEOCODE_BASE_URL. The worker builds
// a client from baseURL, and the api queues a lookup only when on is true.
// Unset means on, with the public service, so a fresh install geocodes at the
// paced rate. Only an explicit "off" disables it.
func Resolve(setting string) (baseURL string, on bool) {
	switch trimmed := strings.TrimSpace(setting); trimmed {
	case "", "public":
		return PublicBaseURL, true
	case OffSetting:
		return "", false
	default:
		return trimmed, true
	}
}

// Nominatim is the OSM client.
type Nominatim struct {
	baseURL string
	http    *http.Client
	pacer   *Pacer
}

// NewNominatim builds the client. An empty baseURL means the public service.
//
// The pacer is created here and held by the client, so ONE client is one
// requester: the policy's single-thread requirement is a property of the
// object, and two clients would be two requesters however carefully each
// paced itself. The composition root builds exactly one.
func NewNominatim(baseURL string, httpClient *http.Client) *Nominatim {
	if baseURL == "" {
		baseURL = PublicBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Nominatim{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		http:    httpClient,
		pacer:   NewPacer(RecurringInterval),
	}
}

// nominatimResult is the one field set this package reads back. The service
// returns a great deal more; taking only the coordinates and the country code
// keeps the parse stable across its versions.
type nominatimResult struct {
	Lat     string `json:"lat"`
	Lon     string `json:"lon"`
	Address struct {
		CountryCode string `json:"country_code"`
	} `json:"address"`
}

// Resolve asks the provider where a place is.
//
// Three outcomes, and telling them apart is the caller's whole retry policy: a
// point, a definite "not a place" (ok=false, no error — recorded and never
// re-asked), and a lookup that did not complete (an error — retried, and after
// a 429 retried on the provider's own schedule via ProviderRefusedError).
func (n *Nominatim) Resolve(ctx context.Context, query string) (Point, bool, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Point{}, false, nil
	}
	results, err := n.search(ctx, url.Values{"q": {query}, "limit": {"1"}})
	if err != nil {
		return Point{}, false, err
	}
	if len(results) == 0 {
		// No match. A FACT about the address, not a failure of the lookup.
		return Point{}, false, nil
	}
	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return Point{}, false, fmt.Errorf("geocode: the provider's latitude %q is not a number: %w",
			results[0].Lat, err)
	}
	lon, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return Point{}, false, fmt.Errorf("geocode: the provider's longitude %q is not a number: %w",
			results[0].Lon, err)
	}
	if !plausible(lat, lon) {
		return Point{}, false, fmt.Errorf("geocode: the provider answered (%v, %v), which is not on the earth", lat, lon)
	}
	return Point{Lat: lat, Lon: lon}, true, nil
}

// search asks the provider one paced question and decodes its candidates.
func (n *Nominatim) search(ctx context.Context, params url.Values) ([]nominatimResult, error) {
	// The pace is taken BEFORE the request is built, and it is the whole
	// reason this client is safe to run on a schedule.
	if err := n.pacer.Wait(ctx); err != nil {
		return nil, err
	}

	params.Set("format", "jsonv2")
	endpoint := n.baseURL + "/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := n.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("geocode: asking the provider: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close: the decode below reads what it needs and may leave the body mid-stream, so a close error says nothing about whether the address resolved
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// A 429 or a 403 means this installation is being rate-limited or
		// blocked — a condition to surface rather than absorb, and one that
		// carries its own instruction about when to come back.
		return nil, &ProviderRefusedError{
			Status:     resp.StatusCode,
			RetryAfter: retryafter.Of(resp),
		}
	}

	var results []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("geocode: reading the provider's answer: %w", err)
	}
	return results, nil
}

// countryCandidates is how many places a country question reads back. A postal
// code and city can name places in two countries, and one answer would pick
// between them by the provider's ranking alone.
const countryCandidates = 5

// Countries answers the distinct countries, as upper-case ISO 3166-1 alpha-2
// codes in the provider's order, of the places a query can name. An empty
// answer is "no such place", not an error.
func (n *Nominatim) Countries(ctx context.Context, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	results, err := n.search(ctx, url.Values{
		"q": {query}, "limit": {strconv.Itoa(countryCandidates)}, "addressdetails": {"1"},
	})
	if err != nil {
		return nil, err
	}
	codes := []string{}
	for _, result := range results {
		code := strings.ToUpper(result.Address.CountryCode)
		if code != "" && !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	return codes, nil
}

// plausible rejects a coordinate outside the earth's own range.
//
// Not paranoia about the provider: it is the cheapest place to catch a
// transposed lat/lon pair, which is the mistake that produces confidently
// wrong distances rather than an error anyone notices.
func plausible(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// ProviderRefusedError is the provider declining, with the wait it asked for.
//
// The wait is the part worth carrying. A 429 means this installation is asking
// too often, and retrying on the job runner's own schedule rather than the
// provider's is how a rate limit becomes a block.
type ProviderRefusedError struct {
	Status int
	// RetryAfter is what the provider asked for, or zero when it said nothing.
	RetryAfter time.Duration
}

func (e *ProviderRefusedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("geocode: the provider answered %d and asked for %s before the next request",
			e.Status, e.RetryAfter)
	}
	return fmt.Sprintf("geocode: the provider answered %d", e.Status)
}
