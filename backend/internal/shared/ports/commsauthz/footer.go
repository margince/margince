// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// WithFooterFindings keeps rendering omissions beside the permission decision.
func (d Decision) WithFooterFindings(findings []FooterFinding) Decision {
	d.Evidence.FooterFindings = findings
	if len(findings) > 0 && d.Verdict == VerdictAllow {
		d.Verdict = VerdictReview
		d.ReasonCode = ReasonFooterIncomplete
	}
	return d
}
