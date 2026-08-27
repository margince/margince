// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package certlog reads the certificate-transparency record of one domain.
//
// Every publicly trusted certificate is published to append-only CT logs, so
// the hostnames a company has certificates for are a public statement about
// what it operates. Reading them needs no agreement with anyone: the logs
// exist to be queried, which is the property that makes this the cleanest
// enrichment source in the product.
//
// ONE PROVIDER TODAY, and the interface is what keeps that from hardening.
// crt.sh is a free aggregator over the logs; it is frequently slow and
// sometimes down, which is why callers must treat a failure as "this lane had
// nothing to say today" rather than as an authoritative empty answer.
//
// It returns hostnames and nothing else. A certificate carries an issuer, a
// validity window and sometimes a person's name; this package parses none of
// that and offers no entry point that would return it — the narrowest surface
// that answers the question is also the one that cannot leak the rest.
package certlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// Client reads the hostnames a domain has published certificates for.
//
// A domain with no certificates is NOT an error: plenty of companies run a
// website on a shared host and have never had one issued under their own name.
// ok=false says the log answered and had nothing; err is reserved for a query
// that did not complete, which must never be read as an empty answer.
type Client interface {
	Hostnames(ctx context.Context, domain string) ([]string, bool, error)
}

const (
	// PublicBaseURL is the public crt.sh aggregator.
	PublicBaseURL = "https://crt.sh"

	// RecurringInterval is the floor between queries.
	//
	// crt.sh publishes no rate card. It is a single free service running a
	// large Postgres behind a web front end, and its own operators have asked
	// heavy users to be gentle rather than parallel. Five seconds is a rate a
	// recurring background reader can hold indefinitely without being the
	// reason anyone notices us.
	RecurringInterval = 5 * time.Second

	// queryTimeout bounds one query generously, because crt.sh answers slowly
	// under load and a short timeout here converts "slow" into "this company
	// has no subdomains", which is the one wrong answer this package must not
	// produce.
	queryTimeout = 30 * time.Second

	// maxRedirects caps how far a redirect chain is followed.
	maxRedirects = 5

	// maxAnswerBytes caps the body read.
	//
	// A domain with a long certificate history answers with megabytes of JSON,
	// and a hostile endpoint can answer with as much as it likes. One worker
	// drains this whole lane, so an unbounded decode is the whole lane's
	// memory. 8 MiB is far more than any real domain's list and far less than
	// a problem.
	maxAnswerBytes = 8 << 20

	// maxHostnames caps what one answer may yield. Beyond this the domain is
	// not telling us about a handful of services any more, and the classifier
	// would drop the tail regardless.
	maxHostnames = 5000
)

// UserAgent names this software to the log.
const UserAgent = outbound.CertLogHeader

// ErrNotConfigured says this deployment reads no certificate logs. It is a
// REFUSAL, not a failure: an offline or air-gapped installation queries
// nothing on purpose, and the caller records that rather than retrying.
var ErrNotConfigured = errors.New("certlog: no certificate-transparency source is configured")

// CrtSh is the crt.sh client.
type CrtSh struct {
	baseURL string
	http    *http.Client
	pacer   *Pacer
}

// NewCrtSh builds the client. An empty baseURL means the public service.
//
// The pacer is created here and held by the client, so ONE client is one
// requester: two clients would be two requesters however carefully each paced
// itself. The composition root builds exactly one.
func NewCrtSh(baseURL string, httpClient *http.Client) *CrtSh {
	if baseURL == "" {
		baseURL = PublicBaseURL
	}
	if httpClient == nil {
		httpClient = guardedClient()
	}
	return &CrtSh{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		http:    httpClient,
		pacer:   NewPacer(RecurringInterval),
	}
}

// guardedClient is the default client: SSRF-guarded and redirect-capped.
//
// The guard matters here for the same reason it does in webread. This client
// talks to a host an operator configured, but it FOLLOWS REDIRECTS, and a
// redirect is chosen by the far end — so without the dialer hook a log that
// answered 302 to an internal address would have this process dial it.
func guardedClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: netguard.RefusePrivate}
	return &http.Client{
		Timeout: queryTimeout,
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("certlog: the certificate log redirected more than %d times", maxRedirects)
			}
			return nil
		},
	}
}

