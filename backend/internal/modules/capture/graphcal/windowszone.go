// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Graph's event create and update are documented to take every Windows zone
// and only some IANA names. So a chosen IANA zone is sent as its Windows zone,
// by the Unicode CLDR mapping copied into data/ (see data/README.md).
var (
	//go:embed data/windowsZones.xml
	cldrWindowsZones []byte
	//go:embed data/timezone.xml
	cldrZoneAliases []byte
	//go:embed data/moved-windows-zones.txt
	cldrMovedZones []byte

	windowsZoneOf = sync.OnceValues(func() (map[string]string, error) {
		return windowsZones(cldrWindowsZones, cldrZoneAliases, cldrMovedZones)
	})
)

// windowsZone names the Windows zone that keeps an IANA zone's local clock,
// and false for a zone CLDR maps to none.
func windowsZone(iana string) (string, bool, error) {
	zones, err := windowsZoneOf()
	if err != nil {
		return "", false, err
	}
	windows, ok := zones[iana]
	return windows, ok, nil
}

// cldrMapping is windowsZones.xml read as the Windows zone of each IANA name
// it lists. CLDR lists each zone under its own canonical name, which is often
// the older one (Asia/Calcutta, not Asia/Kolkata).
func cldrMapping(mapping []byte) (map[string]string, error) {
	var windows struct {
		Zones []struct {
			Windows string `xml:"other,attr"`
			IANA    string `xml:"type,attr"`
		} `xml:"windowsZones>mapTimezones>mapZone"`
	}
	if err := xml.Unmarshal(mapping, &windows); err != nil {
		return nil, fmt.Errorf("graphcal: reading CLDR windowsZones.xml: %w", err)
	}
	byCanonical := map[string]string{}
	for _, zone := range windows.Zones {
		for canonical := range strings.FieldsSeq(zone.IANA) {
			if _, mapped := byCanonical[canonical]; !mapped {
				byCanonical[canonical] = zone.Windows
			}
		}
	}
	if len(byCanonical) == 0 {
		return nil, errors.New("graphcal: CLDR windowsZones.xml maps no zone")
	}
	return byCanonical, nil
}

// windowsZones maps every IANA name CLDR knows, aliases included, to its
// Windows zone. A zone in moved gets none. A later CLDR release gave it a new
// Windows zone because its rules changed, so the old name states a wrong clock.
func windowsZones(mapping, aliases, moved []byte) (map[string]string, error) {
	byCanonical, err := cldrMapping(mapping)
	if err != nil {
		return nil, err
	}
	var bcp47 struct {
		Types []struct {
			Alias string `xml:"alias,attr"`
		} `xml:"keyword>key>type"`
	}
	if err := xml.Unmarshal(aliases, &bcp47); err != nil {
		return nil, fmt.Errorf("graphcal: reading CLDR timezone.xml: %w", err)
	}
	sameZone := map[string][]string{}
	for _, zone := range bcp47.Types {
		names := strings.Fields(zone.Alias)
		for _, name := range names {
			sameZone[name] = names
		}
	}
	for canonical := range strings.FieldsSeq(string(moved)) {
		delete(byCanonical, canonical)
	}
	byIANA := map[string]string{}
	for canonical, windows := range byCanonical {
		for _, name := range append([]string{canonical}, sameZone[canonical]...) {
			byIANA[name] = windows
		}
	}
	return byIANA, nil
}
