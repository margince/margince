// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailrole answers one question: does this address name a FUNCTION an
// organization answers rather than a person? A yes means no contact may be
// created for it — support@acme.com is a queue, not somebody called "Support".
//
// It is not the machine-address question. `noreply@` reaches nobody and capture
// refuses it on those grounds (recordWorthy); `support@` reaches real people
// typing real replies all day, and the correspondence is genuine. What is
// missing is a PERSON to name, which is why a role mailbox is captured, kept
// visible, and simply never turned into a contact record.
//
// Two modules need the same answer from opposite ends of the capture path:
// capture's tier ladder gates creation, and people's name parser must not lift
// a role word into somebody's name. Neither may import the other, and a second
// spelling of the list would be a second answer — the defect that put a contact
// called "Billing" and one called "Events The Sentry" in a shared CRM.
//
// It sits in platform for the same reason freemail does: it owns no domain —
// mail addressing is plumbing, not CRM.
package mailrole

import "strings"

// Match reports whether an address names a role mailbox, and which token said
// so. The token is for a ledger reason and for tests; it is never shown to a
// user and never becomes part of a public vocabulary.
//
// The empty string and an address with no local part are not role mailboxes:
// this answers a question about a name, and there is nothing to read.
func Match(address string) (string, bool) {
	local, domain, ok := split(address)
	if !ok {
		return "", false
	}
	if token, found := localPartRole(local); found {
		return token, true
	}
	// A helpdesk vendor answers on its customer's behalf, so the mailbox is the
	// vendor's queue however the local part reads: `idy4dl62-9rnjp@…zendesk.com`
	// carries no role word at all and is still nobody.
	if vendor := helpdeskVendor(domain); vendor != "" {
		return "helpdesk:" + vendor, true
	}
	return "", false
}

// IsRoleLocalPart reports whether a bare local part — no domain — names a
// function. The name parser holds one of these and no address.
func IsRoleLocalPart(local string) bool {
	_, found := localPartRole(local)
	return found
}

// DisplayName reports whether a header display name is nothing but role words:
// "Billing", "APAC Billing", "Support Team". Such a name is a department, and
// storing it as a person's full name invents somebody.
//
// A name carrying any word that is not a role token is left alone — "Anna from
// Billing" names Anna, and refusing it would lose a real person.
func DisplayName(name string) bool {
	fields := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !isNameRune(r)
	})
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		if _, role := roleTokens[field]; role {
			continue
		}
		// A qualifier that only ever modifies a department — a region, "team",
		// "dept" — is not itself a person's name.
		if _, qualifier := roleQualifiers[field]; qualifier {
			continue
		}
		return false
	}
	// At least one field has to be an actual role word: a name made only of
	// qualifiers ("APAC") names a region, and this function is not the place to
	// decide what that is.
	for _, field := range fields {
		if _, role := roleTokens[field]; role {
			return true
		}
	}
	return false
}

// PromptExamples are the role words the AI verdict's prompt names as examples.
//
// The prompt is a second statement of this rule — it tells the model which local
// parts make a role mailbox — and it disagreed with this list about `service@`
// for exactly as long as the two were written independently. A reviewer found
// that; nothing in the tree would have.
//
// It returns words rather than a sentence: the prompt owns its own phrasing, and
// only the vocabulary has to be one answer.
//
// Held by: TestThePromptsRoleExamplesAreAllRoleTokens (backend/internal/platform/mailrole/mailrole_test.go)
func PromptExamples() []string {
	return []string{"support", "info", "sales", "office", "service", "kontakt"}
}

// Tokens returns the role vocabulary, sorted for a stable gate corpus. The
// fitness function that forbids a second spelling of this list derives its
// subject from here rather than from a sample of its own.
func Tokens() []string {
	out := make([]string, 0, len(roleTokens))
	for token := range roleTokens {
		out = append(out, token)
	}
	sortStrings(out)
	return out
}

