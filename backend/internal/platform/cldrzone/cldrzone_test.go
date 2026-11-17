// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package cldrzone

import "testing"

func TestACountryWithOneZoneNamesIt(t *testing.T) {
	for country, want := range map[string]string{"vn": "Asia/Ho_Chi_Minh", "VN": "Asia/Ho_Chi_Minh", "at": "Europe/Vienna", "gb": "Europe/London"} {
		zone, ok, err := CountryZone(country)
		if err != nil || !ok || zone.String() != want {
			t.Errorf("CountryZone(%q) = %v, %v, %v; want %s", country, zone, ok, err, want)
		}
	}
}

func TestACountryWithSeveralZonesOrNoneNamesNone(t *testing.T) {
	// Germany lists Busingen beside Berlin, so it has two entries.
	for _, country := range []string{"us", "ru", "de", "", "zz", "ut", "un"} {
		if zone, ok, err := CountryZone(country); err != nil || ok {
			t.Errorf("CountryZone(%q) = %v, %v, %v; want no single zone", country, zone, ok, err)
		}
	}
}

func TestTheZoneListIsReadWhole(t *testing.T) {
	zones, err := singleZones()
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) < 150 {
		t.Fatalf("read %d single-zone countries, want the CLDR list's 150 or more", len(zones))
	}
}
