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
	regexp.MustCompile(`^Vào\s.{1,200}\sđã viết:\s*$`),
	regexp.MustCompile(`^Ngày\s.{1,200}\sđã viết:\s*$`),
	regexp.MustCompile(`(?i)^-{2,}\s*(Original Message|Ursprüngliche Nachricht|Thư gốc|Tin nhắn gốc)`),
	regexp.MustCompile(`^_{5,}\s*$`),
}

// forwardPatterns open mail the sender passes on. It is new to the reader, so
// no reading folds it away as history.
var forwardPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^-{2,}\s*(Forwarded message|Weitergeleitete Nachricht|Thư được chuyển tiếp|Thư đã chuyển tiếp|Tin nhắn được chuyển tiếp)`),
	regexp.MustCompile(`(?i)^Begin forwarded message:\s*$`),
	regexp.MustCompile(`(?i)^(Thư được chuyển tiếp|Thư đã chuyển tiếp|Tin nhắn được chuyển tiếp):\s*$`),
	regexp.MustCompile(`(?i)^Anfang der weitergeleiteten (E-Mail|Nachricht)`),
}

// isReply reports whether a line introduces quoted earlier mail.
func isReply(line string) bool {
	return matchesAny(attributionPatterns, line)
}

// isForward reports whether a line opens mail the sender passes on.
func isForward(line string) bool {
	return matchesAny(forwardPatterns, line)
}

// IsAttribution reports whether a line introduces quoted or forwarded mail.
func IsAttribution(line string) bool {
	return isReply(line) || isForward(line)
}

func matchesAny(patterns []*regexp.Regexp, line string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}
