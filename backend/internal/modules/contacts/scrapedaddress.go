// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Ambiguous website addresses stay whole in registered_address. Only an
// unambiguous split may fill the company's structured address columns.

import (
	"regexp"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

var (
	// "12345 Example City", "1010 Wien", "D-12345 Berlin", "CH-8001 Zürich".
	postalThenCity = regexp.MustCompile(`^(?:[A-Z]{1,3}-)?(\d{4,5})\s+(\S.*)$`)
	// "Mountain View, CA 94043": the state and ZIP code follow the city.
	usStateZip = regexp.MustCompile(`^([A-Z]{2})\s+(\d{5}(?:-\d{4})?)$`)
	// "Thành phố Hồ Chí Minh 700000": a six-digit code follows the city.
	cityThenPostal = regexp.MustCompile(`^(\S.*?)\s+(\d{6})$`)
	postalToken    = regexp.MustCompile(`^(?:[A-Z]{1,3}-)?\d{4,5}$`)
	// "1012 AB Amsterdam": a Dutch postcode would read as a city named "AB Amsterdam".
	letterPostcodeTail = regexp.MustCompile(`^[A-Z]{2}\s`)
	anyDigit           = regexp.MustCompile(`\d`)
	addressSeparators  = strings.NewReplacer(",", "\n", ";", "\n", "|", "\n", " · ", "\n")
)

// countryByName answers a country written out in English, German or Vietnamese,
// in lower case, with the alpha-2 code the address column stores.
var countryByName = func() map[string]string {
	names := map[string]string{
		"usa": "US", "u.s.a.": "US", "united states of america": "US",
		"uk": "GB", "great britain": "GB", "viet nam": "VN",
	}
	namers := []display.Namer{display.English.Regions(), display.German.Regions(), display.Vietnamese.Regions()}
	for code := range assignedCountry {
		region, err := language.ParseRegion(code)
		if err != nil {
			continue
		}
		for _, namer := range namers {
			if name := namer.Name(region); name != "" {
				names[strings.ToLower(name)] = code
			}
		}
	}
	return names
}()

// addressSplitRule says how much a split must see before it is accepted.
type addressSplitRule uint8

const (
	// splitWhenReadable takes any street, postal code and city it can place.
	splitWhenReadable addressSplitRule = iota
	// splitWithStreetEvidence also needs the postal code set apart from the
	// street, by a separator or by a house number before it. So "Highway 1234
	// Dallas" stays whole. A split nobody reviews takes this rule.
	splitWithStreetEvidence
)

// SplitAddressText requires a street, postal code and city without dropping
// source text. An unplaced trailing part refuses the whole split.
func SplitAddressText(raw string) (crmcontracts.Address, bool) {
	return splitAddressBy(raw, splitWhenReadable)
}

func splitAddressBy(raw string, rule addressSplitRule) (crmcontracts.Address, bool) {
	var parts []string
	for part := range strings.SplitSeq(addressSeparators.Replace(raw), "\n") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	var country string
	if len(parts) > 1 {
		if code, named := addressCountry(parts[len(parts)-1]); named {
			country, parts = code, parts[:len(parts)-1]
		}
	}
	var out crmcontracts.Address
	var ok bool
	switch {
	case len(parts) == 1:
		out, ok = splitRunOnAddress(parts[0], country)
		ok = ok && (rule == splitWhenReadable || anyDigit.MatchString(*out.Line1))
	case len(parts) > 1:
		out, ok = splitSeparatedAddress(parts)
	}
	if !ok {
		return crmcontracts.Address{}, false
	}
	if country != "" {
		out.Country = &country
	}
	return out, true
}

// splitSeparatedAddress reads parts whose last one holds the postal code and
// city, the first one the street, and those between a second line.
func splitSeparatedAddress(parts []string) (crmcontracts.Address, bool) {
	last := parts[len(parts)-1]
	if m := usStateZip.FindStringSubmatch(last); m != nil && len(parts) >= 3 {
		if !placeName(parts[len(parts)-2]) {
			return crmcontracts.Address{}, false
		}
		out := streetLines(parts[:len(parts)-2])
		out.City, out.Region, out.PostalCode = new(parts[len(parts)-2]), new(m[1]), new(m[2])
		return out, true
	}
	if m := postalThenCity.FindStringSubmatch(last); m != nil {
		if !placeName(m[2]) {
			return crmcontracts.Address{}, false
		}
		out := streetLines(parts[:len(parts)-1])
		out.PostalCode, out.City = new(m[1]), new(m[2])
		return out, true
	}
	if m := cityThenPostal.FindStringSubmatch(last); m != nil && placeName(m[1]) {
		out := streetLines(parts[:len(parts)-1])
		out.City, out.PostalCode = new(m[1]), new(m[2])
		return out, true
	}
	return crmcontracts.Address{}, false
}

// splitRunOnAddress reads an address with no separators at all, such as
// "Example Street 1 Tower B 12345 Example City Germany". Multiple postal-code
// candidates make the split ambiguous.
func splitRunOnAddress(text, country string) (crmcontracts.Address, bool) {
	tokens := strings.Fields(text)
	at := -1
	for i, token := range tokens {
		if postalToken.MatchString(token) {
			if at >= 0 {
				return crmcontracts.Address{}, false
			}
			at = i
		}
	}
	if at <= 0 || at == len(tokens)-1 {
		return crmcontracts.Address{}, false
	}
	city := tokens[at+1:]
	if country == "" {
		city, country = trailingCountry(city)
	}
	name := strings.Join(city, " ")
	if !placeName(name) {
		return crmcontracts.Address{}, false
	}
	postal := strings.TrimLeft(tokens[at], "ABCDEFGHIJKLMNOPQRSTUVWXYZ-")
	out := crmcontracts.Address{Line1: new(strings.Join(tokens[:at], " ")), PostalCode: &postal, City: &name}
	if country != "" {
		out.Country = &country
	}
	return out, true
}

// trailingCountry takes the longest run of final tokens that names a country,
// and always leaves at least one token for the city.
func trailingCountry(tokens []string) ([]string, string) {
	for n := len(tokens) - 1; n >= 1; n-- {
		if code, named := addressCountry(strings.Join(tokens[n:], " ")); named {
			return tokens[:n], code
		}
	}
	return tokens, ""
}

func addressCountry(part string) (string, bool) {
	if _, assigned := assignedCountry[part]; assigned {
		return part, true
	}
	code, named := countryByName[strings.ToLower(part)]
	return code, named
}

func streetLines(parts []string) crmcontracts.Address {
	out := crmcontracts.Address{Line1: new(parts[0])}
	if len(parts) > 1 {
		out.Line2 = new(strings.Join(parts[1:], ", "))
	}
	return out
}

// Digits and lettered postcodes indicate an unresolved address part,
// rather than a city name.
func placeName(city string) bool {
	return city != "" && !anyDigit.MatchString(city) && !letterPostcodeTail.MatchString(city)
}
