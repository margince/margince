// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type position struct {
	Score     float64    `json:"s"`
	Kind      string     `json:"k"`
	ID        ids.UUID   `json:"id"`
	CreatedAt *time.Time `json:"at,omitempty"`
}

func TestAPositionSurvivesTheRoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 25, 9, 30, 0, 0, time.UTC)
	want := position{Score: 0.75, Kind: "person", ID: ids.NewV7(), CreatedAt: &at}
	token, err := storekit.EncodeOpaque(want)
	if err != nil {
		t.Fatalf("minting a token: %v", err)
	}
	got, err := storekit.DecodeOpaque[position](token)
	if err != nil {
		t.Fatalf("decoding a token this package minted: %v", err)
	}
	if got.Score != want.Score || got.Kind != want.Kind || got.ID != want.ID ||
		got.CreatedAt == nil || !got.CreatedAt.Equal(at) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

// A relevance score is a keyset, so a rounded one skips or repeats rows on the
// page boundary. The delimited encoding this replaced spelled the precision out
// (strconv 'g' -1) and the property is now load-bearing on encoding/json's
// shortest-round-trip behaviour instead — which is a fact about the stdlib
// nobody would think to check, and exactly the kind that stops being true
// quietly. So it is asked, at the edges where a lesser encoder would fail.
func TestAScoreKeepsEveryBitAcrossTheEnvelope(t *testing.T) {
	for _, score := range []float64{
		0.1,
		1.0 / 3.0,
		0.30000000000000004, // the classic sum whose shortest form is 17 digits
		123456789.123456789,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		1e-300,
	} {
		token, err := storekit.EncodeOpaque(position{Score: score})
		if err != nil {
			t.Fatalf("minting a score of %v: %v", score, err)
		}
		got, err := storekit.DecodeOpaque[position](token)
		if err != nil {
			t.Fatalf("decoding a score of %v: %v", score, err)
		}
		if got.Score != score {
			t.Errorf("a score of %v came back as %v; a keyset that loses a bit skips or repeats a row "+
				"on the page boundary", score, got.Score)
		}
	}
}

func TestATokenThisPackageDidNotMintIsTheClientsMistake(t *testing.T) {
	for _, name := range []string{
		"not base64 at all",
		"base64 of something that is not JSON",
	} {
		t.Run(name, func(t *testing.T) {
			token := "not-base64-!!"
			if name == "base64 of something that is not JSON" {
				token = "X19fXw"
			}
			if _, err := storekit.DecodeOpaque[position](token); !errors.As(err, new(*storekit.MalformedCursorError)) {
				t.Errorf("decoding %q gave %v, want MalformedCursorError — httperr answers 422 on that "+
					"sentinel and falls through to a 500 on anything else", token, err)
			}
		})
	}
}

// Well-formed JSON is not yet a position. `{}` and `null` both unmarshal
// without error and leave every field at its zero value, which reads as a real
// place in the list rather than as a refusal — and the envelope deliberately
// does NOT judge that, because only the caller knows which field being zero is
// the tell. This pins the behaviour the callers are written against.
func TestAnEmptyDocumentDecodesToAZeroPositionRatherThanARefusal(t *testing.T) {
	for _, token := range []string{"e30", "bnVsbA"} { // {} and null
		got, err := storekit.DecodeOpaque[position](token)
		if err != nil {
			t.Fatalf("decoding %q gave %v; the envelope must leave this judgement to the caller", token, err)
		}
		if got != (position{}) {
			t.Errorf("decoding %q = %+v, want the zero position", token, got)
		}
	}
}

// The failure that is not obvious: a position carrying an instant Postgres can
// store but JSON cannot write. Swallowing it would hand a caller an empty token
// beside "there is more" — a page a client can ask for and never receive.
func TestAnInstantJSONCannotWriteIsAnErrorRatherThanAnEmptyToken(t *testing.T) {
	beyond := time.Date(294276, 1, 1, 0, 0, 0, 0, time.UTC)
	token, err := storekit.EncodeOpaque(position{CreatedAt: &beyond})
	if err == nil {
		t.Fatalf("a year-294276 instant encoded to %q; Postgres stores that timestamp and JSON has no "+
			"form for it, so the token would be empty and the page unreachable", token)
	}
	if token != "" {
		t.Errorf("the failed mint returned %q, want no token at all", token)
	}
}

// A generic keyset position needs BOTH halves of its tuple. `null` and `{}`
// decode cleanly into a zero Cursor, and every caller reads a zero position as
// the top of the list — so a token nobody minted would silently restart the
// walk instead of being refused.
func TestAGenericCursorMissingHalfItsTupleIsRefused(t *testing.T) {
	at := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	for name, position := range map[string]storekit.Cursor{
		"no id at all":                {CreatedAt: at},
		"no instant":                  {ID: ids.NewV7()},
		"neither, as `{}` decodes to": {},
	} {
		token, err := storekit.EncodeOpaque(position)
		if err != nil {
			t.Fatalf("minting %s: %v", name, err)
		}
		if _, err := storekit.DecodeCursor(token); !errors.As(err, new(*storekit.MalformedCursorError)) {
			t.Errorf("DecodeCursor(<%s>) = %v, want MalformedCursorError — a zero position reads as the "+
				"top of the list and pages it from the start", name, err)
		}
	}
	// And the whole tuple still round-trips.
	whole := storekit.Cursor{CreatedAt: at, ID: ids.NewV7()}
	token, err := storekit.EncodeOpaque(whole)
	if err != nil {
		t.Fatalf("minting a whole position: %v", err)
	}
	got, err := storekit.DecodeCursor(token)
	if err != nil {
		t.Fatalf("DecodeCursor(<a whole position>): %v", err)
	}
	if !got.CreatedAt.Equal(whole.CreatedAt) || got.ID != whole.ID {
		t.Errorf("round trip = %+v, want %+v", got, whole)
	}
}
