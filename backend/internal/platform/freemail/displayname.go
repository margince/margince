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

import "strings"

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
	label := RegistrableLabel(normalized)
	if label == "" {
		return normalized
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
