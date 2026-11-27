// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"testing"
)

func TestACalendarAddressIsRecognisedByItsDomain(t *testing.T) {
	for domain, want := range map[string]bool{
		"resource.calendar.google.com":      true,
		"group.calendar.google.com":         true,
		"eu.group.calendar.google.com":      true,
		"calendar.google.com":               false,
		"notgroup.calendar.google.com":      false,
		"example.com":                       false,
		"group.calendar.google.com.example": false,
	} {
		if got := IsCalendarResourceDomain(domain); got != want {
			t.Errorf("IsCalendarResourceDomain(%q) = %v, want %v", domain, got, want)
		}
	}
}

// The SQL reading names the same domains as the Go reading.
func TestTheCalendarSQLNamesEveryCalendarDomain(t *testing.T) {
	sql := CalendarResourceDomainSQL("d")
	for _, domain := range calendarResourceDomains {
		if !strings.Contains(sql, "d = '"+domain+"'") || !strings.Contains(sql, "= '."+domain+"'") {
			t.Errorf("the SQL reading misses %s: %s", domain, sql)
		}
	}
}
