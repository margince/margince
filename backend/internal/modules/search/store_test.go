// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRankedCursorRoundTripsFullPrecision(t *testing.T) {
	in := rankedCursor{Score: 0.1234567890123456789, Type: "activity", ID: ids.NewV7()}
	out, err := decodeCursor(encodeCursor(in))
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("cursor round trip lost information: %+v != %+v", out, in)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	for name, cursor := range map[string]string{
		"not base64":    "%%%",
		"too few parts": base64.RawURLEncoding.EncodeToString([]byte("only-one")),
		"bad score":     base64.RawURLEncoding.EncodeToString([]byte("NaNish|contact|" + ids.NewV7().String())),
		"bad id":        base64.RawURLEncoding.EncodeToString([]byte("0.5|contact|not-a-uuid")),
	} {
		if _, err := decodeCursor(cursor); err == nil {
			t.Errorf("%s: malformed cursor accepted", name)
		}
	}
}

func TestClampLimitFollowsTheContractBounds(t *testing.T) {
	for in, want := range map[int]int{0: 50, -3: 50, 1: 1, 200: 200, 900: 200} {
		if got := clampLimit(in); got != want {
			t.Errorf("clampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

// A query past the word cap is refused before any SQL is built. The Store has
// no database, so a query that reached one would fail differently.
func TestAQueryWithTooManyWordsIsRefusedBeforeItRuns(t *testing.T) {
	words := strings.TrimSpace(strings.Repeat("acme ", maxQueryWords+1))
	_, err := (&Store{}).Search(context.Background(), Input{Query: words, WithEmployees: true})
	bad, ok := errors.AsType[*BadQueryError](err)
	if !ok || bad.Field != "q" || !strings.Contains(bad.Reason, strconv.Itoa(maxQueryWords)) {
		t.Fatalf("a %d-word query answered %v, want a refusal of q naming the limit %d", maxQueryWords+1, err, maxQueryWords)
	}
}
