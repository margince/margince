// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	azureTestEndpoint = "https://contoso.openai.azure.com"
	azureTestRegion   = "swedencentral"
	azureTestKey      = "azure-test-key"
)

// azureRecorder is an Azure resource stand-in: it records each request and
// answers with the canned status and body.
type azureRecorder struct {
	mu      sync.Mutex
	seen    []*http.Request
	status  int
	body    string
	headers map[string]string
}

func (a *azureRecorder) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.seen = append(a.seen, r)
		a.mu.Unlock()
		for k, v := range a.headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(a.status)
		if _, err := io.WriteString(w, a.body); err != nil {
			t.Errorf("writing the fixture reply: %v", err)
		}
	}
}

func azureClientAgainst(t *testing.T, rec *azureRecorder, cfg ProviderConfig) model.Client {
	t.Helper()
	srv := httptest.NewServer(rec.handler(t))
	t.Cleanup(srv.Close)
	cfg.Provider, cfg.BaseURL = providerAzureOpenAI, srv.URL
	client, err := selectLocalBrain(cfg, allCloudKeys(t))
	if err != nil {
		t.Fatalf("building the azure_openai adapter: %v", err)
	}
	return client
}

const azureChatReply = `{"model":"gpt-4o","choices":[{"message":{"content":"hej"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`

func TestAzureChatNamesTheDeploymentInThePathAndSendsTheApiKeyHeader(t *testing.T) {
	t.Parallel()
	rec := &azureRecorder{status: http.StatusOK, body: azureChatReply}
	client := azureClientAgainst(t, rec, ProviderConfig{Model: "my-gpt", Location: azureTestRegion})
	if _, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := rec.seen[0]
	if got.URL.Path != "/openai/deployments/my-gpt/chat/completions" {
		t.Errorf("path = %q, want the deployment in the path", got.URL.Path)
	}
	if got.URL.Query().Get("api-version") != azureAPIVersion {
		t.Errorf("api-version = %q, want %q", got.URL.Query().Get("api-version"), azureAPIVersion)
	}
	if got.Header.Get("api-key") == "" || got.Header.Get("Authorization") != "" {
		t.Errorf("credential headers: api-key=%q Authorization=%q, want api-key only", got.Header.Get("api-key"), got.Header.Get("Authorization"))
	}
}

func TestAzureEmbeddingsUseTheDeploymentPathToo(t *testing.T) {
	t.Parallel()
	rec := &azureRecorder{status: http.StatusOK, body: `{"data":[{"embedding":[0.1,0.2]}]}`}
	client := azureClientAgainst(t, rec, ProviderConfig{Model: "embed-dep", Location: azureTestRegion})
	if _, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: []string{"a"}}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got := rec.seen[0].URL.Path; got != "/openai/deployments/embed-dep/embeddings" {
		t.Errorf("path = %q, want the embeddings deployment path", got)
	}
}

func TestAzureListsModelsUnderOpenaiModels(t *testing.T) {
	t.Parallel()
	rec := &azureRecorder{status: http.StatusOK, body: `{"data":[{"id":"gpt-4o"}]}`}
	client := azureClientAgainst(t, rec, ProviderConfig{Model: "d", Location: azureTestRegion})
	lister, ok := client.(model.Lister)
	if !ok {
		t.Fatalf("the azure_openai client is a %T, which cannot list models", client)
	}
	models, err := lister.ListModels(context.Background())
	if err != nil || len(models) != 1 {
		t.Fatalf("ListModels = %v, %v", models, err)
	}
	got := rec.seen[0]
	if got.URL.Path != "/openai/models" || got.URL.Query().Get("api-version") != azureAPIVersion || got.Header.Get("api-key") == "" {
		t.Errorf("list request = %s?%s api-key=%q", got.URL.Path, got.URL.RawQuery, got.Header.Get("api-key"))
	}
}

func TestAzureTooManyRequestsIsTheSharedThrottleRefusal(t *testing.T) {
	t.Parallel()
	rec := &azureRecorder{
		status:  http.StatusTooManyRequests,
		headers: map[string]string{"Retry-After": "7"},
		body:    `{"error":{"code":"429","message":"Requests to the ChatCompletions_Create Operation have exceeded call rate limit of your current OpenAI S0 pricing tier. Please retry after 7 seconds."}}`,
	}
	client := azureClientAgainst(t, rec, ProviderConfig{Model: "my-gpt", Location: azureTestRegion})
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(err, ErrProviderThrottled) {
		t.Fatalf("a 429 with Retry-After is a throttle, got %v", err)
	}
	if errors.Is(err, ErrProviderQuota) {
		t.Errorf("a rate limit is not an exhausted budget: %v", err)
	}
}

