// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestAVCardTitleIsCutToTheContactTitleLimit(t *testing.T) {
	card := "BEGIN:VCARD\nFN:Dana Buyer\nTITLE:" + strings.Repeat("ế", 300) + "\nEND:VCARD\n"
	entries, err := ParseVCards(strings.NewReader(card))
	if err != nil || len(entries) != 1 {
		t.Fatalf("parsed %d cards: %v", len(entries), err)
	}
	if got, want := len([]rune(entries[0].Title)), textbounds.LimitOf("public.contact.title"); got != want {
		t.Fatalf("title holds %d characters, want the contact.title limit of %d", got, want)
	}
}

func TestABoughtJobTitleIsCutToTheContactTitleLimit(t *testing.T) {
	employment, err := json.Marshal(providerEmployment{JobTitle: " " + strings.Repeat("a", 200) + " "})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := decodeApplicableClaims([]provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: employment}})
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("a", textbounds.LimitOf("public.contact.title")); claims.title != want {
		t.Fatalf("title holds %d characters, want the contact.title limit", len(claims.title))
	}
}
