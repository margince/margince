// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// IdentityEmail folds an address to its identity key: case folded and the
// domain in its ASCII (IDNA) form. `anna@bücher.example` and
// `anna@xn--bcher-kva.example` are one mailbox and so one key.
func IdentityEmail(email string) string {
	folded := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndexByte(folded, '@')
	if at < 0 {
		return folded
	}
	return folded[:at+1] + owndomain.Fold(folded[at+1:])
}

// EmailSpellings are the stored spellings one mailbox can have: the address
// case-folded as typed, and with its domain in ASCII and in Unicode form.
// Postgres cannot fold IDNA, so a lookup against a stored address matches any
// of them.
func EmailSpellings(email string) []string {
	folded := strings.ToLower(strings.TrimSpace(email))
	if folded == "" {
		return nil
	}
	out := []string{folded}
	at := strings.LastIndexByte(folded, '@')
	if at < 0 {
		return out
	}
	for _, domain := range owndomain.Spellings(folded[at+1:]) {
		if spelled := folded[:at+1] + domain; spelled != folded {
			out = append(out, spelled)
		}
	}
	return out
}

// emailSpellingsOf is EmailSpellings over a list, without repeats.
func emailSpellingsOf(emails []string) []string {
	out := make([]string, 0, len(emails))
	seen := map[string]bool{}
	for _, email := range emails {
		for _, spelled := range EmailSpellings(email) {
			if !seen[spelled] {
				seen[spelled] = true
				out = append(out, spelled)
			}
		}
	}
	return out
}
