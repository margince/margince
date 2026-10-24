// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package credentialmask

import (
	"fmt"
	"strings"
)

// A rule's witnesses are texts it must mask. Together they take every branch of
// the rule's alternations, and the tests hold each one against the rule's cues,
// so a cue that misses a shape fails a test. Vendor-shaped witnesses are built
// by concatenation, so the source holds nothing a secret scanner reads as a key.

// The bare words of the ambiguous labels, which the strong labels end in.
const (
	wordSecret = "secret"
	wordToken  = "token"
	wordKey    = "key"
)

// The label words spell every branch of the label patterns.
var (
	passwordWords  = []string{"password", "passwd", "pwd", "passwort", "kennwort", "passcode", "pin", "pin code", "pin-code", "pincode"}
	ambiguousWords = []string{wordSecret, wordToken, wordKey}
	loginWords     = []string{"login", "user", "username", "benutzer", "benutzername"}
	strongHeads    = []string{"api", "access", "auth", "client", "private", wordSecret}
	strongTails    = []string{wordKey, wordToken, wordToken, wordSecret, wordKey, wordKey}
	strongWords    = spelledWith(strongHeads, strongTails, []string{" ", "_", "-", ""})
	jsonWords      = append([]string{"password", "passwd", "pwd", "passwort", "kennwort", "passcode", wordSecret},
		spelledWith(strongHeads[:5], strongTails[:5], []string{"_", "-", ""})...)
)

// spelledWith joins each head to its tail through every separator.
func spelledWith(heads, tails, separators []string) []string {
	var words []string
	for i, head := range heads {
		for _, separator := range separators {
			words = append(words, head+separator+tails[i])
		}
	}
	return words
}

// witnessesOf puts each word into format.
func witnessesOf(words []string, format string) []string {
	out := make([]string, 0, len(words))
	for _, word := range words {
		out = append(out, fmt.Sprintf(format, word))
	}
	return out
}

// loginWitnesses pairs every login word with a password word and back, so both
// label sets of the login rule are taken.
func loginWitnesses() []string {
	var out []string
	for i := range max(len(loginWords), len(passwordWords)) {
		out = append(out, loginWords[i%len(loginWords)]+": anna\n"+passwordWords[i%len(passwordWords)]+": Quokka7")
	}
	return out
}

// vendorKeys is one key per prefix, each prefix + middle + a filler.
func vendorKeys(prefixes []string, middle string, filler int) []string {
	out := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		out = append(out, prefix+middle+strings.Repeat("a1", filler))
	}
	return out
}
