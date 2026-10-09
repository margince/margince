// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// WHAT went wrong technically, said in the product's own words.
//
// The vocabulary beside this one classifies by SENTINEL: a job returned
// apperrors.ErrNotFound, so the record is gone. Most infrastructure failures
// carry no sentinel — they are a *net.DNSError, a refused dial, a handshake
// that timed out — and those all landed on the same sentence, which tells an
// operator only that the diagnosis is in a log they then have to go and read.
// "The provider's host name did not resolve" is a different morning.
//
// THE CAUSE IS READ AND NEVER PROJECTED. Every sentence here is authored: the
// classifier inspects the cause's SHAPE — its type, its sentinel, a status
// integer a typed error carries — and CHOOSES one of a closed set. No part of
// the cause's own text reaches the returned sentence, which is what keeps
// Fault's promise absolute: river_job.errors has no workspace column and no
// row-level protection, and a provider's prose routinely names the address or
// the record it refused. Reading a cause to pick a sentence is not publishing
// it, and a classifier that reached for the nearest match on a substring would
// be how the prose starts leaking by another name.
//
// AN UNRECOGNISED CAUSE GETS NOTHING. It falls through to the unclassified
// substitute exactly as before. A guess here would be worse than silence: the
// sentence is what an operator acts on, and one chosen by resemblance sends
// them after the wrong thing with the same confidence as a true one.

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"syscall"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// technicalFault is one recognisable failure shape and what the product says
// about it. Same three fields as the sentinel vocabulary, because both are read
// back through one surface and an operator must not be able to tell which table
// classified their failure.
type technicalFault struct {
	// match answers whether this is the shape. It reads the cause's type and
	// sentinels, never its message.
	match    func(error) bool
	class    string
	sentence string
	remedy   string
}

// technicalFaults is the closed set, most specific first.
//
// ORDER MATTERS between the transport entries: a DNS failure and a refused
// connection both surface as a *net.OpError, so the more specific test has to
// run first or every unresolved host would report as a refusal.
var technicalFaults = []technicalFault{
	{
		// FIRST, because an interrupted lookup or dial still arrives as a
		// *net.DNSError or *net.OpError: a job cancelled mid-request must read
		// as interrupted, not as a host that did not resolve.
		match:    isInterrupted,
		class:    "interrupted",
		sentence: "the job's work was cancelled before it finished",
		remedy:   "At a worker shutdown, nothing to do: the attempt is retried. A job interrupted at every shutdown runs longer than the worker's drain window, and the window (with the termination grace period around it) is what to raise. A job cancelled on purpose stays cancelled.",
	},
	{
		match:    isDNSFailure,
		class:    "host_unresolved",
		sentence: "the provider's host name did not resolve",
		remedy:   "Check the installation's DNS resolver and, if the host is right, whether the provider has retired it. A name that resolves nowhere does not fix itself on retry.",
	},
	{
		match:    isTLSFailure,
		class:    "tls_refused",
		sentence: "the TLS handshake with the provider did not complete",
		remedy:   "Check the installation's clock and its trust store first — an expired certificate and a wrong system time look identical from here — then whether an inspecting proxy sits in front of this connection.",
	},
	{
		match:    isConnectionRefused,
		class:    "connection_refused",
		sentence: "the provider refused the connection",
		remedy:   "Nothing to re-queue yet: the port answered and said no. Check whether the provider is in a maintenance window and whether this installation's egress address is allowed.",
	},
	{
		match:    isTimeout,
		class:    "provider_timed_out",
		sentence: "the provider did not answer in time",
		remedy:   "Retry normally. Persistent timeouts against one provider mean either their outage or a route that cannot carry this volume, and the run's own cadence is the next thing to look at.",
	},
	{
		match:    providerStatusIn(401, 403),
		class:    "provider_rejected_credential",
		sentence: "the provider rejected this installation's credential",
		remedy:   "Re-authorise the connection. A revoked token, a changed password and a withdrawn app grant all arrive exactly here.",
	},
	{
		match:    providerServerError,
		class:    "provider_server_error",
		sentence: "the provider answered with a server error",
		remedy:   "Theirs to fix. Retry is correct and the run does it; a day of these is a status page to read rather than a change to make here.",
	},
	{
		match:    siteRefused,
		class:    "site_refused_the_read",
		sentence: "the website refused the read",
		remedy:   "Bot protection or a rate limit on the site's side; the read is retried later. Nothing to change here, and the company's own site-read record says the same.",
	},
	{
		match:    siteServerError,
		class:    "site_server_error",
		sentence: "the website answered with a server error",
		remedy:   "Theirs to fix. The read is retried later.",
	},
	{
		match:    siteAnswerNotRetried,
		class:    "site_page_unreadable",
		sentence: "the website would not serve that page",
		remedy:   "The site moved, removed or closed it. The company's domain may need correcting; retrying does not help.",
	},
}

