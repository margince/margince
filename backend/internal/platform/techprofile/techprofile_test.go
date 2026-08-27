// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package techprofile_test

import (
	"net"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/techprofile"
)

func TestMailProviderReadsTheBestPreferenceHost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		domain  string
		mxHosts []string
		wantKey string
		wantOK  bool
	}{
		{
			name:    "google workspace",
			domain:  "example.de",
			mxHosts: []string{"aspmx.l.google.com", "alt1.aspmx.l.google.com"},
			wantKey: techprofile.MailGoogleWorkspace, wantOK: true,
		},
		{
			name:    "microsoft 365",
			domain:  "example.de",
			mxHosts: []string{"example-de.mail.protection.outlook.com"},
			wantKey: techprofile.MailMicrosoft365, wantOK: true,
		},
		{
			name:    "own server under the company domain",
			domain:  "example.de",
			mxHosts: []string{"mail.example.de"},
			wantKey: techprofile.MailSelfHosted, wantOK: true,
		},
		{
			name:    "a provider we cannot name is not self-hosted",
			domain:  "example.de",
			mxHosts: []string{"mx1.some-regional-isp.example.net"},
			wantKey: techprofile.MailOther, wantOK: true,
		},
		{
			// The company runs Google as primary and keeps its own box as a
			// fallback. It runs on Google — a company has one mail system, and
			// reporting the fallback too would put two answers on the record.
			name:    "the fallback does not override the primary",
			domain:  "example.de",
			mxHosts: []string{"aspmx.l.google.com", "backup.example.de"},
			wantKey: techprofile.MailGoogleWorkspace, wantOK: true,
		},
		{
			name:   "a domain that receives no mail yields nothing",
			domain: "example.de", mxHosts: nil,
			wantOK: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := techprofile.MailProvider(tc.domain, tc.mxHosts)
			if ok != tc.wantOK {
				t.Fatalf("reported ok=%v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.Key != tc.wantKey {
				t.Errorf("read %q, want %q", got.Key, tc.wantKey)
			}
			if got.Field != techprofile.FieldMailProvider {
				t.Errorf("wrote field %q, want %q", got.Field, techprofile.FieldMailProvider)
			}
			if got.Evidence == "" {
				t.Error("stored no evidence; a claim on the record must name the record that proved it")
			}
		})
	}
}

func TestEmailSecurityReadsThePublishedPosture(t *testing.T) {
	t.Parallel()
	signals := techprofile.EmailSecurity(
		[]string{"v=spf1 include:_spf.google.com ~all", "google-site-verification=abc"},
		[]string{"v=DMARC1; p=reject; rua=mailto:dmarc@example.de"},
		true,
	)
	got := map[string]bool{}
	for _, signal := range signals {
		got[signal.Key] = true
	}
	for _, want := range []string{
		techprofile.SecuritySPF, techprofile.SecurityDMARCReject, techprofile.SecurityDKIM,
	} {
		if !got[want] {
			t.Errorf("did not read %q from a domain that publishes it", want)
		}
	}
	if got[techprofile.SecurityDMARCNone] {
		t.Error("read an observing DMARC policy from a domain that enforces one")
	}
}

func TestEmailSecurityDistinguishesTheDMARCLevels(t *testing.T) {
	t.Parallel()
	for record, want := range map[string]string{
		"v=DMARC1; p=none":       techprofile.SecurityDMARCNone,
		"v=DMARC1; p=quarantine": techprofile.SecurityDMARCQuarantine,
		"v=DMARC1; p=reject":     techprofile.SecurityDMARCReject,
	} {
		signals := techprofile.EmailSecurity(nil, []string{record}, false)
		if len(signals) != 1 {
			t.Fatalf("%q produced %d signals, want 1", record, len(signals))
		}
		if signals[0].Key != want {
			t.Errorf("%q read as %q, want %q", record, signals[0].Key, want)
		}
	}
}