// split takes an address apart, stripping the plus-addressing tag the mailbox
// owner appended: `support+idy4dl62-9rnjp@zendesk.com` is the support queue
// with a ticket routing tag, and reading the tag as part of the name makes one
// queue look like thousands of separate senders.
func split(address string) (local, domain string, ok bool) {
	address = strings.ToLower(strings.TrimSpace(address))
	at := strings.LastIndex(address, "@")
	if at <= 0 || at == len(address)-1 {
		return "", "", false
	}
	local, domain = address[:at], address[at+1:]
	if base, _, found := strings.Cut(local, "+"); found {
		if base == "" {
			// `+tag@host` carries no local part at all. Nothing to read.
			return "", "", false
		}
		local = base
	}
	return local, domain, true
}

// localPartRole reads the local part as a sequence of words and reports the
// first that names a function. Splitting matters: `hello.events@` and
// `asia-accounting@` are role mailboxes whose role word is not the whole local
// part, and matching the whole string would miss both.
func localPartRole(local string) (string, bool) {
	local = strings.ToLower(strings.TrimSpace(local))
	if local == "" {
		return "", false
	}
	if base, _, found := strings.Cut(local, "+"); found && base != "" {
		local = base
	}
	if token, whole := roleTokens[local]; whole {
		_ = token
		return local, true
	}
	for _, field := range strings.FieldsFunc(local, isSeparator) {
		if _, role := roleTokens[field]; role {
			return field, true
		}
	}
	return "", false
}

// isSeparator splits a local part into words. Only the three characters mail
// addresses conventionally use as word separators — a digit or a letter never
// ends a word here, so `supporter@` keeps its single field and stays a name.
func isSeparator(r rune) bool { return r == '.' || r == '_' || r == '-' }

func isNameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func helpdeskVendor(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	for vendor := range helpdeskVendors {
		// A subdomain is the ordinary shape: `acme.zendesk.com`. Suffix matching
		// on a dot-prefixed vendor cannot match a lookalike registration
		// (`notzendesk.com`), which a bare strings.Contains would.
		if domain == vendor || strings.HasSuffix(domain, "."+vendor) {
			return vendor
		}
	}
	return ""
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
}

// GreetsNobody reports whether a display name and the address behind it,
// together, name no natural person — so a greeting built from that name would
// address somebody who does not exist.
//
// Three ways a mailbox can name nobody, and the third is why this is not just
// Match:
//
//  1. The address is a role mailbox (Match).
//  2. The display name is nothing but role words (DisplayName).
//  3. The display name OPENS with the mailbox's own domain label. That token is
//     the organization's name, so a greeting takes it for a first name and
//     writes "steireif," to `partner@steireif.net` — a company greeted as a
//     person, in the message a rep is about to send. `partner` is deliberately
//     not in the role vocabulary, because it is ordinary business vocabulary a
//     person's address may honestly contain, so rule 1 cannot reach this and
//     widening the vocabulary would refuse real contacts.
//
// Rule 3 costs a greeting where a directory writes surname first and the
// surname IS the company's — "Steireif Anna" at steireif.net. That draft opens
// "Hallo," instead of "Hallo Anna,", which is the fallback the drafting floor
// already renders for a contact it has no name for. Abstaining is the cheaper
// error: a missing name reads as plain, an invented one reads as a mistake the
// recipient can see.
func GreetsNobody(displayName, address string) bool {
	if _, role := Match(address); role {
		return true
	}
	if DisplayName(displayName) {
		return true
	}
	return opensWithDomainLabel(displayName, address)
}

// opensWithDomainLabel reports whether the display name's first word is the
// address's own domain label.
func opensWithDomainLabel(displayName, address string) bool {
	_, domain, ok := split(address)
	if !ok {
		return false
	}
	label, _, found := strings.Cut(domain, ".")
	if !found || label == "" {
		return false
	}
	fields := strings.FieldsFunc(strings.ToLower(displayName), func(r rune) bool {
		return !isNameRune(r)
	})
	return len(fields) > 0 && fields[0] == label
}
