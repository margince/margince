// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// MailCoverage retains Connections' own-reader boundary and never returns account details.
func (r *Registry) MailCoverage(ctx context.Context, providers []string, from *time.Time, at time.Time) (crmcontracts.MailCaptureCoverage, error) {
	views, err := r.connectionViews(ctx, false)
	if err != nil {
		return crmcontracts.MailCaptureCoverage{}, err
	}
	return mailCoverageOf(views, providers, from, at), nil
}

func mailCoverageOf(views []ConnectionView, providers []string, from *time.Time, at time.Time) crmcontracts.MailCaptureCoverage {
	out := crmcontracts.MailCaptureCoverage{
		Channel: crmcontracts.MailCaptureCoverageChannelEmail, State: crmcontracts.MailCaptureCoverageStateUnknown,
		IntervalFrom: from, IntervalTo: at, EvaluatedAt: at,
	}
	var seen, missing, unavailable bool
	for _, view := range views {
		if !slices.Contains(providers, view.Provider) {
			continue
		}
		seen = true
		unavailable = unavailable || view.Status != statusConnected || view.LastErrorClass != nil
		if view.LastSuccessAt == nil || view.LastSuccessAt.After(at) {
			missing = true
			continue
		}
		if out.LastSuccessAt == nil || view.LastSuccessAt.Before(*out.LastSuccessAt) {
			out.LastSuccessAt = view.LastSuccessAt
		}
	}
	if missing {
		out.LastSuccessAt = nil
	}
	switch {
	case unavailable:
		out.State = crmcontracts.MailCaptureCoverageStateUnavailable
	case seen && !missing:
		// Success and a completed import do not attest an uninterrupted Inbox and Sent interval.
		out.State = crmcontracts.MailCaptureCoverageStatePartial
	}
	return out
}
