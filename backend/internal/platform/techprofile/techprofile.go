// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package techprofile turns raw public observations about a domain — its DNS
// records, the hostnames in its certificates, and what its homepage declares —
// into the vocabulary the company record stores.
//
// It is pure: no network, no database, no clock. The lanes that gather the
// observations live in platform/dnsread, platform/certlog and
// platform/webread; the writing lives in the people module. This package is
// the rules between them, which is what makes every rule table-testable
// without a fixture.
//
// THE ALLOWLIST IS THE PRIVACY BOUNDARY. A certificate or a DNS record can
// carry a person's name — an admin's, a developer's, whoever asked for the
// certificate. Nothing here passes a name through: a hostname becomes a signal
// only by matching a known service label, and everything else is dropped
// rather than stored. That is why the whole feature stays outside personal-data
// scope, and it is a property of this file rather than of a caller's care.
package techprofile

import "strings"

// Signal is one observation ready to be written: which field it belongs to,
// the stable key that filters and comparisons use, the label a person reads,
// and the public record that proves it.
type Signal struct {
	Field string
	Key   string
	Label string
	// Evidence is the public record this was read from — the MX host, the
	// certificate hostname, the matched marker. It is what an audit answer
	// shows when someone asks how the product knows.
	Evidence string
}

// The fields a technical profile writes. They are the `field` half of the
// organization_fact vocabulary, under category `signal`.
const (
	FieldMailProvider    = "mail_provider"
	FieldEmailSecurity   = "email_security"
	FieldHostingProvider = "hosting_provider"
	FieldOperatedService = "operated_service"
	// FieldTechnology already exists in the fact vocabulary — the homepage
	// fingerprint writes the same field a site read does, because "this
	// company runs Shopware" is one claim however it was observed.
	FieldTechnology = "technology"
)

// Mail providers. `other` names a provider we did not recognize, which is a
// different fact from `self_hosted` and worth keeping apart: one says the
// company outsources mail to somebody we cannot name, the other says it runs
// its own.
const (
	MailGoogleWorkspace = "google_workspace"
	MailMicrosoft365    = "microsoft365"
	MailSelfHosted      = "self_hosted"
	MailOther           = "other"
)

// The mail providers a reader sees. Named because each operates several MX
// suffixes and the label must read the same on all of them.
const (
	labelMicrosoft365    = "Microsoft 365"
	labelGoogleWorkspace = "Google Workspace"
)

// mailProviderSuffixes maps an MX host suffix to the provider that operates
// it. Matched against the best-preference host, longest suffix first.
var mailProviderSuffixes = []struct {
	suffix string
	key    string
	label  string
}{
	{".mail.protection.outlook.com", MailMicrosoft365, labelMicrosoft365},
	{".olc.protection.outlook.com", MailMicrosoft365, labelMicrosoft365},
	{".mail.eo.outlook.com", MailMicrosoft365, labelMicrosoft365},
	{".google.com", MailGoogleWorkspace, labelGoogleWorkspace},
	{".googlemail.com", MailGoogleWorkspace, labelGoogleWorkspace},
	{".mailhost.i.gmail.com", MailGoogleWorkspace, labelGoogleWorkspace},
}

// MailProvider reads the mail provider from a domain's MX hosts.
//
// The best-preference host decides. A domain listing Google as primary and its
// own server as a fallback runs on Google, and the fallback is not a second
// answer to report — a company has one mail system.
//
// A host under the company's own domain is self-hosted. That check runs LAST,
// because a provider's host can sit under the customer's domain by delegation
// and the named providers above are the stronger evidence.
func MailProvider(domain string, mxHosts []string) (Signal, bool) {
	for _, host := range mxHosts {
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
		if host == "" {
			continue
		}
		for _, candidate := range mailProviderSuffixes {
			if strings.HasSuffix(host, candidate.suffix) {
				return Signal{
					Field: FieldMailProvider, Key: candidate.key,
					Label: candidate.label, Evidence: host,
				}, true
			}
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return Signal{
				Field: FieldMailProvider, Key: MailSelfHosted,
				Label: "Eigener Mailserver", Evidence: host,
			}, true
		}
		return Signal{
			Field: FieldMailProvider, Key: MailOther,
			Label: "Anderer Anbieter", Evidence: host,
		}, true
	}
	return Signal{}, false
}

// Email-security postures. The DMARC policy is carried as its own key per
// level because "publishes DMARC" and "enforces DMARC" are different facts
// about how seriously a company takes its own mail.
const (
	SecuritySPF             = "spf"
	SecurityDMARCNone       = "dmarc_none"
	SecurityDMARCQuarantine = "dmarc_quarantine"
	SecurityDMARCReject     = "dmarc_reject"
	SecurityDKIM            = "dkim"
)

// EmailSecurity reads the mail-authentication posture from a domain's TXT
// records: the root records for SPF, the _dmarc records for DMARC policy, and
// whether any well-known DKIM selector answered.
//
// The evidence is the record itself, truncated: an SPF record names the hosts
// a company sends from, which is a technical fact about the company and the
// proof a reader would want to see.
func EmailSecurity(rootTXT, dmarcTXT []string, dkimFound bool) []Signal {
	var signals []Signal
	for _, record := range rootTXT {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(record)), "v=spf1") {
			signals = append(signals, Signal{
				Field: FieldEmailSecurity, Key: SecuritySPF,
				Label: "SPF veröffentlicht", Evidence: truncate(record),
			})
			break
		}
	}
	if policy, record, ok := dmarcPolicy(dmarcTXT); ok {
		signals = append(signals, Signal{
			Field: FieldEmailSecurity, Key: policy.key,
			Label: policy.label, Evidence: truncate(record),
		})
	}
	if dkimFound {
		signals = append(signals, Signal{
			Field: FieldEmailSecurity, Key: SecurityDKIM,
			Label: "DKIM eingerichtet", Evidence: "DKIM-Selector beantwortet",
		})
	}
	return signals
}

type dmarcLevel struct{ key, label string }

// dmarcPolicy reads the p= tag out of a _dmarc TXT record.
func dmarcPolicy(dmarcTXT []string) (dmarcLevel, string, bool) {
	levels := map[string]dmarcLevel{
		"none":       {SecurityDMARCNone, "DMARC nur beobachtend"},
		"quarantine": {SecurityDMARCQuarantine, "DMARC in Quarantäne"},
		"reject":     {SecurityDMARCReject, "DMARC durchgesetzt"},
	}
	for _, record := range dmarcTXT {
		trimmed := strings.TrimSpace(record)
		if !strings.HasPrefix(strings.ToLower(trimmed), "v=dmarc1") {
			continue
		}
		for _, tag := range strings.Split(trimmed, ";") {
			name, value, found := strings.Cut(strings.TrimSpace(tag), "=")
			if !found || !strings.EqualFold(strings.TrimSpace(name), "p") {
				continue
			}
			if level, known := levels[strings.ToLower(strings.TrimSpace(value))]; known {
				return level, trimmed, true
			}
		}
	}
	return dmarcLevel{}, "", false
}

// evidenceLimit caps a stored evidence string. A TXT record can be long and
// the reader needs enough to recognize it, not all of it.
const evidenceLimit = 200

// truncate bounds one stored evidence string at evidenceLimit.
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= evidenceLimit {
		return s
	}
	return s[:evidenceLimit] + "…"
}
