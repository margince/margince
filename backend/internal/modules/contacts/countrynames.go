// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/margince/margince/backend/internal/platform/namematch"
)

// countryNameLanguages are the languages the app speaks. The names come from the
// platform's region table, as the frontend's picker does, so no list of
// country names is typed here.
var countryNameLanguages = []language.Tag{language.English, language.German, language.Vietnamese}

// countryNameAliases are spellings a spreadsheet uses that no region table
// lists as the country's name.
var countryNameAliases = map[string]string{
	"usa": "US", "u.s.a.": "US", "united states of america": "US", "america": "US",
	"uk": "GB", "great britain": "GB", "england": "GB",
	"czech republic": "CZ", "holland": "NL", "south korea": "KR",
	"russian federation": "RU", "viet nam": "VN", "ivory coast": "CI",
	"vereinigte staaten von amerika": "US", "grossbritannien": "GB",
}

// countryByName holds each folded country name against its code. A name two
// countries share holds "" so a lookup refuses it instead of guessing.
var countryByName = func() map[string]string {
	byName := map[string]string{}
	add := func(name, code string) {
		key := namematch.Fold(name)
		if key == "" {
			return
		}
		if held, seen := byName[key]; seen && held != code {
			byName[key] = ""
			return
		}
		byName[key] = code
	}
	for _, code := range assignedCountryCodes {
		region, err := language.ParseRegion(code)
		if err != nil {
			continue
		}
		for _, lang := range countryNameLanguages {
			add(display.Regions(lang).Name(region), code)
		}
	}
	for name, code := range countryNameAliases {
		add(name, code)
	}
	return byName
}()

// ResolveCountry reads a country as typed: an assigned alpha-2 code, or
// the name of one country in English, German or Vietnamese. It returns the
// upper-case code, and false for anything unknown or shared by two countries.
// NormalizeCountryCode stays the stricter rule the stores apply.
func ResolveCountry(raw string) (string, bool) {
	if code, err := NormalizeCountryCode(raw); err == nil {
		return code, true
	}
	return countryNamed(raw)
}

// countryNamed answers a country written out as a name, and false for a name
// that is unknown or shared by two countries.
func countryNamed(name string) (string, bool) {
	code := countryByName[namematch.Fold(strings.TrimSpace(name))]
	return code, code != ""
}
