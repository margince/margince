// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"strings"
	"testing"
)

func TestABookingAliasIsNormalizedAndValidatedOnTheServer(t *testing.T) {
	if got := normalizeBookingAlias("  Alex-Sales "); got != "alex-sales" {
		t.Fatalf("normalized to %q", got)
	}
	for raw, code := range map[string]string{
		"al":                            "too_short",
		strings.Repeat("a", 31):         "too_long",
		"alex.sales":                    "invalid",
		"alex_sales":                    "invalid",
		"-alex":                         "invalid",
		"alex-":                         "invalid",
		"al ex":                         "invalid",
		"älex":                          "invalid",
		"al%2Fex":                       "invalid",
		"preview":                       "reserved",
		"contact":                       "reserved",
		"manage-me":                     "reserved",
		"proposal-x":                    "reserved",
		"meeting-room":                  "reserved",
		"contact-us":                    "reserved",
		"company-logo":                  "reserved",
		"alex":                          "",
		"a1b":                           "",
		strings.Repeat("a", 30):         "",
		"Alex-Sales":                    "",
		"x9-" + strings.Repeat("b", 27): "",
	} {
		err := validateBookingAlias(normalizeBookingAlias(raw))
		var refusal *SchedulingArgumentError
		switch {
		case code == "" && err != nil:
			t.Errorf("%q refused: %v", raw, err)
		case code != "" && (!errors.As(err, &refusal) || refusal.Code != code || refusal.Field != "alias"):
			t.Errorf("%q: %v, want %s", raw, err, code)
		}
	}
}

func TestNoAliasCanEqualARandomSlug(t *testing.T) {
	slug, err := mintBookingSlug(func(buf []byte) (int, error) { return len(buf), nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(slug) <= bookingAliasMax {
		t.Fatalf("a %d-character slug fits in a %d-character alias", len(slug), bookingAliasMax)
	}
}
