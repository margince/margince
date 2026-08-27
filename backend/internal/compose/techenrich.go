// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading what a company publicly runs, from the three sources that answer
// without anybody's permission: its DNS records, its certificate history, and
// its own homepage.
//
// The lanes are INDEPENDENT on purpose. Each one completes or fails on its own,
// and only a completed lane is authoritative for the fields it owns. That is
// what keeps a certificate log outage — crt.sh is down often — from being
// recorded as "this company operates no services" and wiping a webshop signal a
// rep was about to act on.
//
// Every lookup is cache-first, and the cache is installation-global: a domain's
// DNS answer is the same for every tenant holding that domain, and asking a
// free public service once per tenant is how an installation gets blocked.

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/certlog"
	"github.com/margince/margince/backend/internal/platform/dnsread"
	"github.com/margince/margince/backend/internal/platform/techprofile"
	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dkimSelectors are the selector names probed for DKIM.
//
// Bounded deliberately: a selector is arbitrary text and there is no way to
// enumerate the ones a domain uses, so an exhaustive probe does not exist —
// only a longer guess. These three are what Google Workspace and Microsoft 365
// publish by default, which covers most of the market. A company self-hosting
// DKIM under its own selector reads as "no DKIM observed", which is honest:
// the product never claims DKIM is absent, only that it saw none.
var dkimSelectors = []string{"google._domainkey", "selector1._domainkey", "selector2._domainkey"}

// technicalReader is the lookup surface the engine needs. It is an interface so
// the engine is testable without a resolver, a log or a network.
type technicalReader interface {
	MX(ctx context.Context, domain string) ([]dnsread.MXHost, bool, error)
	TXT(ctx context.Context, name string) ([]string, bool, error)
	Addresses(ctx context.Context, host string) ([]net.IP, bool, error)
	CNAME(ctx context.Context, host string) (string, bool, error)
	Names(ctx context.Context, addr net.IP) ([]string, bool, error)
}

// certHostnameReader reads the hostnames a domain has certificates for.
type certHostnameReader interface {
	Hostnames(ctx context.Context, domain string) ([]string, bool, error)
}

// homepageReader reads one page's own declarations about its stack.
type homepageReader interface {
	FetchFingerprint(ctx context.Context, rawURL string) (webread.Fingerprint, error)
}

// technicalLookupCache is the remembered-answer surface. The people store
// implements it; the engine holds the narrow shape so a test can substitute a
// cache that forgets everything.
type technicalLookupCache interface {
	LookupTechnical(ctx context.Context, query, kind string) (people.CachedLookup, bool, error)
	RememberTechnical(ctx context.Context, query, kind string, answer people.CachedLookup) error
}

// TechnicalEnricher reads a company's public technical profile.
type TechnicalEnricher struct {
	dns   technicalReader
	certs certHostnameReader
	pages homepageReader
	cache technicalLookupCache
	now   func() time.Time
}

// NewTechnicalEnricher wires the engine. A nil reader is a DECLARED absence:
// the lane it serves reports as not completed, so the record keeps whatever
// that lane last wrote rather than being cleared by a deployment that simply
// does not run it.
func NewTechnicalEnricher(
	dns technicalReader, certs certHostnameReader, pages homepageReader,
	cache technicalLookupCache, now func() time.Time,
) *TechnicalEnricher {
	if now == nil {
		now = time.Now
	}
	return &TechnicalEnricher{dns: dns, certs: certs, pages: pages, cache: cache, now: now}
}

// laneOutcome is what one lane did, for the attempt ledger and the log.
type laneOutcome struct {
	Lane people.TechnicalLane
	// Completed says the source answered. Only a completed lane reconciles.
	Completed bool
	// Refused marks a source that declined rather than failed — a site's
	// robots.txt saying no. It COMPLETES the lane, because a refusal is a
	// settled answer and the claims it would have supported must be cleared,
	// but it earns the long backoff rather than the short one: asking again
	// next week is polite, asking again in six hours is not.
	Refused bool
	// Err is why it did not, kept for the ledger rather than to fail the run:
	// one lane failing must not cost the other two.
	Err error
}