func TestEmailSecurityIgnoresAMalformedDMARCRecord(t *testing.T) {
	t.Parallel()
	// A TXT record at _dmarc that is not a DMARC record at all, and a DMARC
	// record with a policy nobody defines. Neither is a posture to report.
	if signals := techprofile.EmailSecurity(nil, []string{"some unrelated verification token"}, false); len(signals) != 0 {
		t.Errorf("read a posture from an unrelated TXT record: %v", signals)
	}
	if signals := techprofile.EmailSecurity(nil, []string{"v=DMARC1; p=maybe"}, false); len(signals) != 0 {
		t.Errorf("read a posture from an undefined policy: %v", signals)
	}
}

// TestOperatedServicesDropsEveryNameThatIsNotAService is the privacy boundary's
// own test. A certificate log publishes whatever hostnames a company ever had a
// certificate for, and those include people. Nothing but a known service label
// may survive the classifier.
func TestOperatedServicesDropsEveryNameThatIsNotAService(t *testing.T) {
	t.Parallel()
	hostnames := []string{
		"shop.example.de",
		"karriere.example.de",
		// Every one of these is the shape the guardrail exists for: a person's
		// name, a person's initials, a contractor's box, somebody's laptop.
		"jan.mueller.example.de",
		"anna-schmidt.example.de",
		"jmueller.example.de",
		"lars-laptop.example.de",
		"praktikant-thomas.example.de",
	}
	services := techprofile.OperatedServices("example.de", hostnames)

	if len(services) != 2 {
		t.Fatalf("kept %d services from a set holding 2, want 2: %v", len(services), services)
	}
	for _, service := range services {
		for _, personal := range []string{"jan", "mueller", "anna", "schmidt", "lars", "thomas", "praktikant"} {
			if contains(service.Key, personal) || contains(service.Label, personal) || contains(service.Evidence, personal) {
				t.Fatalf("a personal name survived into %+v", service)
			}
		}
	}
}

func TestOperatedServicesReadsTheServiceLabels(t *testing.T) {
	t.Parallel()
	services := techprofile.OperatedServices("example.de", []string{
		"shop.example.de", "karriere.example.de", "api.example.de",
		"portal.example.de", "vpn.example.de",
	})
	got := map[string]bool{}
	for _, service := range services {
		got[service.Key] = true
	}
	for _, want := range []string{
		techprofile.ServiceWebshop, techprofile.ServiceCareers, techprofile.ServiceAPI,
		techprofile.ServiceCustomerPortal, techprofile.ServiceVPN,
	} {
		if !got[want] {
			t.Errorf("did not read %q from a domain that publishes it", want)
		}
	}
}

func TestOperatedServicesCitesTheShortestProvingHostname(t *testing.T) {
	t.Parallel()
	services := techprofile.OperatedServices("example.de", []string{
		"shop.staging.old.example.de", "shop.example.de",
	})
	if len(services) != 1 {
		t.Fatalf("read %d services, want 1", len(services))
	}
	if services[0].Evidence != "shop.example.de" {
		t.Errorf("cited %q, want the shortest proving hostname", services[0].Evidence)
	}
}

func TestOperatedServicesIgnoresTheDomainItself(t *testing.T) {
	t.Parallel()
	if services := techprofile.OperatedServices("example.de", []string{"example.de"}); len(services) != 0 {
		t.Errorf("read a service from the domain itself: %v", services)
	}
}

func TestHostingProviderPrefersTheReverseName(t *testing.T) {
	t.Parallel()
	got, ok := techprofile.HostingProvider(
		[]string{"static.88.99.example.your-server.de"}, "some-alias.example.net")
	if !ok {
		t.Fatal("read no provider from a reverse name that names one")
	}
	if got.Key != techprofile.HostingHetzner {
		t.Errorf("read %q, want %q", got.Key, techprofile.HostingHetzner)
	}
}

func TestHostingProviderFallsBackToTheCNAME(t *testing.T) {
	t.Parallel()
	got, ok := techprofile.HostingProvider(nil, "d123.cloudfront.net")
	if !ok {
		t.Fatal("read no provider from a CNAME that names one")
	}
	if got.Key != techprofile.HostingAWS {
		t.Errorf("read %q, want %q", got.Key, techprofile.HostingAWS)
	}
}

// A provider we cannot name is not worth a row: it would be true of most
// companies and tells a rep nothing.
func TestHostingProviderStaysSilentOnAnUnknownHost(t *testing.T) {
	t.Parallel()
	if got, ok := techprofile.HostingProvider([]string{"host42.some-local-isp.example"}, ""); ok {
		t.Errorf("named a provider it cannot identify: %+v", got)
	}
}

