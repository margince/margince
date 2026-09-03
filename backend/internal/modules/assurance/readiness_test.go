// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import "testing"

func checked(sources ...string) []SourceCoverage {
	out := make([]SourceCoverage, 0, len(sources))
	for _, s := range sources {
		out = append(out, SourceCoverage{Source: s, State: CoverageChecked})
	}
	return out
}

// The rule the whole vocabulary exists for: a pass that could not look must not
// say the pipeline is sound.
func TestReadinessNeverSaysReadyWithARequiredSourceMissing(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	for _, tc := range []struct {
		name     string
		coverage []SourceCoverage
	}{
		{"the mailbox could not be opened", []SourceCoverage{
			{Source: "mail", State: CoverageUnavailable},
			{Source: "offers", State: CoverageChecked},
		}},
		{"a source was permission-limited", []SourceCoverage{
			{Source: "mail", State: CoveragePermissionLimited},
			{Source: "offers", State: CoverageChecked},
		}},
		{"a source was stale", []SourceCoverage{
			{Source: "mail", State: CoverageStale},
			{Source: "offers", State: CoverageChecked},
		}},
		// Silence is not coverage. A run that forgot to record a source looks
		// identical to one that could not read it, and the safe reading of
		// both is that we do not know.
		{"a required source was never mentioned", checked("mail")},
		{"nothing was recorded at all", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// No findings at all — the case that would otherwise read as Ready,
			// which is exactly the misreading this rule prevents.
			if got := Readiness(tc.coverage, nil, cfg); got != ReadinessChecksIncomplete {
				t.Errorf("readiness was %q with a required source unread, want %q — a "+
					"manager told the pipeline is sound when nobody looked cannot act on it",
					got, ReadinessChecksIncomplete)
			}
		})
	}
}

func TestReadinessGradesWhatTheRunActuallyFound(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	full := checked("mail", "offers")
	small := int64(10)
	large := int64(500_000)

	for _, tc := range []struct {
		name     string
		findings []Finding
		want     string
	}{
		{"a clean pipeline", nil, ReadinessReady},
		{
			// Recorded and counted, but not worth interrupting somebody for.
			// Materiality changes severity and urgency, never coverage.
			name:     "only immaterial findings",
			findings: []Finding{{Severity: SeverityLow, AffectedMinor: &small}},
			want:     ReadinessReadyWithExceptions,
		},
		{
			name:     "a material finding",
			findings: []Finding{{Severity: SeverityLow, AffectedMinor: &large}},
			want:     ReadinessNeedsReview,
		},
		{
			// High severity is worth a look whatever the money says — a
			// committed deal with nobody who can sign has no amount attached to
			// the problem.
			name:     "a high-severity finding with no amount",
			findings: []Finding{{Severity: SeverityHigh}},
			want:     ReadinessNeedsReview,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Readiness(full, tc.findings, cfg); got != tc.want {
				t.Errorf("readiness was %q, want %q", got, tc.want)
			}
		})
	}
}