// Read gathers what the domain publicly says about itself.
//
// It never fails as a whole. A domain whose every lane failed comes back with
// no completed lanes, which the writer reads as "change nothing" — the honest
// outcome, and one the next pass simply retries.
func (e *TechnicalEnricher) Read(
	ctx context.Context, orgID ids.OrganizationID, domain string,
) (people.TechnicalEnrichment, []laneOutcome) {
	domain = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(domain, ".")))
	result := people.TechnicalEnrichment{
		OrganizationID: orgID,
		ObservedAt:     e.now().UTC(),
	}
	if domain == "" {
		return result, nil
	}
	outcomes := []laneOutcome{
		e.readDNS(ctx, domain, &result),
		e.readCertLog(ctx, domain, &result),
		e.readHomepage(ctx, domain, &result),
	}
	for _, outcome := range outcomes {
		if outcome.Completed {
			result.Completed = append(result.Completed, outcome.Lane)
		}
	}
	return result, outcomes
}

// readDNS reads the mail provider, the mail-authentication posture and the
// hosting hint. It completes when the resolver answered about the domain at
// all — a domain with no MX records still has an answer, and recording "this
// company receives no mail" is the point of asking.
func (e *TechnicalEnricher) readDNS(
	ctx context.Context, domain string, result *people.TechnicalEnrichment,
) laneOutcome {
	outcome := laneOutcome{Lane: people.LaneDNS}
	if e.dns == nil {
		return outcome
	}
	sourceURL := "dns:" + domain

	mxHosts, err := e.cachedStrings(ctx, domain, people.CacheKindMX, func() ([]string, bool, error) {
		hosts, found, err := e.dns.MX(ctx, domain)
		names := make([]string, 0, len(hosts))
		for _, host := range hosts {
			names = append(names, host.Host)
		}
		return names, found, err
	})
	if err != nil {
		return laneOutcome{Lane: people.LaneDNS, Err: err}
	}
	if provider, ok := techprofile.MailProvider(domain, mxHosts); ok {
		result.Observations = append(result.Observations, observationOf(provider, sourceURL))
	}

	// EVERY sub-lookup must answer for this lane to be authoritative. A lane
	// that completes on a partial read is worse than one that fails: the
	// reconciliation then deletes the mail posture and the hosting provider
	// because one TXT query timed out, and the next pass writes them back —
	// so the record flickers with the resolver's mood.
	if !e.appendEmailSecurity(ctx, domain, sourceURL, result) {
		return laneOutcome{Lane: people.LaneDNS, Err: errPartialDNS}
	}
	if !e.appendHosting(ctx, domain, sourceURL, result) {
		return laneOutcome{Lane: people.LaneDNS, Err: errPartialDNS}
	}

	outcome.Completed = true
	return outcome
}

// errPartialDNS marks a DNS read that answered about some of what it owns and
// not the rest — reported as a lane failure, because partial authority over a
// set of fields is not authority at all.
var errPartialDNS = errors.New("compose: the DNS lane could not read every record it is authoritative for")

// appendEmailSecurity reads SPF, DMARC and the bounded DKIM probe.
//
// The classifier runs BEFORE the cache, so what is remembered is the posture
// — `spf`, `dmarc_reject`, `dkim` — and never the records themselves. A DMARC
// record carries `rua=mailto:someone@example.de` as a matter of course, and a
// cache holding that would put a person's address in an installation-global
// table the erasure path does not reach.
//
// It returns whether the posture is authoritative. A lookup that did not
// complete leaves it false, and the DNS lane then does not claim these fields
// — the alternative is deleting a company's whole mail posture because one TXT
// query timed out.
func (e *TechnicalEnricher) appendEmailSecurity(
	ctx context.Context, domain, sourceURL string, result *people.TechnicalEnrichment,
) bool {
	cached, hit, err := e.cache.LookupTechnical(ctx, domain, people.CacheKindTXT)
	if err != nil {
		return false
	}
	var keys []string
	if hit {
		keys = cached.Answer
	} else {
		keys, err = e.readEmailSecurity(ctx, domain)
		if err != nil {
			return false
		}
		if err := e.cache.RememberTechnical(ctx, domain, people.CacheKindTXT, people.CachedLookup{
			Answer: keys, Found: len(keys) > 0,
		}); err != nil {
			return false
		}
	}
	for _, signal := range techprofile.EmailSecurityFromKeys(keys) {
		result.Observations = append(result.Observations, observationOf(signal, sourceURL))
	}
	return true
}

