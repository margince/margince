// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package freemail

// Turning a mail domain into a name a person would recognise.
//
// It lives beside the consumer-provider question because the two are asked
// TOGETHER, always and by everybody: a caller deriving a company from an
// address must first know the domain is not a mailbox host, and both answers
// come out of the same public-suffix walk. Splitting them put the list in one
// package and the derivation in another, and the second door then grew its own
// copy of each.

import (
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// DisplayName turns a mail domain into a readable organization name by
// title-casing its registrable label: "gitex.com" → "Gitex",
// "acme-corp.co.uk" → "Acme Corp", "eu.docusign.net" → "Docusign".
//
// The public-suffix list is what makes the third example right, and it is the
// case a naive first-label split gets wrong in the direction that matters:
// splitting on the first dot yields "Eu", which is not a company and reads as
// a bug in front of whoever is looking at the record.
//
// Falls back to the normalized domain when no registrable label can be found —
// a bare public suffix, an intranet label — an honest last resort rather than a
// fabrication. Callers that persist the result stamp it as provisional
// (organization.name_source='domain'); a lead's own company_name column has no
// such marker, and needs none, because it is free text a human is expected to
// correct.
func DisplayName(domain string) string {
	normalized := normalize(domain)
	if normalized == "" {
		return ""
	}
	// A bare public suffix has no registrable label to name. `Registrable`
	// passes such a domain through unchanged, so cutting it would yield "co"
	// from "co.uk" and render a company called "Co" — a fabrication wearing a
	// title case. The documented fallback is the domain itself.
	//
	// ICANN is the discriminator, not "is it its own suffix". Every unknown
	// single label is its own suffix as far as the list is concerned, so the
	// broader test would refuse to name "localhostonly" — a hostname somebody
	// typed, where titling the whole input IS the honest last resort. "co.uk"
	// and "com" are on the ICANN list and are nobody's company; "internal" and
	// "localhostonly" are not.
	if suffix, icann := publicsuffix.PublicSuffix(normalized); icann && suffix == normalized {
		// Decoded on the way out like every other return: an IDN suffix is
		// normalized to punycode on the way in, and handing back "xn--p1ai"
		// where the input said "рф" shows transport where a name belongs.
		if unicodeSuffix, err := idna.ToUnicode(normalized); err == nil {
			return unicodeSuffix
		}
		return normalized
	}
	label := RegistrableLabel(normalized)
	if label == "" {
		return normalized
	}
	// Punycode is TRANSPORT, not a name. `normalize` folds a Unicode domain to
	// its xn-- form because that is what a mail header carries, and titling
	// that form renders "müll.email" as "Xn Mll Hoa". The display half decodes
	// it again; a label that will not decode keeps its ASCII form rather than
	// being dropped.
	if unicodeLabel, err := idna.ToUnicode(label); err == nil {
		label = unicodeLabel
	}
	return titleizeLabel(label)
}

// RegistrableLabel returns the single label immediately left of the public
// suffix — the part a human reads as the company. "eu.docusign.net" →
// "docusign"; "acme.co.uk" → "acme".
//
// Exported because the label is read for its OWN sake as well as for a name:
// people's domain triage asks whether a label looks like a person rather than
// a business, and it has to ask about the same label this derives a name from
// or the two answers are about different strings.
func RegistrableLabel(domain string) string {
	base := Registrable(domain)
	if base == "" {
		return ""
	}
	// Registrable passes a domain through unchanged when it can derive no
	// eTLD+1, so `base` may still be the whole input. Cutting at the first dot
	// is right in both cases: on a derived eTLD+1 it is the registrable label,
	// and on the passthrough it is the best name available.
	label, _, _ := strings.Cut(base, ".")
	return label
}

// titleizeLabel renders a registrable label as a display name: word-split on
// '-' and '_', each word capitalized. "acme-corp" → "Acme Corp". A label with
// no separators is simply capitalized ("gitex" → "Gitex").
func titleizeLabel(label string) string {
	words := strings.FieldsFunc(label, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		words[i] = capitalizeFirst(w)
	}
	if len(words) == 0 {
		return capitalizeFirst(label)
	}
	return strings.Join(words, " ")
}

// capitalizeFirst upper-cases the first rune and leaves the rest untouched.
// The input is always an already-lowercased domain label (mail domains are
// case-insensitive, so the domain's own casing carries no signal and is
// discarded at normalize) — leaving the tail as-is is simply the cheapest
// correct thing, not a bid to preserve inner case.
func capitalizeFirst(w string) string {
	if w == "" {
		return ""
	}
	r := []rune(w)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
