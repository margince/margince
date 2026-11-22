// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	_ "embed"
	"strings"
)

// siteChecksGolden is one line per shipped site, "task/variant digest". The
// digest covers the Go code its case's Evaluate reaches, which decides whether a
// reply is right. sitecheck_test.go derives it from the source and fails when the
// two differ, so an edit to a site's check moves the stamp of its records.
//
//go:embed testdata/site_checks.golden
var siteChecksGolden string

// siteChecks is siteChecksGolden read once.
var siteChecks = parseSiteChecks(siteChecksGolden)

func parseSiteChecks(golden string) map[string]string {
	checks := map[string]string{}
	for line := range strings.SplitSeq(golden, "\n") {
		if site, digest, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			checks[site] = digest
		}
	}
	return checks
}

// siteCheck is the digest of the check that grades task/site's replies. A site
// the golden does not name, as in a test's own census, reads empty; the gate
// fails a shipped site the golden misses.
func siteCheck(task, site string) string {
	return siteChecks[task+"/"+site]
}