// readEmailSecurity asks the resolver and classifies in one step, so no raw
// record is returned to a caller that could store it.
func (e *TechnicalEnricher) readEmailSecurity(ctx context.Context, domain string) ([]string, error) {
	rootTXT, _, err := e.dns.TXT(ctx, domain)
	if err != nil {
		return nil, err
	}
	dmarcTXT, _, err := e.dns.TXT(ctx, "_dmarc."+domain)
	if err != nil {
		return nil, err
	}
	dkim := false
	for _, selector := range dkimSelectors {
		records, found, err := e.dns.TXT(ctx, selector+"."+domain)
		if err != nil {
			return nil, err
		}
		if found && len(records) > 0 {
			dkim = true
			break
		}
	}
	keys := make([]string, 0, 3)
	for _, signal := range techprofile.EmailSecurity(rootTXT, dmarcTXT, dkim) {
		keys = append(keys, signal.Key)
	}
	return keys, nil
}

// appendHosting reads where the domain resolves and who signs that address
// space, preferring the reverse name and falling back to the CNAME target.
//
// Reports whether the read was authoritative, for the reason readDNS states: a
// hosting provider deleted because one address lookup failed is a fact removed
// by a network blip rather than by anything the company did.
func (e *TechnicalEnricher) appendHosting(
	ctx context.Context, domain, sourceURL string, result *people.TechnicalEnrichment,
) bool {
	addrText, err := e.cachedStrings(ctx, domain, people.CacheKindAddress, func() ([]string, bool, error) {
		addrs, found, err := e.dns.Addresses(ctx, domain)
		text := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			text = append(text, addr.String())
		}
		return text, found, err
	})
	if err != nil {
		return false
	}
	var addrs []net.IP
	for _, text := range addrText {
		if addr := net.ParseIP(text); addr != nil {
			addrs = append(addrs, addr)
		}
	}
	var reverseNames []string
	for _, addr := range techprofile.ReverseLookupTargets(addrs) {
		names, err := e.cachedStrings(ctx, addr.String(), people.CacheKindReverse, func() ([]string, bool, error) {
			return e.dns.Names(ctx, addr)
		})
		if err != nil {
			// A host with no PTR record answers ok=false, not an error, so an
			// error here really is a lookup that did not complete.
			return false
		}
		reverseNames = append(reverseNames, names...)
	}
	cname, err := e.cachedStrings(ctx, "www."+domain, people.CacheKindCNAME, func() ([]string, bool, error) {
		target, found, err := e.dns.CNAME(ctx, "www."+domain)
		if !found || target == "" {
			return nil, found, err
		}
		return []string{target}, true, err
	})
	if err != nil {
		return false
	}
	var cnameTarget string
	if len(cname) > 0 {
		cnameTarget = cname[0]
	}
	if hosting, ok := techprofile.HostingProvider(reverseNames, cnameTarget); ok {
		result.Observations = append(result.Observations, observationOf(hosting, sourceURL))
	}
	return true
}

// readCertLog reads the services the domain's certificate hostnames reveal.
//
// The CLASSIFIER runs before the cache write, never after it: a raw certificate
// hostname can carry a person's name, and a cache holding raw names would put
// personal data in a table the erasure path does not reach. What is remembered
// is the allowlisted service keys and their proving hostnames.
func (e *TechnicalEnricher) readCertLog(
	ctx context.Context, domain string, result *people.TechnicalEnrichment,
) laneOutcome {
	outcome := laneOutcome{Lane: people.LaneCertLog}
	if e.certs == nil {
		return outcome
	}
	cached, hit, err := e.cache.LookupTechnical(ctx, domain, people.CacheKindCertLog)
	if err != nil {
		return laneOutcome{Lane: people.LaneCertLog, Err: err}
	}
	var services []techprofile.Signal
	if hit {
		services = servicesFromCache(cached.Answer)
	} else {
		hostnames, found, err := e.certs.Hostnames(ctx, domain)
		if err != nil {
			// The log did not answer. NOT an empty result: recording this as
			// "no services" is the one wrong answer this lane must not give.
			return laneOutcome{Lane: people.LaneCertLog, Err: err}
		}
		services = techprofile.OperatedServices(domain, hostnames)
		if err := e.cache.RememberTechnical(ctx, domain, people.CacheKindCertLog, people.CachedLookup{
			Answer: servicesToCache(services), Found: found,
		}); err != nil {
			return laneOutcome{Lane: people.LaneCertLog, Err: err}
		}
	}
	sourceURL := certlog.PublicBaseURL + "/?q=%25." + domain
	for _, service := range services {
		result.Observations = append(result.Observations, observationOf(service, sourceURL))
	}
	outcome.Completed = true
	return outcome
}

