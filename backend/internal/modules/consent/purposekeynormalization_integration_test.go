// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestStoredPurposeReferencesNormalizeAsTheSendGateDoes(t *testing.T) {
	e := setupChannelConsent(t)
	inputs := []string{"Spring_News", "  SPRING_NEWS  ", "\tSpring_News\r\n"}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if unicode.IsSpace(r) {
			inputs = append(inputs, string(r)+"Spring_News"+string(r))
		}
		lower := unicode.ToLower(r)
		if r != lower && lower >= 'a' && lower <= 'z' {
			inputs = append(inputs, "spring_"+string(r)+"ews")
		}
	}
	rows, err := e.owner.Query(e.ctx, `SELECT value,public.normalized_consent_purpose_key(value) FROM unnest($1::text[]) value`, inputs)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var input, got string
		if err := rows.Scan(&input, &got); err != nil {
			t.Fatal(err)
		}
		if want := commsauthz.NormalizedPurposeKey(input); got != want {
			t.Errorf("SQL key %q normalized to %q, Go send gate uses %q", input, got, want)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != len(inputs) {
		t.Fatalf("read %d of %d keys", count, len(inputs))
	}
}