// isInterrupted answers a job whose work context was cancelled under it — what
// a worker's stop does to a job still running when its drain window ends. A
// deadline is not this: a job that outran its own timeout is isTimeout's.
func isInterrupted(err error) bool {
	return errors.Is(err, context.Canceled)
}

// isDNSFailure answers the one shape that never fixes itself on retry.
func isDNSFailure(err error) bool {
	var dns *net.DNSError
	return errors.As(err, &dns)
}

// isTLSFailure covers both ends of a handshake that did not complete: the
// record the peer sent was not TLS at all, or the certificate was refused.
func isTLSFailure(err error) bool {
	var record tls.RecordHeaderError
	var verify *tls.CertificateVerificationError
	var alert tls.AlertError
	return errors.As(err, &record) || errors.As(err, &verify) || errors.As(err, &alert)
}

// isConnectionRefused reads the syscall underneath, not the message.
func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

// isTimeout takes the three spellings one timeout arrives in: the deadline the
// caller set, the one the transport set, and net's own Timeout predicate.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var timeout net.Error
	return errors.As(err, &timeout) && timeout.Timeout()
}

// providerStatusIn matches a typed provider failure carrying one of these
// statuses.
//
// The STATUS is read and the sentence is authored, which is the distinction the
// whole file rests on: a bare status integer carries no record data, and
// enumerating the ones that mean something keeps every published sentence a
// member of the closed set rather than a number formatted into prose.
func providerStatusIn(statuses ...int) func(error) bool {
	return func(err error) bool {
		var provider *connector.ProviderError
		if !errors.As(err, &provider) {
			return false
		}
		for _, s := range statuses {
			if provider.Status == s {
				return true
			}
		}
		return false
	}
}

// providerServerError matches any 5xx. Enumerated as a RANGE rather than as
// statuses because they mean one thing to an operator — the provider broke —
// and one sentence for them is the honest granularity; splitting 502 from 503
// would publish a distinction nobody acts on differently.
func providerServerError(err error) bool {
	var provider *connector.ProviderError
	return errors.As(err, &provider) && provider.Status >= 500 && provider.Status <= 599
}

// siteAnswer is a website's refusal of a read, seen by shape. webread's own
// StatusError satisfies it, and its Retryable is the decision the crawl's stop
// reason also follows, so the three classes below say what the site-read
// record says.
type siteAnswer interface {
	error
	HTTPStatus() int
	Retryable() bool
}

// siteAnswerOf is a pointer-free read of the shape: the answer, or false.
//
//nolint:ireturn // the interface is the point: jobs matches the shape without importing the crawler
func siteAnswerOf(err error) (siteAnswer, bool) {
	var site siteAnswer
	return site, errors.As(err, &site)
}

// siteRefused is a retryable answer below the server errors: bot protection or
// a throttle on the site's side.
func siteRefused(err error) bool {
	site, ok := siteAnswerOf(err)
	return ok && site.Retryable() && site.HTTPStatus() < 500
}

// siteServerError is any 5xx a website answered.
func siteServerError(err error) bool {
	site, ok := siteAnswerOf(err)
	return ok && site.HTTPStatus() >= 500 && site.HTTPStatus() <= 599
}

// siteAnswerNotRetried matches the answers the crawl does not retry.
func siteAnswerNotRetried(err error) bool {
	site, ok := siteAnswerOf(err)
	return ok && !site.Retryable()
}

// technicalFaultFor answers the authored classification for a cause's shape.
func technicalFaultFor(err error) (technicalFault, bool) {
	for _, f := range technicalFaults {
		if f.match(err) {
			return f, true
		}
	}
	return technicalFault{}, false
}
