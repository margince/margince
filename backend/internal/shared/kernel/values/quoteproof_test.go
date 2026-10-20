// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

func TestQuoteProofChecksCurrentWordsWithoutRetainingThem(t *testing.T) {
	proof, ok := ProveQuote("Hallo.\n Wir beenden\tden Pilot.", "Wir beenden den Pilot.")
	if !ok {
		t.Fatal("quoted words were refused")
	}
	quote, ok := VerifyQuote("Hallo. Wir beenden den Pilot.", proof)
	if !ok || quote != "Wir beenden den Pilot." {
		t.Fatalf("verified quote=%q, %v", quote, ok)
	}
	if _, ok := VerifyQuote("Hallo. Wir verlängern den Pilot.", proof); ok {
		t.Fatal("changed words satisfied the old proof")
	}
	if _, ok := VerifyQuote("Hallo. Wir beenden den Pilot. Actually, all services continue.", proof); ok {
		t.Fatal("changed source context satisfied the old scope proof")
	}
	for _, invalid := range []QuoteProof{{Start: -1}, {Start: 0, End: 1000}, {Start: 0, End: 1, SHA256: "wrong"}} {
		if _, ok := VerifyQuote("text", invalid); ok {
			t.Fatalf("invalid proof admitted: %+v", invalid)
		}
	}
	for _, absent := range []string{"", "not in the source"} {
		if _, ok := ProveQuote("source", absent); ok {
			t.Fatalf("absent quote admitted: %q", absent)
		}
	}
}
