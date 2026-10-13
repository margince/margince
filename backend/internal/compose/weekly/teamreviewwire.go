// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// addTeamWeekToWire puts the roster, quality, coverage, summary and the live
// lists on the wire. Each is omitted where the snapshot or the read has none,
// so an older week reads "not measured" rather than zeros.
func addTeamWeekToWire(out *crmcontracts.TeamWeeklyReview, review TeamReview) {
	if len(review.Members) > 0 {
		members := make([]crmcontracts.TeamWeeklyMember, 0, len(review.Members))
		for _, m := range review.Members {
			members = append(members, crmcontracts.TeamWeeklyMember{
				UserId: openapi_types.UUID(m.UserID), DisplayName: m.DisplayName, Measured: m.Measured,
			})
		}
		out.Members = &members
	}
	q := review.Quality
	if q.ProposalsAccepted != nil || q.ProposalsRejected != nil || q.AutoChangeReversals != nil {
		out.Quality = &crmcontracts.TeamWeeklyQuality{
			ProposalsAccepted: q.ProposalsAccepted, ProposalsRejected: q.ProposalsRejected,
			AutoChangeReversals: q.AutoChangeReversals,
		}
	}
	if len(review.Coverage) > 0 {
		coverage := make([]crmcontracts.TeamWeeklyCoverage, 0, len(review.Coverage))
		for _, c := range review.Coverage {
			coverage = append(coverage, coverageToWire(c))
		}
		out.Coverage = &coverage
		replan := review.ReplanRecommended()
		out.ReplanRecommended = &replan
	}
	out.Summary = nullableText(review.Summary)
	if review.Attention.Read {
		exceptions := review.Attention.Exceptions
		if exceptions == nil {
			exceptions = []crmcontracts.InputCheck{}
		}
		stale := make([]crmcontracts.TeamWeeklyStaleDeal, 0, len(review.Attention.StaleDeals))
		for _, d := range review.Attention.StaleDeals {
			stale = append(stale, crmcontracts.TeamWeeklyStaleDeal{
				DealId: openapi_types.UUID(d.DealID), DealName: d.DealName,
				OwnerId: openapi_types.UUID(d.OwnerID), LastActivityAt: d.LastActivityAt,
				DaysQuiet: d.DaysQuiet,
			})
		}
		out.Exceptions, out.StaleDeals = &exceptions, &stale
	}
}

// coverageToWire sends the figures only beside a basis, which is what the
// table's own CHECK guarantees about the row.
func coverageToWire(c Coverage) crmcontracts.TeamWeeklyCoverage {
	out := crmcontracts.TeamWeeklyCoverage{
		PeriodKind:        crmcontracts.TeamWeeklyCoveragePeriodKind(c.PeriodKind),
		BaseCurrency:      c.BaseCurrency,
		ReplanRecommended: c.ReplanRecommended,
	}
	if c.Absent != "" {
		absent := crmcontracts.TeamWeeklyCoverageAbsent(c.Absent)
		out.Absent = &absent
		return out
	}
	basis := crmcontracts.TeamWeeklyCoverageBasis(c.Basis)
	out.Basis = &basis
	out.ReferenceLandingMinor, out.NeededOpenMinor = &c.ReferenceLandingMinor, &c.NeededOpenMinor
	out.CurrentOpenMinor, out.RemainingToSupportMinor = &c.CurrentOpenMinor, &c.RemainingToSupportMinor
	out.CoverageBp = &c.CoverageBp
	return out
}

func patternsToWire(patterns []Pattern) []crmcontracts.TeamWeeklyPattern {
	out := make([]crmcontracts.TeamWeeklyPattern, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, crmcontracts.TeamWeeklyPattern{
			Kind: crmcontracts.TeamWeeklyPatternKind(p.Kind), Label: p.Label,
		})
	}
	return out
}

func funnelToWire(funnel []FunnelDeviation) *[]crmcontracts.TeamWeeklyFunnelDeviation {
	out := make([]crmcontracts.TeamWeeklyFunnelDeviation, 0, len(funnel))
	for _, f := range funnel {
		out = append(out, crmcontracts.TeamWeeklyFunnelDeviation{
			PipelineId: openapi_types.UUID(f.PipelineID), PipelineName: f.PipelineName,
			StageId: openapi_types.UUID(f.StageID), StageName: f.StageName,
			NextStageName: nullableText(f.NextStageName),
			RepEntered:    f.RepEntered, RepConverted: f.RepConverted,
			TeamEntered: f.TeamEntered, TeamConverted: f.TeamConverted,
			DeviationBp: f.DeviationBp,
		})
	}
	return &out
}
