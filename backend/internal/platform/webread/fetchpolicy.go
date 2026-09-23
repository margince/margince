// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// The fetch policy: which addresses this product will read at all.
//
// It lives HERE, beside robots and the SSRF guard, because it is a decision
// about fetching rather than about searching. It was written in the websearch
// port, where only a consumer of the Brave seam could reach it — and in
// practice no consumer did: it was written, tested, and called by nothing,
// while the enrich tool handed a caller-supplied URL straight to this fetcher.
//
// What actually refused a denied host was the host's OWN robots.txt. That is a
// real guard and it fails closed here, but it is somebody else's document: a
// platform may permit a path in robots and still prohibit automated collection
// in its terms, and a host serving no robots.txt allows everything. A promise
// this product makes cannot rest on a file the other party writes.
//
// Asked BEFORE robots, which is the whole point of the ordering: fetching a
// denied platform's robots.txt is itself a request to that platform, and "never
// fetched" has to mean never.

import (
	"errors"
	"net/url"
	"strings"
)

// ErrFetchPolicy marks an address this product refuses to read on its own
// account, as opposed to one the target site refuses (ErrRobotsDisallowed).
// Separate errors because the two are different promises to different parties,
// and an operator reading a run record needs to know which one fired.
var ErrFetchPolicy = errors.New("webread: this product does not fetch this address")

// deniedHosts are the platforms this product never fetches, whatever a search
// returns.
//
// The reason is contract law rather than data protection, and the distinction
// matters because the two have different remedies: these platforms' terms
// prohibit automated collection, and being public data does not touch that.
// LinkedIn's sanctioned channels stay what ADR-0078 §8 made them — the
// member's own portability export and CSV — and this seam does not add a
// third by the back door.
//
// Subdomains are covered: the check is suffix-based, so `de.linkedin.com`
// is as denied as `www.linkedin.com`.
var deniedHosts = []string{
	"linkedin.com",
	"xing.com",
	"facebook.com",
	"instagram.com",
	"x.com",
	"twitter.com",
}

// FetchDecision is why a URL may or may not be read.
type FetchDecision struct {
	Allowed bool
	// Reason is operator-facing and states the RULE, not the URL — it goes
	// into the run-transparency record, where a reader needs to know which
	// policy fired rather than re-reading the address they can already see.
	Reason string
}

// MayFetch answers whether the fetch pipeline may read this URL.
//
// It is deliberately conservative about what it cannot parse: a URL this
// function cannot decompose is refused rather than passed through, because
// the failure mode of guessing is fetching something the policy exists to
// keep us away from.
func MayFetch(raw string) FetchDecision {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return FetchDecision{Reason: "the address could not be parsed, so no policy could be applied to it"}
	}
	if !isWebScheme(u.Scheme) {
		return FetchDecision{Reason: "only http and https are fetched"}
	}
	// Credentials in the address are refused before the deny-list is even
	// consulted. ADR-0081 forbids a credentialed fetch outright — a URL
	// carrying userinfo would have the fetcher present somebody's password to
	// a site this product has no account with, and no host allowlist makes
	// that acceptable.
	if u.User != nil {
		return FetchDecision{Reason: "the address carries credentials, and this product never fetches with them"}
	}
	host := normalizeHost(u.Hostname())
	for _, denied := range deniedHosts {
		if host == denied || strings.HasSuffix(host, "."+denied) {
			return FetchDecision{
				Reason: "the platform's terms prohibit automated collection; its results are cited, never fetched",
			}
		}
	}
	return FetchDecision{Allowed: true, Reason: "public page, fetched under robots and the site-read caps"}
}

// normalizeHost renders a hostname the way DNS resolves it, so the policy
// cannot be walked past by spelling the same host differently.
//
// The trailing dot is the one that matters and it is not theoretical:
// `linkedin.com.` is the fully-qualified form of `linkedin.com`, resolves to
// the same servers, and is a DIFFERENT string. A suffix check written
// against the raw hostname therefore allows a fetch the deny-list exists to
// refuse — a parser differential between this policy and the HTTP client
// underneath it. Case folds for the same reason.
func normalizeHost(host string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(host)), ".")
}

// isWebScheme reports whether a URL names a scheme this product fetches.
//
// One spelling, because three places asked it three ways: this policy folded
// case and the two page walkers did not, so `HTTPS://…` was a fetchable
// address to one of them and not to the others. A scheme is case-insensitive
// per RFC 3986 §3.1, and the answer here is the folding one.
func isWebScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}
