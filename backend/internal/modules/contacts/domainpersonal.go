// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The sender-name heuristic: does this domain's own label say it belongs to a
// contact rather than a company? It answers only when the triage crawl could not
// — an unreachable site, a domain that resolves to nothing — because a read
// site is always better evidence than a name coincidence.
//
// It is the last line before falling back to creating a company. Getting it
// wrong in one direction costs a real one-contact consultancy its company
// row; in the other it manufactures a company named after a human. So the rule
// is deliberately narrow: the domain's registrable label must BE somebody's
// name, spelled the way contacts spell their own domains.

import "strings"

// DomainContact is one human the workspace already records on a domain: their
// name as captured, and the local parts of EVERY address they hold on it. Both
// matter — "Ines Marsh" writing from rowan@rowanmarsh.example is only explained
// by joining the local part to the surname.
//
// The locals are a list because a contact often has two mailboxes on their own
// domain. One entry per ADDRESS would make those look like two different
// contacts, and since the test below requires every contact to be explained, a
// second mailbox would silently turn a personal domain into a company.
type DomainContact struct {
	FullName    string
	EmailLocals []string
}

// domainPersonalSimilarity is how close a spelling has to be to a name before
// it counts as that name. High on purpose: "baumert" IS Alexander Baumert's
// surname and that domain is a real agency, so the heuristic must not also
// start matching near-misses — it already stands on thin evidence.
const domainPersonalSimilarity = 0.94

// DomainLooksPersonal reports whether the domain's registrable label is a
// natural contact's name. label is that label alone ("kestner" from
// kestner.example), not the full domain.
//
// The answer is yes only when EVERY known contact on the domain is explained by
// it. One contact whose name is the domain is a personal domain; two contacts with
// different family names sharing it is a company however well the first one
// matches, because a company is exactly what a shared domain is.
//
// With nobody known it answers no: there is then no name to compare, and
// refusing a company on no evidence at all would be a guess.
func DomainLooksPersonal(label string, contacts []DomainContact) bool {
	label = normalizeName(strings.Join(strings.FieldsFunc(label, isNameSeparator), ""))
	if label == "" || len(contacts) == 0 {
		return false
	}
	for _, p := range contacts {
		if !contactExplainsLabel(label, p) {
			return false
		}
	}
	return true
}

// contactExplainsLabel reports whether this one human's name accounts for the
// domain label, in any of the spellings contacts actually register: the surname
// alone, the full name run together either way round, or — the case a header
// display name alone never explains — the address's own local part joined to
// the surname.
func contactExplainsLabel(label string, p DomainContact) bool {
	for _, candidate := range contactLabelCandidates(p) {
		if candidate == "" {
			continue
		}
		if candidate == label || jaroWinkler(candidate, label) >= domainPersonalSimilarity {
			return true
		}
	}
	return false
}

// contactLabelCandidates spells one contact's name every way a domain label
// plausibly renders it. Order does not matter; the caller takes any match.
//
// The local part NEVER stands alone as a candidate. A role address on its own
// domain — tvpartner@tvpartner.example — would otherwise read as a personal domain, when a
// mailbox named after the company is if anything the opposite signal. It counts
// only joined to a name part, which is the case a display name cannot explain
// by itself.
func contactLabelCandidates(p DomainContact) []string {
	parts := strings.Fields(nameWithoutAffiliation(p.FullName))
	// A one-word name is not a contact's name, it is a label — `"Acme"
	// <info@acme.example>` is how a company signs its own mail. Matching it
	// against the domain would read the company's own name as a human's and
	// refuse the company its record.
	if len(parts) < 2 {
		return nil
	}
	first, last := parts[0], parts[len(parts)-1]
	// Every ordering of the whole name, not just first+last: a domain built
	// from a middle name ("annakatharinaweber") is still that contact's own.
	out := []string{last, strings.Join(parts, ""), last + strings.Join(parts[:len(parts)-1], ""), first + last}
	for _, raw := range p.EmailLocals {
		local := normalizeName(strings.Join(strings.FieldsFunc(raw, isNameSeparator), ""))
		if local == "" {
			continue
		}
		// rowan@rowanmarsh.example against "Ines Marsh": the address carries
		// the given name the header does not.
		out = append(out, local+last, last+local)
	}
	return out
}

// nameWithoutAffiliation keeps the part of a display name that is the contact,
// dropping the employer contacts staple onto it — "Tomas Vidal - TVPartner",
// "Nikos Adamos (Cloud Atlas)", "Ed Sander @ ChinaTechTrip". Without
// this the company reads as the surname, and a domain named after the COMPANY
// would then look like it was named after the contact.
func nameWithoutAffiliation(name string) string {
	normalized := normalizeName(name)
	for _, sep := range []string{" - ", " – ", " — ", " | ", " • ", " · ", " @ ", "(", ",", "/"} {
		if head, _, found := strings.Cut(normalized, sep); found {
			normalized = strings.TrimSpace(head)
		}
	}
	return normalized
}

// isNameSeparator splits the punctuation a domain label or a mail local part
// uses where a name has a space: "acme-corp", "first.last", "first_last".
func isNameSeparator(r rune) bool { return r == '-' || r == '_' || r == '.' || r == '+' }
