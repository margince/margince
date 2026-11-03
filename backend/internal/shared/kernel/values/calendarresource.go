// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strconv"
	"strings"
)

const calendarResourceDomain = "resource.calendar.google.com"

// IsCalendarResourceDomain identifies folded calendar room and device domains.
func IsCalendarResourceDomain(domain string) bool {
	return domain == calendarResourceDomain || strings.HasSuffix(domain, "."+calendarResourceDomain)
}

// CalendarResourceDomainSQL accepts a compile-time folded domain expression.
func CalendarResourceDomainSQL(domain string) string {
	return "(" + domain + " = '" + calendarResourceDomain + "' OR right(" + domain + ", " + strconv.Itoa(len(calendarResourceDomain)+1) + ") = '." + calendarResourceDomain + "')"
}