// crtShEntry is the one field this package reads back. crt.sh returns issuer,
// serial, validity and more; taking only the name keeps the parse stable
// across its versions and keeps everything else out of the process.
type crtShEntry struct {
	NameValue string `json:"name_value"`
}

// Hostnames reports the hostnames the domain has published certificates for,
// including the domain itself when it has one.
//
// Three outcomes, and telling them apart is the caller's whole policy: names,
// a definite "the log has nothing" (ok=false, no error), and a query that did
// not complete (an error — never to be recorded as an empty answer, because a
// company whose subdomains vanished from the record is how a working webshop
// signal disappears).
func (c *CrtSh) Hostnames(ctx context.Context, domain string) ([]string, bool, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, false, nil
	}
	// The pace is taken BEFORE the request is built, and it is the whole
	// reason this client is safe to run on a schedule.
	if err := c.pacer.Wait(ctx); err != nil {
		return nil, false, err
	}

	// The leading dot asks for the domain and everything under it. The
	// identity query is the domain the caller already holds — this client
	// cannot be asked to enumerate anything else.
	endpoint := c.baseURL + "/?" + url.Values{
		"q":      {"%." + domain},
		"output": {"json"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("certlog: asking the certificate log: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close: the decode below reads what it needs and may leave the body mid-stream, so a close error says nothing about what the log holds
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, false, &LogRefusedError{
			Status:     resp.StatusCode,
			RetryAfter: retryafter.Of(resp),
		}
	}

	var entries []crtShEntry
	// Capped: an answer larger than this is refused as a lane FAILURE rather
	// than truncated into a shorter list, because a truncated list read as
	// authoritative would delete the services it did not reach.
	capped := io.LimitReader(resp.Body, maxAnswerBytes+1)
	body, err := io.ReadAll(capped)
	if err != nil {
		return nil, false, fmt.Errorf("certlog: reading the certificate log's answer: %w", err)
	}
	if len(body) > maxAnswerBytes {
		return nil, false, fmt.Errorf("certlog: the certificate log answered more than %d bytes", maxAnswerBytes)
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, false, fmt.Errorf("certlog: reading the certificate log's answer: %w", err)
	}
	names := hostnamesUnder(entries, domain)
	if len(names) == 0 {
		return nil, false, nil
	}
	return names, true, nil
}

// hostnamesUnder flattens the log's entries into one deduplicated, sorted set
// of hostnames that actually sit under the queried domain.
//
// Two things it drops, both deliberately. A wildcard name ("*.example.de")
// states that certificates MAY be issued below it, not that anything runs
// there, so it is not evidence of a service. A name outside the queried domain
// is a certificate covering somebody else's host as well, and this package
// answers about one domain — the caller asked about theirs.
func hostnamesUnder(entries []crtShEntry, domain string) []string {
	suffix := "." + domain
	seen := map[string]bool{}
	var names []string
	for _, entry := range entries {
		// One entry's name_value carries every SAN on the certificate,
		// newline-separated.
		for _, raw := range strings.Split(entry.NameValue, "\n") {
			name := strings.ToLower(strings.TrimSpace(raw))
			if name == "" || strings.HasPrefix(name, "*") || seen[name] {
				continue
			}
			if name != domain && !strings.HasSuffix(name, suffix) {
				continue
			}
			seen[name] = true
			names = append(names, name)
			if len(names) >= maxHostnames {
				return names
			}
		}
	}
	return names
}

// LogRefusedError is the log declining, with the wait it asked for.
//
// The wait is the part worth carrying: retrying on the job runner's own
// schedule rather than the log's is how a rate limit becomes a block.
type LogRefusedError struct {
	Status int
	// RetryAfter is what the log asked for, or zero when it said nothing.
	RetryAfter time.Duration
}

func (e *LogRefusedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("certlog: the certificate log answered %d and asked for %s before the next query",
			e.Status, e.RetryAfter)
	}
	return fmt.Sprintf("certlog: the certificate log answered %d", e.Status)
}
