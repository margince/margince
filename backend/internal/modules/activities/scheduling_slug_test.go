// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A reader that first yields the bytes base64url spells as "manage-…", then
// an ordinary draw.
func scriptedReader(t *testing.T, draws ...[]byte) func([]byte) (int, error) {
	t.Helper()
	return func(buf []byte) (int, error) {
		if len(draws) == 0 {
			t.Fatal("drew more than scripted")
		}
		n := copy(buf, draws[0])
		draws = draws[1:]
		return n, nil
	}
}

func TestAMintedSlugNeverTakesAReservedAddress(t *testing.T) {
	// The bare ids ("contact", "preview") are shorter than any 32-character
	// slug, so only the prefixes can be drawn.
	for _, reserved := range reservedBookingPrefixes {
		// Pad the reserved spelling to a full 32-character slug and decode it
		// back to the 24 bytes that would produce it.
		spelled := (reserved + strings.Repeat("A", 32))[:32]
		raw, err := base64.RawURLEncoding.DecodeString(spelled)
		if err != nil {
			t.Fatalf("%s: %v", reserved, err)
		}
		ordinary := make([]byte, 24)
		slug, err := mintBookingSlug(scriptedReader(t, raw, ordinary))
		if err != nil {
			t.Fatal(err)
		}
		if bookingSlugReserved(slug) || slug != base64.RawURLEncoding.EncodeToString(ordinary) {
			t.Fatalf("%s: minted %q", reserved, slug)
		}
	}
}

func TestReservedBookingIDsMatchTheRouteOwner(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "frontend", "src", "lib", "bookingaddress.ts"))
	if err != nil {
		t.Fatal(err)
	}
	list := func(name string) []string {
		match := regexp.MustCompile(`(?s)export const ` + name + `[^=]*=\s*\[(.*?)\]`).FindSubmatch(raw)
		if match == nil {
			t.Fatalf("bookingaddress.ts no longer declares %s", name)
		}
		var out []string
		for _, quoted := range regexp.MustCompile(`"([^"]*)"`).FindAllSubmatch(match[1], -1) {
			out = append(out, string(quoted[1]))
		}
		return out
	}
	if got := list("BOOKING_HOST_IDS"); !slices.Equal(got, reservedBookingIDs) {
		t.Errorf("BOOKING_HOST_IDS %v, Go has %v", got, reservedBookingIDs)
	}
	if got := list("BOOKING_ID_PREFIXES"); !slices.Equal(got, reservedBookingPrefixes) {
		t.Errorf("BOOKING_ID_PREFIXES %v, Go has %v", got, reservedBookingPrefixes)
	}
}
