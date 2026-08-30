// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/dnsread"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// observedAt is a fixed instant: the engine stamps what it read, and a test
// that used the real clock would be asserting against a moving target.
var observedAt = time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)

type stubResolver struct {
	mx      []dnsread.MXHost
	txt     map[string][]string
	addrs   []net.IP
	cname   string
	names   []string
	failMX  error
	failTXT error
}

func (s stubResolver) MX(context.Context, string) ([]dnsread.MXHost, bool, error) {
	if s.failMX != nil {
		return nil, false, s.failMX
	}
	return s.mx, len(s.mx) > 0, nil
}

func (s stubResolver) TXT(_ context.Context, name string) ([]string, bool, error) {
	if s.failTXT != nil {
		return nil, false, s.failTXT
	}
	records := s.txt[name]
	return records, len(records) > 0, nil
}

func (s stubResolver) Addresses(context.Context, string) ([]net.IP, bool, error) {
	return s.addrs, len(s.addrs) > 0, nil
}

func (s stubResolver) CNAME(context.Context, string) (string, bool, error) {
	return s.cname, s.cname != "", nil
}

func (s stubResolver) Names(context.Context, net.IP) ([]string, bool, error) {
	return s.names, len(s.names) > 0, nil
}

type stubCertLog struct {
	hostnames []string
	err       error
}

func (s stubCertLog) Hostnames(context.Context, string) ([]string, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	return s.hostnames, len(s.hostnames) > 0, nil
}

// recordingCache remembers what was written, so a test can assert on what the
// cache was asked to hold — which is where the privacy boundary is checked.
type recordingCache struct {
	stored map[string][]string
}

func newRecordingCache() *recordingCache {
	return &recordingCache{stored: map[string][]string{}}
}

func (c *recordingCache) LookupTechnical(_ context.Context, query, kind string) (people.CachedLookup, bool, error) {
	answer, hit := c.stored[kind+"|"+query]
	return people.CachedLookup{Answer: answer, Found: len(answer) > 0}, hit, nil
}

func (c *recordingCache) RememberTechnical(_ context.Context, query, kind string, answer people.CachedLookup) error {
	c.stored[kind+"|"+query] = answer.Answer
	return nil
}

func (c *recordingCache) everythingStored() string {
	var all []string
	for key, values := range c.stored {
		all = append(all, key)
		all = append(all, values...)
	}
	return strings.Join(all, " ")
}

func fixedClock() func() time.Time { return func() time.Time { return observedAt } }

func TestReadGathersEveryLane(t *testing.T) {
	t.Parallel()
	enricher := NewTechnicalEnricher(
		stubResolver{
			mx:    []dnsread.MXHost{{Host: "example-de.mail.protection.outlook.com", Preference: 10}},
			txt:   map[string][]string{"example.de": {"v=spf1 -all"}, "_dmarc.example.de": {"v=DMARC1; p=reject"}},
			addrs: []net.IP{net.ParseIP("1.2.3.4")},
			names: []string{"static.1.2.3.4.your-server.de"},
		},
		stubCertLog{hostnames: []string{"shop.example.de", "karriere.example.de"}},
		newRecordingCache(), fixedClock(),
	)

	got, outcomes := enricher.Read(context.Background(), ids.OrganizationID{}, "example.de")

	// TWO, not three: the homepage lane belongs to the site read now, which
	// matches every page it crawled rather than fetching one here.
	if len(got.Completed) != 2 {
		t.Fatalf("completed %d lanes, want 2: %v", len(got.Completed), outcomes)
	}
	if !got.ObservedAt.Equal(observedAt) {
		t.Errorf("stamped %s, want the injected clock's %s", got.ObservedAt, observedAt)
	}
	for _, want := range []struct{ field, key string }{
		{people.FactMailProvider, "microsoft365"},
		{people.FactEmailSecurity, "dmarc_reject"},
		{people.FactHostingProvider, "hetzner"},
		{people.FactOperatedService, "webshop"},
		{people.FactOperatedService, "careers"},
	} {
		if !observed(got, want.field, want.key) {
			t.Errorf("did not read %s=%s; read %v", want.field, want.key, got.Observations)
		}
	}
	for _, observation := range got.Observations {
		if observation.Evidence == "" || observation.SourceURL == "" {
			t.Errorf("%+v names nothing that proves it; the DDL refuses such a row", observation)
		}
	}
}

