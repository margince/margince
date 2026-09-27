// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type namedBookingBrand string

func (b namedBookingBrand) PublicBookingBrand(context.Context) (string, *string, error) {
	return string(b), nil, nil
}

func TestBookingCompanyNameKeepsThePublishedCharacterLimit(t *testing.T) {
	for _, name := range []string{"Example company", strings.Repeat("界", 201)} {
		t.Run(name[:min(len(name), 12)], func(t *testing.T) {
			store := (&Store{}).WithSchedulingBrand(namedBookingBrand(name))
			profile := store.brandSchedulingProfile(context.Background(), crmcontracts.SchedulingProfile{})
			if profile.CompanyName == nil {
				t.Fatal("company name absent")
			}
			got := *profile.CompanyName
			if !utf8.ValidString(got) || utf8.RuneCountInString(got) > 200 {
				t.Fatalf("name violates character contract: %q", got)
			}
			if utf8.RuneCountInString(name) <= 200 && got != name {
				t.Fatalf("short name changed: %q", got)
			}
			if utf8.RuneCountInString(name) > 200 && got != strings.Repeat("界", 199)+"…" {
				t.Fatalf("long name not abbreviated visibly: %q", got)
			}
		})
	}
}
