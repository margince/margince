// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailquote identifies reply and forwarding boundaries shared by mail readers.
package mailquote

import "regexp"

// attributionPatterns introduce a quoted reply. Bounded repetition keeps a
// pathological body from backtracking.
var attributionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^On\s.{1,200}\swrote:\s*$`),
	regexp.MustCompile(`^Am\s.{1,200}\sschrieb\s?.{0,120}:\s*$`),
	regexp.MustCompile(`(?i)^-{2,}\s*(Original Message|Ursprüngliche Nachricht|Forwarded message|Weitergeleitete Nachricht|Thư được chuyển tiếp|Tin nhắn được chuyển tiếp)`),
	regexp.MustCompile(`(?i)^Begin forwarded message:\s*$`),
	regexp.MustCompile(`(?i)^(Thư được chuyển tiếp|Tin nhắn được chuyển tiếp):\s*$`),
	regexp.MustCompile(`(?i)^Anfang der weitergeleiteten (E-Mail|Nachricht)`),
	regexp.MustCompile(`^_{5,}\s*$`),
}

// IsAttribution reports whether a line introduces quoted or forwarded mail.
func IsAttribution(line string) bool {
	for _, pattern := range attributionPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}
