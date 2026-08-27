// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dnsread reads the public DNS records of a domain a tenant already
// recorded: which host receives its mail, what its TXT records say about its
// mail authentication, and where its name resolves.
//
// Three properties hold for every lookup:
//   - It answers about ONE name the caller supplies. This package performs no
//     discovery: it never walks a zone, never enumerates neighbours, and has no
//     entry point that takes anything but a single name.
//   - "No such record" is a FACT, not a failure. Every lookup returns
//     (value, ok, err): ok=false with a nil error means the resolver answered
//     authoritatively that the name has no such record — which is worth
//     recording, because it is the difference between a company that runs no
//     mail and a lookup that did not complete. err is reserved for the second.
//   - It is paced. A resolver is somebody else's infrastructure, and an
//     installation reading thousands of domains shares one budget.
//
// It owns resolution mechanics and nothing else — no vocabulary, no
// classification, no opinion about what an MX host implies. That stays with
// the callers.
package dnsread

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"time"
)

// lookupTimeout bounds one resolver round trip. DNS is fast or it is broken;
// a lookup still outstanding after this has told us what we need to know.
const lookupTimeout = 5 * time.Second

// MXHost is one mail exchanger a domain names: the host, and the preference
// that ranks it. Lower preference wins, which is why the ordering matters
// enough to travel rather than being flattened to a list of names.
type MXHost struct {
	Host       string
	Preference uint16
}

// Resolver reads public DNS records. The interface exists so a test can answer
// without a network and so the production resolver is injected rather than
// reached for — the same reason the geocode client is an interface.
type Resolver interface {
	// MX reports the domain's mail exchangers, best-preference first.
	MX(ctx context.Context, domain string) ([]MXHost, bool, error)
	// TXT reports the TXT records at a name. The name is passed whole, so a
	// caller reads `_dmarc.example.de` by asking for it.
	TXT(ctx context.Context, name string) ([]string, bool, error)
	// Addresses reports the A and AAAA addresses a host resolves to.
	Addresses(ctx context.Context, host string) ([]net.IP, bool, error)
	// CNAME reports the canonical name a host is an alias for, and false when
	// it is not an alias.
	CNAME(ctx context.Context, host string) (string, bool, error)
	// Names reports the reverse-lookup names for an address, which is where a
	// hosting provider usually signs its own infrastructure.
	Names(ctx context.Context, addr net.IP) ([]string, bool, error)
}

// Reader is the production resolver.
type Reader struct {
	resolver *net.Resolver
	pacer    *Pacer
}

// New builds a reader over the system resolver, paced at the given interval.
func New(pacer *Pacer) *Reader {
	return &Reader{resolver: &net.Resolver{}, pacer: pacer}
}

// MX reports the domain's mail exchangers, best-preference first.
//
// A domain with a single "." MX has explicitly declared that it receives no
// mail (RFC 7505), which reads here as ok=false: the domain answered, and the
// answer is that there is no mail host.
func (r *Reader) MX(ctx context.Context, domain string) ([]MXHost, bool, error) {
	records, err := lookup(ctx, r, func(ctx context.Context) ([]*net.MX, error) {
		return r.resolver.LookupMX(ctx, domain)
	})
	if err != nil {
		return nil, false, wrap("mail exchangers", domain, err)
	}
	hosts := make([]MXHost, 0, len(records))
	for _, record := range records {
		host := strings.TrimSuffix(strings.ToLower(record.Host), ".")
		if host == "" {
			continue
		}
		hosts = append(hosts, MXHost{Host: host, Preference: record.Pref})
	}
	if len(hosts) == 0 {
		return nil, false, nil
	}
	sort.SliceStable(hosts, func(i, j int) bool { return hosts[i].Preference < hosts[j].Preference })
	return hosts, true, nil
}

// TXT reports the TXT records at a name.
func (r *Reader) TXT(ctx context.Context, name string) ([]string, bool, error) {
	records, err := lookup(ctx, r, func(ctx context.Context) ([]string, error) {
		return r.resolver.LookupTXT(ctx, name)
	})
	if err != nil {
		return nil, false, wrap("TXT records", name, err)
	}
	if len(records) == 0 {
		return nil, false, nil
	}
	return records, true, nil
}

// Addresses reports the A and AAAA addresses a host resolves to.
func (r *Reader) Addresses(ctx context.Context, host string) ([]net.IP, bool, error) {
	addrs, err := lookup(ctx, r, func(ctx context.Context) ([]net.IP, error) {
		return r.resolver.LookupIP(ctx, "ip", host)
	})
	if err != nil {
		return nil, false, wrap("addresses", host, err)
	}
	if len(addrs) == 0 {
		return nil, false, nil
	}
	return addrs, true, nil
}

// CNAME reports the canonical name a host is an alias for.
//
// The standard library answers a non-alias by echoing the queried name back,
// which is not an alias and reads here as ok=false.
func (r *Reader) CNAME(ctx context.Context, host string) (string, bool, error) {
	canonical, err := lookup(ctx, r, func(ctx context.Context) (string, error) {
		return r.resolver.LookupCNAME(ctx, host)
	})
	if err != nil {
		return "", false, wrap("canonical name", host, err)
	}
	canonical = strings.TrimSuffix(strings.ToLower(canonical), ".")
	if canonical == "" || canonical == strings.TrimSuffix(strings.ToLower(host), ".") {
		return "", false, nil
	}
	return canonical, true, nil
}

// Names reports the reverse-lookup names for an address.
func (r *Reader) Names(ctx context.Context, addr net.IP) ([]string, bool, error) {
	names, err := lookup(ctx, r, func(ctx context.Context) ([]string, error) {
		return r.resolver.LookupAddr(ctx, addr.String())
	})
	if err != nil {
		return nil, false, wrap("reverse names", addr.String(), err)
	}
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSuffix(strings.ToLower(name), "."); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return nil, false, nil
	}
	return cleaned, true, nil
}

// lookup runs one paced, timeout-bounded resolver call and folds the "this
// name has no such record" answer into a zero value with a nil error, so every
// entry point above reports absence the same way.
func lookup[T any](ctx context.Context, r *Reader, ask func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := r.pacer.Wait(ctx); err != nil {
		return zero, err
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	value, err := ask(ctx)
	if err != nil {
		if notFound(err) {
			return zero, nil
		}
		return zero, err
	}
	return value, nil
}

// notFound reports whether the resolver's error means "this name has no such
// record" rather than "the lookup did not complete". NXDOMAIN and an empty
// answer both arrive as *net.DNSError with IsNotFound set.
func notFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// wrap names the record kind and the name asked about, so a failed lookup says
// what it was reading rather than only that DNS failed.
func wrap(what, name string, err error) error {
	if err == nil {
		return nil
	}
	return &LookupError{What: what, Name: name, Err: err}
}

// LookupError is a lookup that did not complete. It carries the name and the
// record kind because a caller reading four record types for one domain
// otherwise cannot tell which of them failed.
type LookupError struct {
	What string
	Name string
	Err  error
}

func (e *LookupError) Error() string {
	return "dnsread: reading " + e.What + " for " + e.Name + ": " + e.Err.Error()
}

func (e *LookupError) Unwrap() error { return e.Err }

// Temporary reports whether the resolver called this failure a transient one,
// which is what decides whether a later pass should ask again.
func (e *LookupError) Temporary() bool {
	var dnsErr *net.DNSError
	return errors.As(e.Err, &dnsErr) && dnsErr.IsTemporary
}
