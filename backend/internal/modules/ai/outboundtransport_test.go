// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// TestOutboundClientGivesUpOnAVendorThatNeverAnswers proves a vendor that
// accepts a connection and then says nothing fails in responseHeaderTimeout
// rather than holding the caller for the full requestTimeout.
//
// This is the shape that cost a re-embed run four of its five attempts: a
// connection the network had dropped stayed in the pool, and each request handed
// to it waited out the five-minute ceiling with the caller's database
// transaction open underneath. The test drives the real client against a server
// that never writes a header, and asserts the deadline it hits is the header one.
func TestOutboundClientGivesUpOnAVendorThatNeverAnswers(t *testing.T) {
	t.Parallel()

	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-blocked // never writes a status line
	}))
	defer srv.Close()
	defer close(blocked)

	client := newOutboundClient()
	// The real constant is minutes long, which no test may wait out. Asserting
	// on the FIELD rather than a real elapsed deadline is what keeps this test
	// honest and fast: the client under test is the one SelectBrain hands every
	// adapter, and the timeout is read off it, not restated.
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the outbound client's transport is %T, so nothing bounds its response headers", client.Transport)
	}
	if transport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Fatalf("ResponseHeaderTimeout is %v, want %v — a vendor that stops answering mid-connection holds the caller for the whole %v ceiling",
			transport.ResponseHeaderTimeout, responseHeaderTimeout, requestTimeout)
	}
	if transport.HTTP2 == nil || transport.HTTP2.SendPingTimeout == 0 {
		t.Fatal("no HTTP/2 keep-alive ping is configured, so a dropped h2 connection stays in the pool looking healthy")
	}
	if transport.IdleConnTimeout != idleConnTimeout {
		t.Fatalf("IdleConnTimeout is %v, want %v", transport.IdleConnTimeout, idleConnTimeout)
	}

	// And the bound is real, not only declared: the same transport with the
	// timeout dialled down to something a test may wait out must actually give
	// up on this server.
	fast := transport.Clone()
	fast.ResponseHeaderTimeout = 100 * time.Millisecond
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	start := time.Now()
	resp, err := (&http.Client{Transport: fast}).Do(req)
	if err == nil {
		//craft:ignore swallowed-errors best-effort close of a body this test never reads — the assertion below is the outcome
		_ = resp.Body.Close()
		t.Fatal("a server that never writes a header answered successfully")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("the request waited %v on a silent server; ResponseHeaderTimeout is not bounding it", waited)
	}
}

// TestEveryCloudAdapterUsesTheHardenedTransport is the census: every provider
// SelectBrain can build must get the hardened client, because a single adapter
// left on http.DefaultTransport is a vendor whose stalls nothing bounds — and
// the one most likely to be missed is the one added next.
//
// It derives its subject from KnownProviders() rather than a hand-written list,
// so a provider added without a transport fails here instead of shipping
// unbounded.
func TestEveryCloudAdapterUsesTheHardenedTransport(t *testing.T) {
	t.Parallel()

	// Every cloud adapter needs its BYOK key present or SelectBrain refuses
	// before it builds a client; the value is never sent anywhere here.
	keys := config.Lookup(func(string) string { return "k" })
	for _, provider := range KnownProviders() {
		if provider == ProviderFake {
			continue // the offline stub makes no outbound call at all
		}
		cfg := ProviderConfig{Provider: provider, Model: "m", BaseURL: "https://vendor.example"}
		client, err := SelectBrain(cfg, keys)
		if err != nil {
			t.Fatalf("SelectBrain(%s): %v", provider, err)
		}
		httpClient := outboundHTTPClientOf(t, client)
		if httpClient.Timeout != requestTimeout {
			t.Errorf("%s: call timeout is %v, want %v", provider, httpClient.Timeout, requestTimeout)
		}
		transport, ok := httpClient.Transport.(*http.Transport)
		if !ok {
			t.Errorf("%s: transport is %T, not the hardened one — its stalls are bounded by nothing but the ceiling", provider, httpClient.Transport)
			continue
		}
		if transport.ResponseHeaderTimeout != responseHeaderTimeout {
			t.Errorf("%s: ResponseHeaderTimeout is %v, want %v", provider, transport.ResponseHeaderTimeout, responseHeaderTimeout)
		}
	}
}

// outboundHTTPClientOf reaches the *http.Client an adapter holds. Each adapter
// keeps it in an unexported `http` field of its own struct type, so this reads
// it the one way a test in the same package can.
func outboundHTTPClientOf(t *testing.T, client model.Client) *http.Client {
	t.Helper()
	switch c := client.(type) {
	case *anthropicClient:
		return c.http
	case *openaiClient:
		return c.http
	case *geminiClient:
		return c.http
	case *ollamaClient:
		return c.http
	case *openAICompatClient:
		return c.http
	default:
		t.Fatalf("unknown adapter type %T — add it here, or its outbound calls are unbounded", client)
		return nil
	}
}
