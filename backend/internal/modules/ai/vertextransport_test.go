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
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestTheVertexHostFollowsTheLocation(t *testing.T) {
	t.Parallel()
	for location, want := range map[string]string{
		"eu":           "https://aiplatform.eu.rep.googleapis.com",
		"us":           "https://aiplatform.us.rep.googleapis.com",
		"global":       "https://aiplatform.googleapis.com",
		"europe-west4": "https://europe-west4-aiplatform.googleapis.com",
		"us-central1":  "https://us-central1-aiplatform.googleapis.com",
	} {
		if err := vertexLocationError("tier premium", location); err != nil {
			t.Errorf("location %q was refused: %v", location, err)
		}
		if got := vertexHost(location); got != want {
			t.Errorf("location %q is served from %s, want %s", location, got, want)
		}
	}
	for _, location := range []string{"", "EU", "eu.attacker.example", "europe-west4/../x", "europe-west4:443", "attacker.example#", "europe-west123"} {
		if err := vertexLocationError("tier premium", location); err == nil {
			t.Errorf("location %q was admitted, and the host is built from it", location)
		}
	}
}

// googleRecorder notes what each API call was addressed to, before the
// fixture reroutes it.
type googleRecorder struct {
	googleFixture
	mu    sync.Mutex
	calls []string
	auth  []string
}

func (g *googleRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != vertexTokenURI {
		g.mu.Lock()
		g.calls = append(g.calls, req.URL.String())
		g.auth = append(g.auth, req.Header.Get("Authorization"))
		g.mu.Unlock()
	}
	return g.googleFixture.RoundTrip(req)
}

func vertexAt(t *testing.T, location string, handler http.HandlerFunc) (model.Client, *googleRecorder) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	recorder := &googleRecorder{googleFixture: googleFixture{base: srv.URL}}
	client, err := selectBrainOn(
		ProviderConfig{Provider: providerGeminiVertex, Location: location, Model: "gemini-3.5-flash"},
		allCloudKeys(t), &http.Client{Timeout: CallCeiling, Transport: recorder},
	)
	if err != nil {
		t.Fatal(err)
	}
	return client, recorder
}

func writeBody(t *testing.T, w io.Writer, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("writing the fixture: %v", err)
	}
}

func TestAVertexBindingCallsItsProjectAtItsLocation(t *testing.T) {
	t.Parallel()
	client, recorder := vertexAt(t, "europe-west4", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":streamGenerateContent"):
			writeBody(t, w, `data: {"candidates":[{"content":{"parts":[{"text":"he"}]}}]}`+"\n\n")
			writeBody(t, w, `data: {"candidates":[{"content":{"parts":[{"text":"llo"}]},"finishReason":"STOP"}]}`+"\n\n")
		case strings.HasSuffix(r.URL.Path, ":embedContent"):
			writeBody(t, w, `{"embedding":{"values":[0.5,0.25]}}`)
		default:
			writeBody(t, w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.5-flash-001"}`)
		}
	})
	ctx := context.Background()
	if resp, err := client.Complete(ctx, model.Request{Messages: []model.Message{{Role: roleUser, Content: "hi"}}}); err != nil || resp.Text != "ok" {
		t.Fatalf("complete: %q, %v", resp.Text, err)
	}
	stream, err := client.Stream(ctx, model.Request{Messages: []model.Message{{Role: roleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var streamed strings.Builder
	for {
		chunk, more, err := stream.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
		streamed.WriteString(chunk)
	}
	if err := stream.Close(); err != nil || streamed.String() != "hello" {
		t.Fatalf("stream read %q, close %v", streamed.String(), err)
	}
	embedded, err := client.Embed(ctx, model.EmbedRequest{Model: "gemini-embedding-001", Inputs: []string{"a"}})
	if err != nil || embedded.Dims != 2 {
		t.Fatalf("embed: %+v, %v", embedded, err)
	}

	const models = "https://europe-west4-aiplatform.googleapis.com/v1/projects/margince-eu-1/locations/europe-west4/publishers/google/models/"
	want := []string{
		models + "gemini-3.5-flash:generateContent",
		models + "gemini-3.5-flash:streamGenerateContent?alt=sse",
		models + "gemini-embedding-001:embedContent",
	}
	if strings.Join(recorder.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("the calls went to\n%s\nwant\n%s", strings.Join(recorder.calls, "\n"), strings.Join(want, "\n"))
	}
	for i, auth := range recorder.auth {
		if auth != "Bearer ya29.fixture" {
			t.Errorf("call %d carried Authorization %q, want the minted bearer token", i, auth)
		}
	}
}

func TestAVertexCallFollowsNoRedirect(t *testing.T) {
	t.Parallel()
	var followed atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed.Add(1) }))
	t.Cleanup(elsewhere.Close)
	client, _ := vertexAt(t, "eu", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	})
	_, err := client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: roleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "http 307") {
		t.Fatalf("want the redirect answered as a refusal, got %v", err)
	}
	if _, err := client.(model.Lister).ListModels(context.Background()); err == nil {
		t.Fatal("a redirected model list was read as an answer")
	}
	if followed.Load() != 0 {
		t.Errorf("the bearer token was carried to the redirect target %d time(s)", followed.Load())
	}
}

