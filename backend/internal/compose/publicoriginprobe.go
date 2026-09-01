// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether the configured public origin actually answers, reported to an
// operator rather than enforced.
//
// Deliberately NOT a readiness check. Routing that sends traffic only to
// ready instances would deadlock on a cold rollout: nothing is ready, so
// the ingress answers nothing, so the origin looks unreachable, so
// nothing becomes ready. The deterministic boot and send guards in
// netguard are what actually stop the misconfiguration this exists for;
// this only tells somebody what the deployment looks like from here.
//
// It is also honest about how little a GET from INSIDE the deployment
// proves. It says this process can reach that origin. It cannot say a
// recipient's mail client can.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// originProbeTimeout bounds one attempt. Short: this answers a settings
	// screen, and a slow origin is a slow answer, not a hung page.
	originProbeTimeout = 1500 * time.Millisecond
	// originProbeTTL is how long an answer stands before it is asked again,
	// so a screen that polls does not turn into traffic against the ingress.
	originProbeTTL = 60 * time.Second
	// originProbeReadLimit is all that is ever read of the response. The
	// body is not the answer — the status is.
	originProbeReadLimit = 1 << 10
)

// PublicOriginStatus is what the Connections screen shows about the
// installation's outward address.
type PublicOriginStatus struct {
	// Origin is the configured value. Safe to show: it is a scheme and a
	// host an operator typed, and the boot guard refuses one carrying
	// userinfo.
	Origin string
	// Reachable is nil until the first probe answers, so a screen can say
	// "not checked yet" rather than implying a failure.
	Reachable *bool
	CheckedAt *time.Time
	// Detail is the HTTP status or the transport failure. Never more than
	// the origin itself — no path, and never a token.
	Detail string
}

// publicOriginProbe asks the configured origin whether it answers, at most
// once per TTL.
type publicOriginProbe struct {
	origin string
	client *http.Client
	clock  func() time.Time
	ttl    time.Duration

	mu   sync.Mutex
	last PublicOriginStatus
}

func newPublicOriginProbe(origin string, client *http.Client, clock func() time.Time) *publicOriginProbe {
	return &publicOriginProbe{
		origin: origin,
		client: client,
		clock:  clock,
		ttl:    originProbeTTL,
		last:   PublicOriginStatus{Origin: origin},
	}
}

// Status answers from the cache, refreshing it when the answer has aged
// past the TTL.
func (p *publicOriginProbe) Status(ctx context.Context) PublicOriginStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last.CheckedAt != nil && p.clock().Sub(*p.last.CheckedAt) < p.ttl {
		return p.last
	}
	p.last = p.probe(ctx)
	return p.last
}

// probe performs one attempt against the origin's health endpoint.
//
// Any status below 500 counts as reachable, and that is the question
// being asked: does this hostname resolve and answer over this scheme. A
// dev stack points the origin at Vite, which answers the SPA fallback
// rather than a health endpoint, and that is still a reachable origin.
func (p *publicOriginProbe) probe(ctx context.Context) PublicOriginStatus {
	at := p.clock()
	status := PublicOriginStatus{Origin: p.origin, CheckedAt: &at}
	reachable := false

	ctx, cancel := context.WithTimeout(ctx, originProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.origin+"/healthz", nil)
	if err != nil {
		status.Reachable = &reachable
		status.Detail = "the configured origin cannot be requested"
		return status
	}
	resp, err := p.client.Do(req)
	if err != nil {
		status.Reachable = &reachable
		// The URL is stripped from the transport error: it would otherwise
		// carry the origin into a screen and a log twice over, and the
		// operator already knows what they configured.
		status.Detail = "the origin did not answer"
		return status
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, originProbeReadLimit))

	reachable = resp.StatusCode < http.StatusInternalServerError
	status.Reachable = &reachable
	status.Detail = fmt.Sprintf("http %d", resp.StatusCode)
	return status
}

// newOriginProbeClient builds the client the probe dials with: bounded,
// and never following a redirect. A redirect is not followed because the
// question is what THIS origin answers, and because following one would
// dial wherever the answer pointed.
func newOriginProbeClient() *http.Client {
	return &http.Client{
		Timeout: originProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
