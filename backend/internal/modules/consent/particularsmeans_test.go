// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

var threeMeans = []messagingrules.ContactMeans{
	messagingrules.MeansWeb, messagingrules.MeansEmail, messagingrules.MeansPhone,
}

func TestAnObjectionRouteNamesEachMeansItIsAskedFor(t *testing.T) {
	cases := []struct {
		text    string
		missing []messagingrules.ContactMeans
	}{
		{"trả lời email này với từ STOP, gửi email đến privacy@example.com, gọi số +84 00 0000 0000 " +
			"hoặc dùng trang https://example.com/tuy-chon", nil},
		{"Write to privacy@example.com or open zalo.me/acme, or call (030) 1234-5678.", nil},
		{
			"Reply STOP or write to privacy@example.com.",
			[]messagingrules.ContactMeans{messagingrules.MeansWeb, messagingrules.MeansPhone},
		},
		{
			"Open www.example.com or call +84 28 3822 9999.",
			[]messagingrules.ContactMeans{messagingrules.MeansEmail},
		},
		{
			"Write to privacy@example.com, open https://example.com/stop.",
			[]messagingrules.ContactMeans{messagingrules.MeansPhone},
		},
		// A date is not a number to call, and a link's digits are not either.
		{
			"Since 2026-11-16, see https://example.com/12345678 or privacy@example.com.",
			[]messagingrules.ContactMeans{messagingrules.MeansPhone},
		},
	}
	for _, c := range cases {
		if got := meansMissing(c.text, threeMeans); !slices.Equal(got, c.missing) {
			t.Errorf("meansMissing(%q) = %v, want %v", c.text, got, c.missing)
		}
	}
}

// The field error names the version and the part that is missing, so the
// settings form can put it under the right box.
func TestARouteWithoutAPhoneIsRefusedNamingThePhone(t *testing.T) {
	route := messagingrules.Disclosure{Kind: messagingrules.ObjectionRoute, Means: threeMeans}
	p := ControllerParticulars{
		ObjectionRoute: "Write to privacy@example.com or open https://example.com/stop.",
		Translations: []ParticularsTranslation{{
			Language: "vi", ObjectionRoute: "gửi email đến privacy@example.com hoặc dùng trang https://example.com/stop",
		}},
	}
	versions := [][]particularField{particularFields(p, "")}
	err := checkMeansStated(route, versions)
	var field *ValidationError
	if !errors.As(err, &field) || field.Field != "objection_route" || !strings.Contains(field.Reason, "phone number") {
		t.Fatalf("a route with no phone answered %v, want a field error on objection_route naming the phone", err)
	}

	p.ObjectionRoute += " Call +49 30 1234 5678."
	versions = [][]particularField{particularFields(p, ""), particularFields(p.Translations[0].asParticulars(), "vi")}
	err = checkMeansStated(route, versions)
	if !errors.As(err, &field) || field.Field != "translations.vi.objection_route" {
		t.Fatalf("the Vietnamese route with no phone answered %v, want a field error on its own field", err)
	}

	p.Translations[0].ObjectionRoute += ", gọi số +84 00 0000 0000"
	versions = [][]particularField{particularFields(p, ""), particularFields(p.Translations[0].asParticulars(), "vi")}
	if err := checkMeansStated(route, versions); err != nil {
		t.Errorf("complete routes were refused: %v", err)
	}
}

// A field left empty may be saved; the send refuses the mail that owes it.
func TestAnEmptyRouteIsNotRefusedAtSave(t *testing.T) {
	route := messagingrules.Disclosure{Kind: messagingrules.ObjectionRoute, Means: threeMeans}
	if err := checkMeansStated(route, [][]particularField{particularFields(ControllerParticulars{}, "")}); err != nil {
		t.Errorf("an empty route was refused: %v", err)
	}
}

func TestATranslationMustBeALanguageTheMailCatalogCarries(t *testing.T) {
	for _, language := range []string{"xx", "VI", ""} {
		p := ControllerParticulars{Translations: []ParticularsTranslation{{Language: language}}}
		if err := validateControllerParticulars(p); err == nil {
			t.Errorf("translation language %q was accepted", language)
		}
	}
	twice := ControllerParticulars{Translations: []ParticularsTranslation{{Language: "vi"}, {Language: "vi"}}}
	if err := validateControllerParticulars(twice); err == nil {
		t.Error("two Vietnamese versions were accepted")
	}
	blank := ControllerParticulars{Translations: []ParticularsTranslation{{Language: "vi", ObjectionRoute: "  "}}}
	if err := validateControllerParticulars(blank); err == nil {
		t.Error("a translated route of only whitespace was accepted")
	}
	if err := validateControllerParticulars(ControllerParticulars{
		Translations: []ParticularsTranslation{{Language: "vi", LegalName: "Công ty Ví dụ"}},
	}); err != nil {
		t.Errorf("an ordinary translation was refused: %v", err)
	}
}

// The phone digit range is the one values.E164Pattern allows.
func TestThePhoneDigitRangeIsTheE164Range(t *testing.T) {
	e164 := regexp.MustCompile(values.E164Pattern)
	for digits := 1; digits <= 20; digits++ {
		number := "+1" + strings.Repeat("0", digits-1)
		inRange := digits >= phoneMinDigits && digits <= phoneMaxDigits
		if e164.MatchString(number) != inRange {
			t.Errorf("%d digits: E.164 allows %v, this check allows %v", digits, e164.MatchString(number), inRange)
		}
	}
}
