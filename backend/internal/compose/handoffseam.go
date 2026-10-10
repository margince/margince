// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam behind prepare_handoff. It reads the project page's own service
// (compose/project360). The handoff and GET /projects/{id}/360 therefore show
// one project from one transaction, cut at the same row, under the same grants.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/project360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// handoffSections are the page sections a handoff reads. Its other sections
// (health, contracts, the timeline) are not part of the briefing.
var handoffSections = []crmcontracts.Project360Section{
	crmcontracts.Project360SectionDeals,
	crmcontracts.Project360SectionStakeholders,
	crmcontracts.Project360SectionCommitments,
}

// handoffReader assembles one project's handover material.
//
// A list the caller holds no grant for is left out and named in
// SectionsOmitted, as on the page. The project read is mandatory, and its
// refusal is returned unchanged. A contact the caller may not read is a seat
// with no name, not a failed handoff.
func handoffReader(pool *pgxpool.Pool) agents.HandoffReader {
	page := newProject360Service(pool)
	seats := identity.NewService(pool)
	return func(ctx context.Context, projectID ids.UUID) (agents.HandoffFacts, error) {
		res, err := page.Read(ctx, ids.From[ids.ProjectKind](projectID), project360.Reading{
			Only: handoffSections,
			// A handoff lists work somebody owes. The clock's own reminders are
			// on the project and on its page, and are not a promise anyone made.
			ExcludeSystemMinted: true,
		})
		if err != nil {
			return agents.HandoffFacts{}, err
		}
		facts := handoffFacts(res)
		// The receiving side, named: an id restates the question. The owner is a
		// seat and not a record, so identity names it.
		if facts.Project.OwnerID != nil {
			named, err := seats.SeatNames(ctx, []ids.UserID{ids.From[ids.UserKind](*facts.Project.OwnerID)})
			if err != nil {
				return agents.HandoffFacts{}, err
			}
			facts.Project.OwnerName = named[*facts.Project.OwnerID]
		}
		return facts, nil
	}
}

// handoffFacts carries one page read across the seam. Every deal is carried,
// won or not. Which one counts as sold is the tool's judgement, and a filter
// here would hide an open pursuit from the reader.
func handoffFacts(res project360.Result) agents.HandoffFacts {
	page := res.Page
	facts := agents.HandoffFacts{AsOf: page.AsOf, Project: handoffProject(page.Project)}
	for _, section := range page.SectionsOmitted {
		if slices.Contains(handoffSections, section) {
			facts.SectionsOmitted = append(facts.SectionsOmitted, string(section))
		}
	}
	if page.Deals != nil {
		facts.DealsTruncated = page.Deals.Page.HasMore
		for _, d := range page.Deals.Data {
			facts.Deals = append(facts.Deals, agents.HandoffDeal{
				DealID: ids.UUID(d.Id), Name: d.Name, Status: string(d.Status),
				AmountMinor: d.AmountMinor, Currency: d.Currency,
			})
		}
	}
	if page.Stakeholders != nil {
		facts.StakeholdersTruncated = page.Stakeholders.Page.HasMore
		for _, s := range page.Stakeholders.Data {
			seat := agents.HandoffStakeholder{ContactID: ids.UUID(s.ContactId)}
			if s.ContactName != nil {
				seat.Name = *s.ContactName
			}
			if s.Role != nil {
				seat.Role = *s.Role
			}
			facts.Stakeholders = append(facts.Stakeholders, seat)
		}
	}
	if page.Commitments != nil {
		facts.CommitmentsTruncated = page.Commitments.Page.HasMore
		facts.OpenCommitments = asCommitments(res.Tasks)
	}
	return facts
}
