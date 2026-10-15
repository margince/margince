// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"regexp"
	"sync"
)

// compiledPatterns holds every pattern a gate builds from a name at run time.
// Keyed by the full pattern text, which is all a compiled pattern depends on.
var compiledPatterns sync.Map // pattern -> *regexp.Regexp

// compiledPattern compiles pattern once per run. Its callers build one per
// declaration or statement, so the same text comes back thousands of times.
func compiledPattern(pattern string) *regexp.Regexp {
	if cached, ok := compiledPatterns.Load(pattern); ok {
		if re, isRegexp := cached.(*regexp.Regexp); isRegexp {
			return re
		}
	}
	re := regexp.MustCompile(pattern)
	compiledPatterns.Store(pattern, re)
	return re
}