// A certificate log being down is the common case, and recording it as "this
// company operates no services" would delete a webshop signal a rep was about
// to act on. The lane must simply not complete.
func TestACertificateLogOutageCompletesNoLane(t *testing.T) {
	t.Parallel()
	enricher := NewTechnicalEnricher(
		stubResolver{mx: []dnsread.MXHost{{Host: "aspmx.l.google.com"}}},
		stubCertLog{err: errors.New("crt.sh is having a day")},
		newRecordingCache(), fixedClock(),
	)

	got, _ := enricher.Read(context.Background(), ids.OrganizationID{}, "example.de")

	for _, lane := range got.Completed {
		if lane == people.LaneCertLog {
			t.Fatal("the certificate lane completed on a query that failed; its rows would be wiped")
		}
	}
	// One survivor, because this engine now runs two lanes: DNS answered and
	// the certificate log did not.
	if len(got.Completed) != 1 {
		t.Errorf("one lane failing cost the other: completed %v", got.Completed)
	}
}

// A lane with no reader wired is a declared absence, not an empty answer: the
// record must keep whatever that lane last wrote.
func TestAnAbsentReaderCompletesNoLane(t *testing.T) {
	t.Parallel()
	enricher := NewTechnicalEnricher(nil, nil, newRecordingCache(), fixedClock())

	got, _ := enricher.Read(context.Background(), ids.OrganizationID{}, "example.de")

	if len(got.Completed) != 0 {
		t.Errorf("completed %v with nothing wired", got.Completed)
	}
}

func TestAnEmptyDomainAsksNobody(t *testing.T) {
	t.Parallel()
	enricher := NewTechnicalEnricher(
		stubResolver{mx: []dnsread.MXHost{{Host: "aspmx.l.google.com"}}},
		stubCertLog{hostnames: []string{"shop.example.de"}},
		newRecordingCache(), fixedClock(),
	)

	got, outcomes := enricher.Read(context.Background(), ids.OrganizationID{}, "   ")

	if len(got.Completed) != 0 || len(outcomes) != 0 {
		t.Errorf("looked something up for a record carrying no domain: %v", got)
	}
}

// The allowlist protects the fact rows, and it must protect the CACHE too — a
// table holding raw certificate hostnames would hold personal names in a place
// the erasure path does not reach.
func TestTheCacheNeverHoldsARawCertificateHostname(t *testing.T) {
	t.Parallel()
	cache := newRecordingCache()
	enricher := NewTechnicalEnricher(
		stubResolver{}, stubCertLog{hostnames: []string{
			"shop.example.de",
			"jan-mueller.example.de",
			"anna.schmidt.example.de",
		}},
		cache, fixedClock(),
	)

	enricher.Read(context.Background(), ids.OrganizationID{}, "example.de")

	stored := cache.everythingStored()
	for _, personal := range []string{"jan", "mueller", "anna", "schmidt"} {
		if strings.Contains(stored, personal) {
			t.Fatalf("a personal name reached the cache: %q contains %q", stored, personal)
		}
	}
	if !strings.Contains(stored, "webshop") {
		t.Errorf("the classified service did not reach the cache: %q", stored)
	}
}

func TestASecondReadIsAnsweredFromTheCache(t *testing.T) {
	t.Parallel()
	cache := newRecordingCache()
	counting := &countingCertLog{inner: stubCertLog{hostnames: []string{"shop.example.de"}}}
	enricher := NewTechnicalEnricher(stubResolver{}, counting, cache, fixedClock())

	enricher.Read(context.Background(), ids.OrganizationID{}, "example.de")
	second, _ := enricher.Read(context.Background(), ids.OrganizationID{}, "example.de")

	if counting.calls != 1 {
		t.Errorf("asked the certificate log %d times for one domain, want 1", counting.calls)
	}
	if !observed(second, people.FactOperatedService, "webshop") {
		t.Error("the cached read lost the service the first read found")
	}
}

type countingCertLog struct {
	inner stubCertLog
	calls int
}

func (c *countingCertLog) Hostnames(ctx context.Context, domain string) ([]string, bool, error) {
	c.calls++
	return c.inner.Hostnames(ctx, domain)
}

func observed(in people.TechnicalEnrichment, field, key string) bool {
	for _, observation := range in.Observations {
		if observation.Field == field && observation.ValueKey == key {
			return true
		}
	}
	return false
}
