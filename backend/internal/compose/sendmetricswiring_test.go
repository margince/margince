// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
)

// The api's /metrics renders the send families, so the pilot can read them
// from the role that serves the composer.
func TestTheExpositionCarriesTheSendFamilies(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	Server{}.writeMetricsSections(&b)
	for _, family := range []string{
		"# TYPE margince_email_send_outcomes_total counter",
		"# TYPE margince_email_send_reviews_total counter",
		"# TYPE margince_email_send_anyway_total counter",
		"# TYPE margince_email_send_decision_seconds histogram",
	} {
		if !strings.Contains(b.String(), family) {
			t.Errorf("the exposition lacks %q", family)
		}
	}
}
