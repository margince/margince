// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "slices"

// WithFooterFindings keeps rendering omissions beside the permission decision.
// Each finding carries the provision this recipient's pack cites for its kind.
// The list is copied: one footer is shared by every recipient, and each
// recipient's pack may cite other law.
func (d Decision) WithFooterFindings(findings []FooterFinding) Decision {
	cited := slices.Clone(findings)
	for i := range cited {
		cited[i].Provision = d.ProvisionFor(cited[i].Kind)
	}
	d.Evidence.FooterFindings = cited
	if len(findings) > 0 && d.Verdict == VerdictAllow {
		d.Verdict = VerdictReview
		d.ReasonCode = ReasonFooterIncomplete
	}
	return d
}