func TestAzureRefusesADeploymentNameThatIsNotAPathSegment(t *testing.T) {
	t.Parallel()
	rec := &azureRecorder{status: http.StatusOK, body: azureChatReply}
	client := azureClientAgainst(t, rec, ProviderConfig{Model: "my-gpt", Location: azureTestRegion})
	_, err := client.Complete(context.Background(), model.Request{Model: "../other", Messages: []model.Message{{Role: "user", Content: "hi"}}})
	if err == nil || len(rec.seen) != 0 {
		t.Fatalf("a deployment with a separator must be refused before any request, got %v after %d requests", err, len(rec.seen))
	}
}

// The main risk is data leaving the agreed country. That happens through a
// region core does not know, or through one outside a stricter profile.
func TestAzureRegionDecidesWhereDataMayGo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		profile Profile
		region  string
		wantErr string
	}{
		{"an unknown region is blocked on the loosest profile", ProfileCloudFrontier, "marscentral", "not one this build knows"},
		{"an unknown region is blocked under eu_hosted", ProfileEUHosted, "newregion1", "not one this build knows"},
		{"a region spelled like Vertex's is not an Azure region", ProfileCloudFrontier, "europe-west4", "not one this build knows"},
		{"no region at all is refused", ProfileCloudFrontier, "", "needs a `location`"},
		{"a US region is refused under eu_hosted", ProfileEUHosted, "eastus", "inside the EU"},
		{"a UK region is outside the EU", ProfileEUHosted, "uksouth", "inside the EU"},
		{"a Swiss region is outside the EU", ProfileEUHosted, "switzerlandnorth", "inside the EU"},
		{"an EU region is allowed under eu_hosted", ProfileEUHosted, "swedencentral", ""},
		{"another EU region is allowed under eu_hosted", ProfileEUHosted, "germanywestcentral", ""},
		{"a known US region is allowed under cloud_frontier", ProfileCloudFrontier, "eastus", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			binding := ProviderConfig{Provider: providerAzureOpenAI, Model: "d", BaseURL: azureTestEndpoint, Location: tc.region}
			err := ValidateTierBinding(tc.profile, TierPremium, binding)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("want it allowed, got %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("want a refusal containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestAzureBuildsNoClientForAnUnknownRegion(t *testing.T) {
	t.Parallel()
	_, err := SelectBrain(ProviderConfig{Provider: providerAzureOpenAI, Model: "d", BaseURL: azureTestEndpoint, Location: "marscentral"},
		config.Static(map[string]string{cloudKeyEnv[providerAzureOpenAI]: azureTestKey}))
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("an unknown region must fail closed at construction, got %v", err)
	}
}

func TestEveryMappedAzureRegionHasACountryAndTheEUIsRight(t *testing.T) {
	t.Parallel()
	for region, country := range azureRegionCountries {
		if region != strings.ToLower(region) || len(country) != 2 {
			t.Errorf("region %q → %q is not a region name and an ISO country code", region, country)
		}
	}
	for region, wantEU := range map[string]bool{"swedencentral": true, "westeurope": true, "northeurope": true, "norwayeast": false, "uksouth": false, "eastus": false, "japaneast": false} {
		if azureRegionInEU(region) != wantEU {
			t.Errorf("azureRegionInEU(%q) = %v, want %v", region, !wantEU, wantEU)
		}
	}
}

// The key goes to the host named, so only a resource host Azure owns may be named.
func TestAzureEndpointMustBeAnAzureResourceHost(t *testing.T) {
	t.Parallel()
	for endpoint, ok := range map[string]bool{
		"https://contoso.openai.azure.com":           true,
		"https://contoso.openai.azure.com/":          true,
		"http://contoso.openai.azure.com":            false,
		"https://contoso.openai.azure.com/openai/v1": false,
		"https://contoso.openai.azure.com.evil.test": false,
		"https://openai.azure.com":                   false,
		"https://a.b.openai.azure.com":               false,
		"https://user:pw@contoso.openai.azure.com":   false,
		"https://contoso.openai.azure.com:8443":      false,
		"https://api.openai.com":                     false,
		"https://contoso.openai.azure.com?x=1":       false,
	} {
		err := azureEndpointError("test", endpoint)
		if (err == nil) != ok {
			t.Errorf("azureEndpointError(%q) = %v, want ok=%v", endpoint, err, ok)
		}
	}
}

func TestAzureNeedsItsResourceEndpointBeforeItCanBeServed(t *testing.T) {
	t.Parallel()
	binding := ProviderConfig{Provider: providerAzureOpenAI, Model: "d", Location: azureTestRegion}
	err := ValidateTierBinding(ProfileCloudFrontier, TierPremium, binding)
	if _, missing := errors.AsType[missingHostError](err); !missing || !strings.Contains(err.Error(), "azure_openai") {
		t.Fatalf("a binding with no endpoint is a no_host refusal worded for Azure, got %v", err)
	}
}

func TestAzureProviderEntryAndRoutingDocumentCarryRegionAndEndpoint(t *testing.T) {
	t.Parallel()
	doc := func(profile, region string) []byte {
		return []byte("profile: " + profile + "\nproviders:\n  azure_openai: {base_url: " + azureTestEndpoint + ", location: " + region + "}\n" +
			"tiers:\n  premium: {provider: azure_openai, model: my-gpt}\nembeddings: {provider: azure_openai, model: embed-dep}\n")
	}
	if _, err := ParseRouting(doc("eu_hosted", azureTestRegion)); err != nil {
		t.Fatalf("an EU region under eu_hosted: %v", err)
	}
	if _, err := ParseRouting(doc("eu_hosted", "eastus")); err == nil || !strings.Contains(err.Error(), "inside the EU") {
		t.Fatalf("a US region under eu_hosted must be refused, got %v", err)
	}
	if _, err := ParseRouting(doc("cloud_frontier", "marscentral")); err == nil || !strings.Contains(err.Error(), "not one this build knows") {
		t.Fatalf("an unknown region must be refused on every profile, got %v", err)
	}
	if _, err := ParseRouting(doc("sovereign", azureTestRegion)); err == nil || !strings.Contains(err.Error(), "sovereign forbids cloud provider") {
		t.Fatalf("sovereign must refuse Azure as any cloud provider, got %v", err)
	}
}

func TestAzureRegionIsAcceptedInAnyCaseAndStoredInLower(t *testing.T) {
	t.Parallel()
	doc := "profile: eu_hosted\nproviders:\n  azure_openai: {base_url: " + azureTestEndpoint + ", location: ' SwedenCentral '}\n" +
		"tiers:\n  premium: {provider: azure_openai, model: my-gpt}\nembeddings: {provider: azure_openai, model: embed-dep}\n"
	cfg, err := ParseRouting([]byte(doc))
	if err != nil {
		t.Fatalf("a mixed-case region: %v", err)
	}
	if got := cfg.Providers[providerAzureOpenAI].Location; got != azureTestRegion {
		t.Errorf("stored location = %q, want %q", got, azureTestRegion)
	}
	if got := cfg.Tiers[TierPremium].Location; got != azureTestRegion {
		t.Errorf("resolved lane location = %q, want %q", got, azureTestRegion)
	}
	if country, known := azureRegionCountry("  GermanyWestCentral"); !known || country != "DE" {
		t.Errorf("azureRegionCountry mixed case = %q, %v", country, known)
	}
}

// A deployment name is the customer's own. Azure's model list offers ids no
// binding can use, so the list is not published and no suggestion is offered.
func azureStoreAnswering(t *testing.T) (*RoutingStore, RoutingConfig, keyProbes) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"}]}`)); err != nil {
			t.Errorf("writing the fixture body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	selector := brainSelector(func(cfg ProviderConfig, keys config.Lookup) (model.Client, error) {
		return selectBrainOn(cfg, keys, &http.Client{Transport: azureFixture{base: srv.URL}})
	})
	cfg := RoutingConfig{
		Profile:   ProfileCloudFrontier,
		Providers: map[string]ProviderSettings{providerAzureOpenAI: {BaseURL: azureTestEndpoint, Location: azureTestRegion}},
	}
	return &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}, cfg, keyProbes{brain: selector.build}
}

func TestAzureOffersNoModelIDsAsSuggestions(t *testing.T) {
	t.Parallel()
	store, cfg, _ := azureStoreAnswering(t)
	got := store.availableModels(context.Background(), cfg, AvailableModelsQuery{Provider: providerAzureOpenAI})
	if got.Unavailable != AvailabilityNotPublished || len(got.Models) != 0 {
		t.Fatalf("azure_openai list = %+v, want not_published with no models", got)
	}
}

func TestAzureKeyTestPassesWithoutCountingModels(t *testing.T) {
	t.Parallel()
	_, cfg, probes := azureStoreAnswering(t)
	got := probeProviderKey(context.Background(), cfg, providerAzureOpenAI, allCloudKeys(t), probes)
	if !got.OK || got.Counted {
		t.Fatalf("key test = %+v, want OK and not counted", got)
	}
}
