// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package freemail

import "testing"

// DisplayName names a company from a domain, and the two cases below are the
// ones where naming it WRONG is worse than naming nothing: a fabrication and a
// transport artefact, both rendered in title case so they read like a real
// answer.
func TestDisplayNameNamesTheCompanyOrNothingAtAll(t *testing.T) {
	for _, tc := range []struct{ domain, want string }{
		// The registrable label, which is the whole point.
		{"gitex.com", "Gitex"},
		{"acme-corp.co.uk", "Acme Corp"},
		{"eu.docusign.net", "Docusign"},
		{"mail.acme_corp.com", "Acme Corp"},
		// An unknown or intranet TLD still derives cleanly, so requiring the
		// public-suffix walk loses nothing legitimate.
		{"acme.internal", "Acme"},

		// A bare public suffix has no company in it. Cutting at the first dot
		// yielded "Co" from "co.uk" — a fabrication wearing a title case, and
		// one that would be recorded as somebody's employer.
		{"co.uk", "co.uk"},
		{"com", "com"},
		// An IDN suffix comes back in Unicode too. The early return for a bare
		// suffix once skipped the decode, so "рф" came back as "xn--p1ai" —
		// transport shown where a name belongs.
		{"рф", "рф"},
		{"xn--p1ai", "рф"},

		// Punycode is TRANSPORT. normalize folds a Unicode domain to its xn--
		// form because that is what a mail header carries; titling that form
		// rendered "müll.email" as "Xn Mll Hoa".
		{"müll.email", "Müll"},
		{"xn--mll-hoa.email", "Müll"},

		// A single unknown label is NOT a public suffix anybody registers under
		// — it is a hostname somebody typed, so titling the whole input is the
		// honest last resort rather than a refusal. ICANN membership is what
		// separates this from "com" above; "is it its own suffix" is true of
		// both and would have refused to name this one.
		{"localhostonly", "Localhostonly"},

		// Nothing in, nothing out — never a guess.
		{"", ""},
		{"   ", ""},
	} {
		if got := DisplayName(tc.domain); got != tc.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tc.domain, got, tc.want)
		}
	}
}

// The label is read for its own sake too — people's domain triage asks whether
// it looks like a person rather than a business — so it has to answer about the
// same string a name is derived from.
func TestRegistrableLabelIsTheLabelDisplayNameTitles(t *testing.T) {
	for _, domain := range []string{"gitex.com", "eu.docusign.net", "acme-corp.co.uk", "acme.internal"} {
		label := RegistrableLabel(domain)
		if label == "" {
			t.Errorf("RegistrableLabel(%q) is empty", domain)
			continue
		}
		if got := DisplayName(domain); got != titleizeLabel(label) {
			t.Errorf("DisplayName(%q) = %q, but the label is %q", domain, got, label)
		}
	}
}
