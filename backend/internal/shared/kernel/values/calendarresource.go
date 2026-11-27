// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strconv"
	"strings"
)

// calendarResourceDomains are the domains Google gives a calendar that is not
// a human: a booked room or device, and a shared group calendar.
var calendarResourceDomains = []string{"resource.calendar.google.com", "group.calendar.google.com"}

// IsCalendarResourceDomain identifies folded calendar room, device and shared
// calendar domains.
func IsCalendarResourceDomain(domain string) bool {
	for _, resource := range calendarResourceDomains {
		if domain == resource || strings.HasSuffix(domain, "."+resource) {
			return true
		}
	}
	return false
}

// CalendarResourceDomainSQL accepts a compile-time folded domain expression.
func CalendarResourceDomainSQL(domain string) string {
	clauses := make([]string, 0, len(calendarResourceDomains))
	for _, resource := range calendarResourceDomains {
		clauses = append(clauses, domain+" = '"+resource+"' OR right("+domain+", "+
			strconv.Itoa(len(resource)+1)+") = '."+resource+"'")
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
}
