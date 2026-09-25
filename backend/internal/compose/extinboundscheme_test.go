// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// providerProbe is a unit with one provider-signed endpoint and a handshake,
// recording what reached each.
type providerProbe struct {
	saw          extension.InboundRequest
	sawChallenge extension.InboundChallengeRequest
	calls        int
	challenges   int
	answer       string
	outcome      extension.InboundOutcome
	perIP        int
}

func (p *providerProbe) unit() extension.Extension {
	endpoint := inboundEndpointFixture("messenger", "inbound")
	endpoint.Skew = 0
	endpoint.Scheme = extension.SchemeProviderSigned
	endpoint.SignatureHeader = "X-Hub-Signature-256"
	if p.perIP > 0 {
		endpoint.Rate.PerIP = extension.Rate{Limit: p.perIP, Window: time.Minute}
	}
	endpoint.Handle = func(_ context.Context, _ extension.Runtime, req extension.InboundRequest) (extension.InboundOutcome, error) {
		p.calls++
		p.saw = req
		return extension.InboundAccepted, nil
	}
	endpoint.Challenge = func(_ context.Context, _ extension.Runtime, req extension.InboundChallengeRequest) (string, extension.InboundOutcome, error) {
		p.challenges++
		p.sawChallenge = req
		return p.answer, p.outcome, nil
	}
	return unitWithInbound(probeUnitName, endpoint)
}

func mountProvider(t *testing.T, p *providerProbe) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	ws := ids.From[ids.WorkspaceKind](ids.MustParse("00000000-0000-0000-0000-0000000000a1"))
	MountInboundEndpoints(mux, []extension.Extension{p.unit()},
		func(context.Context) (ids.WorkspaceID, error) { return ws, nil },
		extensionRuntimeBinding{}, quietLog())
	return mux
}

func providerPost(body, signature string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/webhooks/ext/u/messenger/ref1", strings.NewReader(body))
	r.RemoteAddr = "10.0.0.1:1234"
	if signature != "" {
		r.Header.Set("X-Hub-Signature-256", signature)
	}
	return r
}

func handshakeGet(query string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/webhooks/ext/u/messenger/ref1?"+query, nil)
	r.RemoteAddr = "10.0.0.1:1234"
	return r
}

func TestAProviderPostCarriesItsSignatureAndNoClock(t *testing.T) {
	p := &providerProbe{}
	w := serve(mountProvider(t, p), providerPost(`{"object":"page"}`, "sha256=abc"))
	if w.Code != http.StatusAccepted {
		t.Fatalf("answered %d, want 202", w.Code)
	}
	if p.saw.Signature != "sha256=abc" || p.saw.Ref != "ref1" || string(p.saw.Body) != `{"object":"page"}` {
		t.Fatalf("handler saw %+v", p.saw)
	}
	if !p.saw.Timestamp.IsZero() || p.saw.Nonce != "" {
		t.Fatalf("a provider-signed request carries no clock, but the handler saw timestamp %v nonce %q", p.saw.Timestamp, p.saw.Nonce)
	}
}