// readHomepage reads what the company's own homepage declares about its stack.
func (e *TechnicalEnricher) readHomepage(
	ctx context.Context, domain string, result *people.TechnicalEnrichment,
) laneOutcome {
	outcome := laneOutcome{Lane: people.LaneHomepage}
	if e.pages == nil {
		return outcome
	}
	page, err := e.pages.FetchFingerprint(ctx, "https://"+domain)
	if err != nil {
		if errors.Is(err, webread.ErrRobotsDisallowed) {
			// The site said no. That is an ANSWER, and the lane completes with
			// nothing rather than retrying against a refusal — which also
			// clears any technology rows a previous read left, because we no
			// longer have permission to claim them.
			outcome.Completed = true
			outcome.Refused = true
			return outcome
		}
		return laneOutcome{Lane: people.LaneHomepage, Err: err}
	}
	found, err := techprofile.Technologies(techprofile.Evidence{
		Headers:     page.Headers,
		CookieNames: page.CookieNames,
		ScriptSrcs:  page.ScriptSrcs,
		Generator:   page.Generator,
		Body:        page.Body,
	})
	if err != nil {
		return laneOutcome{Lane: people.LaneHomepage, Err: err}
	}
	for _, technology := range found {
		result.Observations = append(result.Observations, observationOf(technology, page.URL))
	}
	outcome.Completed = true
	return outcome
}

// cachedStrings answers from the cache when it can and asks the source when it
// cannot, remembering whatever the source said.
func (e *TechnicalEnricher) cachedStrings(
	ctx context.Context, query, kind string, ask func() ([]string, bool, error),
) ([]string, error) {
	cached, hit, err := e.cache.LookupTechnical(ctx, query, kind)
	if err != nil {
		return nil, err
	}
	if hit {
		return cached.Answer, nil
	}
	answer, found, err := ask()
	if err != nil {
		return nil, err
	}
	if err := e.cache.RememberTechnical(ctx, query, kind, people.CachedLookup{
		Answer: answer, Found: found,
	}); err != nil {
		return nil, err
	}
	return answer, nil
}

// observationOf carries a classified signal into the shape the writer stores.
func observationOf(signal techprofile.Signal, sourceURL string) people.TechnicalObservation {
	return people.TechnicalObservation{
		Field:     signal.Field,
		ValueKey:  signal.Key,
		Value:     signal.Label,
		Evidence:  signal.Evidence,
		SourceURL: sourceURL,
	}
}

// servicesToCache and servicesFromCache carry classified services through the
// cache as `key|evidence` pairs.
//
// Only the allowlisted key and its proving hostname travel — the classifier has
// already run, so nothing a certificate said about a person is in here.
func servicesToCache(services []techprofile.Signal) []string {
	encoded := make([]string, 0, len(services))
	for _, service := range services {
		encoded = append(encoded, service.Key+"|"+service.Evidence)
	}
	return encoded
}

func servicesFromCache(encoded []string) []techprofile.Signal {
	services := make([]techprofile.Signal, 0, len(encoded))
	for _, entry := range encoded {
		key, evidence, found := strings.Cut(entry, "|")
		if !found {
			continue
		}
		// Re-derived rather than cached, so a label reworded in the code takes
		// effect on the next read instead of persisting in the cache.
		label, known := techprofile.ServiceLabel(key)
		if !known {
			continue
		}
		services = append(services, techprofile.Signal{
			Field: techprofile.FieldOperatedService, Key: key, Label: label, Evidence: evidence,
		})
	}
	return services
}

// ErrNoTechnicalDomain says the company record carries no domain to look up.
// It is a REFUSAL rather than a failure: the lookup reads only what the record
// already holds, and there is nothing here to ask about.
var ErrNoTechnicalDomain = errors.New("technical enrichment: this company record carries no domain")
