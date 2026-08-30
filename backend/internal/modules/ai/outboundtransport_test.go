// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// TestOutboundClientKeepsItsConnectionsHonest proves the shared transport can
// notice a connection the far end has abandoned, instead of handing it to the
// next request to wait out the five-minute ceiling on.
//
// This is the shape that cost a re-embed run four of its five attempts: a
// dropped connection stayed in the pool looking healthy, and each request handed
// to it stalled for minutes with the caller's database transaction open.
//
// It asserts NO response-header timeout, deliberately. A completion is sent with
// stream:false, so the vendor holds its headers until generation finishes and a
// header deadline would cut exactly the long answers requestTimeout is generous
// for. The embed lane's own deadline is what bounds the calls whose duration is
// actually known — see TestEmbedLaneBoundsOneCall.
func TestOutboundClientKeepsItsConnectionsHonest(t *testing.T) {
	t.Parallel()

	client := newOutboundClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the outbound client's transport is %T, so nothing retires a dead connection", client.Transport)
	}
	if transport.HTTP2 == nil || transport.HTTP2.SendPingTimeout != http2PingAfterIdle {
		t.Fatalf("the HTTP/2 keep-alive ping is %v, want %v — without it a dropped h2 connection stays in the pool looking healthy",
			transport.HTTP2, http2PingAfterIdle)
	}
	if transport.HTTP2.PingTimeout != http2PingTimeout {
		t.Fatalf("PingTimeout is %v, want %v", transport.HTTP2.PingTimeout, http2PingTimeout)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("HTTP/2 is not enabled, so the ping settings above bind nothing")
	}
	if transport.IdleConnTimeout != idleConnTimeout {
		t.Fatalf("IdleConnTimeout is %v, want %v", transport.IdleConnTimeout, idleConnTimeout)
	}
	if transport.ResponseHeaderTimeout != 0 {
		t.Fatalf("ResponseHeaderTimeout is %v, but a stream:false completion holds its headers until generation finishes — this cuts long answers short",
			transport.ResponseHeaderTimeout)
	}
	if client.Timeout != requestTimeout {
		t.Fatalf("the call ceiling is %v, want %v", client.Timeout, requestTimeout)
	}
}

// TestEmbedLaneBoundsOneCall proves Router.Embed puts a deadline on every
// embedding call, so a stalled connection costs EmbedCallTimeout rather than
// the whole five-minute ceiling.
//
// The lane is where the bound belongs because it is the one place the expected
// duration is known: an embedding is a single forward pass and answers in about
// a second, where a completion legitimately takes minutes. The assertion reads
// the deadline the embedder was actually handed — a router that forwards the
// caller's context unchanged hands it none, which is the defect this holds shut.
func TestEmbedLaneBoundsOneCall(t *testing.T) {
	t.Parallel()

	if EmbedCallTimeout <= 0 || EmbedCallTimeout >= requestTimeout {
		t.Fatalf("EmbedCallTimeout is %v, which does not bound anything inside the %v ceiling", EmbedCallTimeout, requestTimeout)
	}

	spy := &deadlineSpyEmbedder{}
	router := routerWithEmbedder(t, spy)
	if _, err := router.Embed(workspaceCtx(), model.EmbedRequest{Inputs: []string{"x"}, Dimensions: 4}); !errors.Is(err, errStubEmbedderRefused) {
		t.Fatalf("Embed reached something other than the spy: %v", err)
	}
	if !spy.hadDeadline {
		t.Fatal("the embedder was handed a context with no deadline: a stalled embedding call holds the caller for the whole ceiling")
	}
	if spy.remaining > EmbedCallTimeout || spy.remaining < EmbedCallTimeout/2 {
		t.Fatalf("the embedder was given %v, want about %v", spy.remaining, EmbedCallTimeout)
	}
}

// TestEmbedLaneKeepsATighterCallerDeadline proves the lane's timeout only ever
// SHORTENS: a caller already nearly out of time is not handed a fresh minute,
// which would outlive the request it belongs to.
func TestEmbedLaneKeepsATighterCallerDeadline(t *testing.T) {
	t.Parallel()

	spy := &deadlineSpyEmbedder{}
	router := routerWithEmbedder(t, spy)
	ctx, cancel := context.WithTimeout(workspaceCtx(), 2*time.Second)
	defer cancel()
	if _, err := router.Embed(ctx, model.EmbedRequest{Inputs: []string{"x"}, Dimensions: 4}); !errors.Is(err, errStubEmbedderRefused) {
		t.Fatalf("Embed reached something other than the spy: %v", err)
	}
	if spy.remaining > 3*time.Second {
		t.Fatalf("the embedder was given %v against a caller deadline of 2s — the lane widened someone else's deadline", spy.remaining)
	}
}

// deadlineSpyEmbedder answers a valid embedding and records the deadline it was
// called under, which is the only place the lane's bound is observable.
type deadlineSpyEmbedder struct {
	hadDeadline bool
	remaining   time.Duration
}

func (d *deadlineSpyEmbedder) Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	deadline, ok := ctx.Deadline()
	d.hadDeadline = ok
	if ok {
		d.remaining = time.Until(deadline)
	}
	// Refused rather than answered: the lane meters a SUCCESSFUL call, and a
	// Router built without a meter (this test needs no database) would panic
	// there. What is under test is the deadline the embedder was handed, which
	// is already recorded above.
	return model.Embeddings{}, errStubEmbedderRefused
}

// errStubEmbedderRefused is the spy's refusal, kept as a sentinel so the tests
// can tell it apart from a real routing fault.
var errStubEmbedderRefused = errors.New("stub embedder: recorded the deadline, refusing the call")

func (d *deadlineSpyEmbedder) Caps() model.Capabilities { return model.Capabilities{} }

func (d *deadlineSpyEmbedder) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, errors.New("deadlineSpyEmbedder serves the embed lane only")
}

func (d *deadlineSpyEmbedder) Stream(context.Context, model.Request) (model.TokenStream, error) {
	return nil, errors.New("deadlineSpyEmbedder serves the embed lane only")
}

// routerWithEmbedder builds a Router whose embed lane is the given client, so a
// test can observe what the lane hands it. The chat tiers are the offline fake;
// nothing here calls them.
func routerWithEmbedder(t *testing.T, embedder model.Client) *Router {
	t.Helper()
	cfg := FakeRoutingConfig()
	cfg.Embeddings = EmbeddingsConfig{
		ProviderConfig: ProviderConfig{Provider: ProviderFake, Model: "spy"},
		Dimensions:     4,
	}
	router, err := NewRouter(cfg, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// A copy, so the swap replaces the whole binding the way rebinding does
	// rather than mutating one another goroutine may be reading.
	bound := *router.binding()
	bound.embedder = embedder
	router.bound.Store(&bound)
	return router
}

// workspaceCtx is the workspace-bound context Router.Embed requires; which
// workspace is immaterial, only that one is bound.
func workspaceCtx() context.Context {
	return principal.WithWorkspaceID(context.Background(), ids.NewV7())
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
		if transport.HTTP2 == nil || transport.HTTP2.SendPingTimeout != http2PingAfterIdle {
			t.Errorf("%s: no HTTP/2 keep-alive ping — a dropped connection to this vendor stalls until the ceiling", provider)
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
