// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// One key keeps every refusal below reporting the same field, so a client can
// bind all of them to one form input instead of matching on whichever spelling
// the failing branch happened to use.
const linkedinURLField = "linkedin_url"

// NormalizeLinkedInURL reduces a LinkedIn profile URL to the one stored
// spelling the E12.11 exact-match dedupe key compares on: https scheme,
// lowercased host, no query, no fragment, no trailing slash. Parsed once
// at the seam (the values.ParseEmail stance), so the dedupe probe, the
// insert, and the audit image all see the same key.
func NormalizeLinkedInURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", &values.ParseError{
			Field: linkedinURLField, Code: "linkedin_url_empty",
			Message: "a LinkedIn profile URL is required",
		}
	}
	malformed := &values.ParseError{
		Field: linkedinURLField, Code: "linkedin_url_malformed",
		Message: "not a resolvable profile URL",
	}
	// A pasted profile often arrives with no authority at all
	// ("linkedin.com/in/x", which parses entirely as a path), and a crawled one
	// often arrives with the authority alone ("//linkedin.com/in/x", how a page
	// writes a link that follows its own scheme). Neither names a different
	// profile than the full form, so the scheme is supplied rather than made a
	// refusal — and the two are told apart by the "//" itself rather than by an
	// empty hostname, which "https://" also has and which must stay malformed.
	if !strings.Contains(trimmed, "//") {
		trimmed = "https://" + trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Hostname() == "" {
		return "", malformed
	}
	if u.Scheme == "" {
		u.Scheme = schemeHTTPS
	}
	if u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS {
		return "", &values.ParseError{
			Field: linkedinURLField, Code: "linkedin_url_malformed",
			Message: "a profile URL uses http or https",
		}
	}
	// http and https address the same profile; canonicalizing to https
	// keeps the dedupe key one spelling per identity. Ports, query and
	// fragment carry tracking noise, never identity.
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	return "https://" + strings.ToLower(u.Hostname()) + path, nil
}

// LinkedInSlotHosts are the hosts a value has to be on before this product will
// store it as a contact's LinkedIn profile — the person_social slot, the vCard
// import's slot, and the classifier that decides LinkedIn from website.
//
// Subdomains count, because LinkedIn's own localized profile links carry them:
// de.linkedin.com and www.linkedin.com are the same site, and a reader who
// pasted either meant their profile.
func LinkedInSlotHosts() []string { return []string{"linkedin.com"} }

// LinkedInDisplayOnlyHosts are hosts the CLIENT draws under the word "LinkedIn"
// that this writer will not put in the slot.
//
// lnkd.in is LinkedIn's own shortener, so a stored one is honestly a LinkedIn
// link and the reader loses nothing by having it drawn as one. It is not
// storable, because the slot is an IDENTITY: it is compared verbatim against
// other profiles for the exact-match dedupe key, and a shortener carries a
// token rather than the profile path, so two shortened links to one person
// never read as the same person and the provider resolver has no handle to look
// up. Widening the writer to admit it would put values in the slot that cannot
// do the slot's job.
//
// Held equal to frontend/src/format/weburl.ts's LINKEDIN_HOSTS by
// backend/gates/frontendlinkedinhosts_test.go, in both directions: the split is
// deliberate and a gate is what keeps it deliberate rather than forgotten.
func LinkedInDisplayOnlyHosts() []string { return []string{"lnkd.in"} }

// onHost reports whether host is one of the allowed hosts or a subdomain of one.
func onHost(host string, allowed []string) bool {
	for _, name := range allowed {
		if host == name || strings.HasSuffix(host, "."+name) {
			return true
		}
	}
	return false
}