func TestAVertexTokenThatCannotBeMintedSendsNothing(t *testing.T) {
	t.Parallel()
	var sent atomic.Int32
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == vertexTokenURI {
			return tokenReply{status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`}.response(), nil
		}
		sent.Add(1)
		return tokenReply{status: http.StatusOK, body: `{}`}.response(), nil
	})
	client, err := selectBrainOn(ProviderConfig{Provider: providerGeminiVertex, Location: "eu", Model: "m"}, allCloudKeys(t), &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), model.Request{Messages: []model.Message{{Role: roleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("want the token refusal, got %v", err)
	}
	if sent.Load() != 0 {
		t.Errorf("%d request(s) went out without a token", sent.Load())
	}
}

func TestVertexListsOnlyTheGeminiFamiliesFromThePublisherCatalog(t *testing.T) {
	t.Parallel()
	client, recorder := vertexAt(t, "eu", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			writeBody(t, w, `{"publisherModels":[{"name":"publishers/google/models/gemini-3.5-flash"},{"name":"publishers/google/models/imagen-4.0-generate-001"}],"nextPageToken":"p2"}`)
			return
		}
		writeBody(t, w, `{"publisherModels":[{"name":"publishers/google/models/gemini-embedding-001"},{"name":"publishers/google/models/text-embedding-005"},{"name":"publishers/google/models/veo-3.0"}]}`)
	})
	models, err := client.(model.Lister).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Info{
		{ID: "gemini-3.5-flash", Lane: model.LaneChat},
		{ID: "gemini-embedding-001", Lane: model.LaneEmbeddings},
		{ID: "text-embedding-005", Lane: model.LaneEmbeddings},
	}
	if len(models) != len(want) {
		t.Fatalf("listed %+v, want %+v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Errorf("model %d = %+v, want %+v", i, models[i], want[i])
		}
	}
	if !strings.HasPrefix(recorder.calls[0], vertexPublisherModelsURL+"?pageSize=100") || recorder.auth[0] != "Bearer ya29.fixture" {
		t.Errorf("the list was %s with %q, want the global publisher catalog with the bearer token", recorder.calls[0], recorder.auth[0])
	}
}

func TestSelectBrainRefusesAVertexBindingItCannotAddress(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		cfg   ProviderConfig
		keys  config.Lookup
		names string
	}{
		"no key":      {ProviderConfig{Provider: providerGeminiVertex, Location: "eu"}, noCloudKeys(), "GEMINI_VERTEX_SA_JSON"},
		"an API key":  {ProviderConfig{Provider: providerGeminiVertex, Location: "eu"}, cloudKeyFor(providerGeminiVertex, "AIza-not-a-key-file"), "not a JSON object"},
		"no location": {ProviderConfig{Provider: providerGeminiVertex}, allCloudKeys(t), "needs a `location`"},
	} {
		_, err := SelectBrain(tc.cfg, tc.keys)
		if err == nil || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: want a refusal naming %q, got %v", name, tc.names, err)
		}
		if name == "an API key" && !errors.Is(err, errInvalidServiceAccount) {
			t.Errorf("%s: the refusal is not an invalid-key error: %v", name, err)
		}
	}
}

func TestOnlyAVertexBindingTakesALocation(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		binding ProviderConfig
		names   string
	}{
		"a location on AI Studio": {ProviderConfig{Provider: providerGemini, Model: "m", Location: "eu"}, "applies only to gemini_vertex"},
		"a base_url on Vertex":    {ProviderConfig{Provider: providerGeminiVertex, Model: "m", Location: "eu", BaseURL: "https://x.example"}, "takes no base_url"},
		"no location on Vertex":   {ProviderConfig{Provider: providerGeminiVertex, Model: "m"}, "needs a `location`"},
	} {
		err := ValidateTierBinding(ProfileCloudHosted, TierPremium, tc.binding)
		if err == nil || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: want a refusal naming %q, got %v", name, tc.names, err)
		}
	}
}
