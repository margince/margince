// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package cldrzone reads the Unicode CLDR time zone list (data/README.md): the
// IANA names of each zone, and the country each zone lies in.
package cldrzone

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TimezoneXML is CLDR's common/bcp47/timezone.xml, byte for byte.
//
//go:embed data/timezone.xml
var TimezoneXML []byte

// Zone is one CLDR zone entry: the IANA names it answers to, the preferred one
// first.
type Zone struct {
	// Country is the zone's ISO 3166-1 alpha-2 code in lower case. It is empty
	// for a zone in no country: UTC, the fixed offsets, the unknown zone.
	Country string
	Names   []string
}

// Zones reads every live entry; a deprecated one is left out, since it names
// the zone it was folded into.
func Zones(data []byte) ([]Zone, error) {
	var bcp47 struct {
		Types []struct {
			Name       string `xml:"name,attr"`
			Alias      string `xml:"alias,attr"`
			IANA       string `xml:"iana,attr"`
			Region     string `xml:"region,attr"`
			Deprecated string `xml:"deprecated,attr"`
		} `xml:"keyword>key>type"`
	}
	if err := xml.Unmarshal(data, &bcp47); err != nil {
		return nil, fmt.Errorf("cldrzone: reading CLDR timezone.xml: %w", err)
	}
	out := make([]Zone, 0, len(bcp47.Types))
	for _, entry := range bcp47.Types {
		if entry.Deprecated == "true" || entry.Alias == "" {
			continue
		}
		names := strings.Fields(entry.Alias)
		if entry.IANA != "" {
			names = append([]string{entry.IANA}, names...)
		}
		out = append(out, Zone{Country: countryOf(entry.Name, entry.Region, names[0]), Names: names})
	}
	return out, nil
}

// countryOf is the region CLDR states, else the first two letters of the BCP 47
// key, which CLDR builds from the country code. An Etc/ zone has none.
func countryOf(key, region, iana string) string {
	switch {
	case strings.HasPrefix(iana, "Etc/"):
		return ""
	case region != "":
		return strings.ToLower(region)
	case len(key) >= 2:
		return key[:2]
	}
	return ""
}

var singleZones = sync.OnceValues(func() (map[string]string, error) {
	zones, err := Zones(TimezoneXML)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	first := map[string]string{}
	for _, zone := range zones {
		if zone.Country == "" {
			continue
		}
		counts[zone.Country]++
		first[zone.Country] = zone.Names[0]
	}
	out := map[string]string{}
	for country, n := range counts {
		if n == 1 {
			out[country] = first[country]
		}
	}
	return out, nil
})

// CountryZone is the one zone a country lies in, and false for a country CLDR
// lists with more than one zone, or with none.
func CountryZone(country string) (*time.Location, bool, error) {
	zones, err := singleZones()
	if err != nil {
		return nil, false, err
	}
	name, ok := zones[strings.ToLower(strings.TrimSpace(country))]
	if !ok {
		return nil, false, nil
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return nil, false, fmt.Errorf("cldrzone: loading %s: %w", name, err)
	}
	return zone, true, nil
}