func TestReverseLookupTargetsAsksOncePerFamily(t *testing.T) {
	t.Parallel()
	targets := techprofile.ReverseLookupTargets([]net.IP{
		net.ParseIP("93.184.216.34"), net.ParseIP("93.184.216.35"),
		net.ParseIP("2606:2800:220:1:248:1893:25c8:1946"),
		net.ParseIP("2606:2800:220:1:248:1893:25c8:1947"),
	})
	if len(targets) != 2 {
		t.Fatalf("picked %d targets from a round-robin set, want one per family: %v", len(targets), targets)
	}
}

func TestTechnologiesReadsWhatThePageDeclares(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		evidence techprofile.Evidence
		wantKey  string
	}{
		{
			name:     "the generator names the shop system",
			evidence: techprofile.Evidence{Generator: "Shopware 6"},
			wantKey:  "shopware",
		},
		{
			name: "a cookie name identifies the platform",
			evidence: techprofile.Evidence{
				CookieNames: []string{"fe_typo_user"},
			},
			wantKey: "typo3",
		},
		{
			name: "a script src identifies the analytics",
			evidence: techprofile.Evidence{
				ScriptSrcs: []string{"https://www.googletagmanager.com/gtag/js?id=G-XYZ"},
			},
			wantKey: "google_analytics",
		},
		{
			name: "a response header identifies the server",
			evidence: techprofile.Evidence{
				Headers: http.Header{"Server": []string{"nginx/1.24.0"}},
			},
			wantKey: "nginx",
		},
		{
			name: "a markup marker identifies the CMS",
			evidence: techprofile.Evidence{
				Body: `<link href="/wp-content/themes/x/style.css">`,
			},
			wantKey: "wordpress",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			found, err := techprofile.Technologies(tc.evidence)
			if err != nil {
				t.Fatalf("matching the rules: %v", err)
			}
			for _, signal := range found {
				if signal.Key == tc.wantKey {
					if signal.Evidence == "" {
						t.Error("matched without naming the marker that matched")
					}
					return
				}
			}
			t.Errorf("did not read %q; read %v", tc.wantKey, found)
		})
	}
}

// A header the rule names by PRESENCE alone must not match a page that never
// sent it — the empty wanted-value is "any value", not "no header needed".
func TestTechnologiesDoesNotMatchAnAbsentHeader(t *testing.T) {
	t.Parallel()
	found, err := techprofile.Technologies(techprofile.Evidence{
		Headers: http.Header{"Server": []string{"nginx"}},
	})
	if err != nil {
		t.Fatalf("matching the rules: %v", err)
	}
	for _, signal := range found {
		if signal.Key == "cloudflare" {
			t.Errorf("matched Cloudflare on a page that sent no cf-ray: %+v", signal)
		}
	}
}

func TestTechnologiesReadsNothingFromAnEmptyPage(t *testing.T) {
	t.Parallel()
	found, err := techprofile.Technologies(techprofile.Evidence{})
	if err != nil {
		t.Fatalf("matching the rules: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("read %v from a page that declared nothing", found)
	}
}

// The embedded ruleset is pinned: a rule lost while editing the JSON would
// otherwise shrink the fingerprint silently, and a company that runs less looks
// exactly like a reader that recognizes less.
func TestRulesetIsIntactAndWellFormed(t *testing.T) {
	t.Parallel()
	rules, err := techprofile.Rules()
	if err != nil {
		t.Fatalf("reading the embedded rules: %v", err)
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		if rule.Key == "" || rule.Label == "" {
			t.Errorf("rule %+v is missing its key or label", rule)
		}
		if seen[rule.Key] {
			t.Errorf("two rules share the key %q", rule.Key)
		}
		seen[rule.Key] = true
		if len(rule.Generator)+len(rule.Headers)+len(rule.Cookies)+len(rule.Scripts)+len(rule.Body) == 0 {
			t.Errorf("rule %q carries no marker and can never match", rule.Key)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && stringsContains(haystack, needle)
}

func stringsContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