func TestAProviderPostWithoutItsHeaderIsTheOpaque401(t *testing.T) {
	p := &providerProbe{}
	mux := mountProvider(t, p)
	bare := providerPost(`{}`, "")
	marginceOnly := providerPost(`{}`, "")
	marginceOnly.Header.Set(extension.InboundHeaderTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
	marginceOnly.Header.Set(extension.InboundHeaderNonce, "0f1e2d3c")
	marginceOnly.Header.Set(extension.InboundHeaderSignature, "sha256=deadbeef")
	for name, r := range map[string]*http.Request{"no header": bare, "Margince headers only": marginceOnly} {
		w := serve(mux, r)
		if w.Code != http.StatusUnauthorized || w.Body.Len() != 0 {
			t.Fatalf("%s: answered %d %q, want an empty 401", name, w.Code, w.Body.String())
		}
	}
	if p.calls != 0 {
		t.Fatal("an unsigned request reached the unit")
	}
}

func TestAProviderPostRefusesAnOversizedOrRepeatedSignature(t *testing.T) {
	p := &providerProbe{}
	mux := mountProvider(t, p)
	oversized := providerPost(`{}`, "sha256="+strings.Repeat("a", extension.MaxInboundSignatureHeader))
	repeated := providerPost(`{}`, "sha256=abc")
	repeated.Header.Add("X-Hub-Signature-256", "sha256=def")
	for name, r := range map[string]*http.Request{"oversized": oversized, "repeated": repeated} {
		if got := serve(mux, r).Code; got != http.StatusUnauthorized {
			t.Fatalf("%s signature answered %d, want 401", name, got)
		}
	}
	if p.calls != 0 {
		t.Fatal("a malformed signature reached the unit")
	}
}

func TestAHandshakeReachesChallengeWithOnlyHubParameters(t *testing.T) {
	p := &providerProbe{answer: "1158201444", outcome: extension.InboundAccepted}
	w := serve(mountProvider(t, p), handshakeGet("hub.mode=subscribe&hub.challenge=1158201444&hub.verify_token=t0k&other=x"))
	if w.Code != http.StatusOK || w.Body.String() != "1158201444" {
		t.Fatalf("answered %d %q, want 200 with the challenge", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q — the answer is a caller-chosen value and must not be sniffed as markup", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	want := map[string]string{"hub.mode": "subscribe", "hub.challenge": "1158201444", "hub.verify_token": "t0k"}
	if len(p.sawChallenge.Query) != len(want) {
		t.Fatalf("the unit saw %v — only hub.* parameters may reach it", p.sawChallenge.Query)
	}
	for k, v := range want {
		if p.sawChallenge.Query[k] != v {
			t.Fatalf("the unit saw %s=%q, want %q", k, p.sawChallenge.Query[k], v)
		}
	}
	if p.sawChallenge.Ref != "ref1" || p.sawChallenge.Slug != "messenger" {
		t.Fatalf("the unit saw slug %q ref %q", p.sawChallenge.Slug, p.sawChallenge.Ref)
	}
}

func TestARefusedHandshakeIsTheOpaque401(t *testing.T) {
	p := &providerProbe{answer: "ignored", outcome: extension.InboundUnauthenticated}
	w := serve(mountProvider(t, p), handshakeGet("hub.mode=subscribe&hub.challenge=1&hub.verify_token=wrong"))
	if w.Code != http.StatusUnauthorized || w.Body.Len() != 0 {
		t.Fatalf("answered %d %q, want an empty 401", w.Code, w.Body.String())
	}
}

func TestAHandshakeQueryIsBoundedBeforeTheUnitSeesIt(t *testing.T) {
	var many []string
	for i := 0; i <= extension.MaxInboundChallengeParams; i++ {
		many = append(many, "hub.p"+strconv.Itoa(i)+"=1")
	}
	cases := map[string]string{
		"too many parameters": strings.Join(many, "&"),
		"a repeated one":      "hub.mode=a&hub.mode=b",
		"an oversized value":  "hub.challenge=" + strings.Repeat("9", extension.MaxInboundChallengeValue+1),
		"an oversized query":  "x=" + strings.Repeat("9", extension.MaxInboundChallengeQuery),
	}
	for name, query := range cases {
		p := &providerProbe{answer: "1", outcome: extension.InboundAccepted}
		if got := serve(mountProvider(t, p), handshakeGet(query)).Code; got != http.StatusUnauthorized {
			t.Fatalf("%s answered %d, want 401", name, got)
		}
		if p.challenges != 0 {
			t.Fatalf("%s reached the unit", name)
		}
	}
}

func TestAHandshakeAnswerOverTheCapIsAServerError(t *testing.T) {
	p := &providerProbe{answer: strings.Repeat("a", extension.MaxInboundChallengeAnswer+1), outcome: extension.InboundAccepted}
	if got := serve(mountProvider(t, p), handshakeGet("hub.challenge=1")).Code; got != http.StatusInternalServerError {
		t.Fatalf("answered %d, want 500", got)
	}
}

func TestAHandshakeIsMeteredLikeAPost(t *testing.T) {
	p := &providerProbe{answer: "1", outcome: extension.InboundAccepted, perIP: 1}
	mux := mountProvider(t, p)
	if got := serve(mux, handshakeGet("hub.challenge=1")).Code; got != http.StatusOK {
		t.Fatalf("first handshake answered %d", got)
	}
	if got := serve(mux, handshakeGet("hub.challenge=1")).Code; got != http.StatusTooManyRequests {
		t.Fatalf("second handshake over a one-request allowance answered %d, want 429", got)
	}
}

func TestAProviderEndpointRefusesOtherMethods(t *testing.T) {
	p := &providerProbe{}
	r := httptest.NewRequest(http.MethodPut, "/webhooks/ext/u/messenger/ref1", nil)
	if got := serve(mountProvider(t, p), r).Code; got != http.StatusMethodNotAllowed {
		t.Fatalf("PUT answered %d, want 405", got)
	}
}
