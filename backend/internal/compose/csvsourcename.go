// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "strings"

// suggestedSourceName is the source name the upload proposes for a ZIP. It is
// built to pass sourceNameFrom, so a name the screen pre-fills is never refused.
func suggestedSourceName(fileName string) string {
	stem := fileName
	if strings.HasSuffix(strings.ToLower(stem), ".zip") {
		stem = stem[:len(stem)-len(".zip")]
	}
	name := strings.Map(func(r rune) rune {
		if sourceNamePattern.MatchString(string(r)) {
			return r
		}
		return '-'
	}, stem)
	if len(name) > maxSourceName {
		name = name[:maxSourceName]
	}
	return name
}
