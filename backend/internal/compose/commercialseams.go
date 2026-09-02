// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seams behind review_commitments and prepare_handoff.
//
// Both are assembled HERE because both cross modules: an open promise is an
// activity row, a handover is a project plus the deals rolled up to it plus
// the people attached to it plus those same promises — and a module never
// imports a sibling (ADR-0054 §9). Every read below is a module's own gated
// store path, so object RBAC and row scope apply exactly as they do on the
// HTTP surface; there is no raw SQL here and no second spelling of a filter.
//
// THE CLOCK LIVES ON THIS SIDE OF THE SEAM. The tools judge a promise as
// overdue or upcoming, and that judgement is a pure function of the due date
// and an instant. Reading the instant here — ONCE per call, and passing it
// across — is what makes the tool's derivation reproducible in a test with no
// clock in it, and what stops two rows in one answer being judged against two
// different nows.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// handoffScanLimit bounds each of the handover's list reads. A handover is a
// briefing rather than an export.
//
// EVERY BOUND IS REPORTED. A briefing that quietly stopped at fifty would be
// fine for the lists — a reader can see there are fifty rows — and wrong for
// the GAPS, two of which are claims that something is ABSENT. Absence cannot
// be read off a truncated list, so whether each read hit its bound crosses the
// seam and the tool withholds the claims a bounded read cannot support.
const handoffScanLimit = 50

// commitmentAboutPerson is the entity type a promise's "about" row carries when
// the record it names is a contact.
//
// Its own constant rather than flipsource.go's flipObjectPerson: that one names
// an object in the system-of-record FLIP, and borrowing it here would tie two
// vocabularies together that are free to move apart.
const commitmentAboutPerson = "person"

// commitmentLister serves the open-promise set from the activities module's
// own gated read.
func commitmentLister(pool *pgxpool.Pool) agents.CommitmentLister {
	db := InstallationDB(pool)
	tasks := activities.NewStore(db)
	claims := people.NewStore(db)
	return func(ctx context.Context, in agents.CommitmentQuery) (agents.CommitmentSweep, error) {
		now := clockNow()
		// ONE bound for the merged answer, resolved here rather than left to
		// each source's own default. The two disagree — the task read defaults
		// to fifty and the claim read to two hundred — so a caller who omits
		// the argument would get whatever the two happened to add up to, and
		// be told nothing was cut.
		asked := agents.CommitmentSweepLimit(in.Limit)
		query := activities.ListOpenTasksInput{
			AssigneeID: in.AssigneeID, Limit: asked,
			// The bound cuts before the merge does, so the store has to keep
			// the promise the ranking is about rather than the oldest ones.
			MostRecentlySlippedFirst: true,
		}
		if in.WithinProjectID != nil {
			project := ids.From[ids.ProjectKind](*in.WithinProjectID)
			query.WithinProjectID = &project
		}
		filed, filedMore, err := tasks.ListOpenTasks(ctx, query)
		if err != nil {
			return agents.CommitmentSweep{}, err
		}
		said, saidMore, err := extractedCommitments(ctx, pool, claims, in, asked)
		if err != nil {
			return agents.CommitmentSweep{}, err
		}
		promises, cut := rankPromises(now, asCommitments(filed), asExtracted(said), asked)
		return agents.CommitmentSweep{
			AsOf:        now,
			Commitments: promises,
			Truncated:   filedMore || saidMore || cut,
		}, nil
	}
}

// extractedCommitments reads the promises made in conversations, or none where
// the caller narrowed to something a claim cannot answer.
//
// A CLAIM CARRIES NO ASSIGNEE and no project. It records what was SAID, in a
// message filed against a person — so "this owner's promises" and "this
// project's promises" are questions the claim table cannot answer, and a claim
// admitted under either filter would be a row the caller did not ask for.
// Returning none is the honest answer; the tool's own copy says the narrowed
// answer covers recorded tasks alone.
func extractedCommitments(
	ctx context.Context, pool *pgxpool.Pool, store *people.Store,
	in agents.CommitmentQuery, limit int,
) ([]people.OrgCommitment, bool, error) {
	if in.AssigneeID != nil || in.WithinProjectID != nil {
		return nil, false, nil
	}
	var out []people.OrgCommitment
	var more bool
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		out, more, err = store.OpenCommitmentsAcrossWorkspace(ctx, tx, limit)
		return err
	})
	return out, more, err
}

// rankPromises merges the two sources into ONE answer, ordered by the rule
// every surface uses, and cuts it to what the caller asked for.
//
// The merge is what makes this tool's answer the same answer the screens give.
// Read task-only, it reported a promise made in a meeting and never typed as
// absent — and a model told "these are the open commitments" repeats that as
// fact. Ranking them together in kernel/owedwork is the same ordering the
// contact and company cards apply, so an agent and a reader looking at one
// account agree about what is most overdue.
func rankPromises(now time.Time, filed, said []agents.OpenCommitment, limit int) ([]agents.OpenCommitment, bool) {
	items := make([]owedwork.Item, 0, len(filed)+len(said))
	for _, promise := range filed {
		items = append(items, owedwork.Item{
			Ref: promise, Source: owedwork.FromTask,
			DueAt: promise.DueAt, FiledAt: promise.FiledAt,
		})
	}
	for _, promise := range said {
		items = append(items, owedwork.Item{
			Ref: promise, Source: owedwork.FromClaim,
			DueAt: promise.DueAt, FiledAt: promise.FiledAt,
		})
	}
	out := make([]agents.OpenCommitment, 0, len(items))
	for _, item := range owedwork.Sorted(items, now) {
		promise, ok := item.Ref.(agents.OpenCommitment)
		if !ok {
			continue
		}
		out = append(out, promise)
	}
	if len(out) > limit {
		return out[:limit], true
	}
	return out, false
}

// asExtracted carries the claim rows across the seam. A rename and nothing
// else: the store decided which rows, and the tool decides what state each is
// in.
func asExtracted(claims []people.OrgCommitment) []agents.OpenCommitment {
	out := make([]agents.OpenCommitment, 0, len(claims))
	for _, claim := range claims {
		claimID, activityID := claim.ID, claim.ActivityID
		out = append(out, agents.OpenCommitment{
			Source: agents.CommitmentFromConversation,
			// A claim names no owner: it records what was said, not who was
			// handed it, so the answer says nobody owns it rather than naming
			// the person it was promised TO as though they owed it.
			ClaimID: &claimID, SourceActivityID: &activityID,
			Quote: claim.SourceQuote, Subject: claim.Body, DueAt: claim.DueAt,
			FiledAt: claim.OccurredAt,
			About: []agents.CommitmentAbout{{
				EntityType: commitmentAboutPerson,
				EntityID:   claim.PersonID.UUID, Name: claim.PersonName,
			}},
		})
	}
	return out
}

// asCommitments carries the store rows across the seam. It is a rename and
// nothing else: no filtering, no re-ordering, no judging — the store decided
// which rows, and the tool decides what each row's state is.
func asCommitments(tasks []activities.OpenTask) []agents.OpenCommitment {
	out := make([]agents.OpenCommitment, 0, len(tasks))
	for _, t := range tasks {
		taskID := t.ID
		about := make([]agents.CommitmentAbout, 0, len(t.About))
		for _, a := range t.About {
			about = append(about, agents.CommitmentAbout{
				EntityType: a.EntityType, EntityID: a.EntityID, Name: a.Name,
			})
		}
		out = append(out, agents.OpenCommitment{
			Source: agents.CommitmentFromTask,
			TaskID: &taskID, Subject: t.Subject, DueAt: t.DueAt,
			FiledAt:    t.CreatedAt,
			AssigneeID: t.AssigneeID, AssigneeName: t.AssigneeName, About: about,
		})
	}
	return out
}

// handoffReader assembles one project's handover material.
//
// FOUR GATED READS, NOT ONE JOIN. Each enforces its own object grant and its
// own row scope, and the two do different things here — a distinction worth
// stating, because the obvious reading of "each read is gated" is wrong for
// one of them:
//
//   - An OBJECT-grant denial FAILS the call. A caller with no `deal` read
//     grant does not get a handover with the deals left out; ListDeals
//     refuses, and that refusal is returned unchanged. Nothing is assembled
//     around a gate nobody asked for, because nothing is assembled at all.
//   - A ROW-SCOPE miss FILTERS. A deal owned outside the caller's scope is
//     simply not in the page, and the brief is assembled from what remains.
//     The gap messages say "this caller can see" for exactly that reason: the
//     honest claim is about what is visible, not about what exists.
//
// The project read runs FIRST and its refusal is returned unchanged, so a
// project outside the caller's scope answers not-found exactly as reading it
// directly would.
func handoffReader(pool *pgxpool.Pool) agents.HandoffReader {
	dealStore := deals.NewStore(InstallationDB(pool), DealsInstallation())
	projectStore := ProjectsStore(pool)
	peopleStore := people.NewStore(InstallationDB(pool))
	taskStore := activities.NewStore(InstallationDB(pool))
	seats := identity.NewService(pool)
	return func(ctx context.Context, projectID ids.UUID) (agents.HandoffFacts, error) {
		project, err := projectStore.GetProject(ctx,
			ids.From[ids.ProjectKind](projectID), storekit.LiveOnly)
		if err != nil {
			return agents.HandoffFacts{}, err
		}
		facts := agents.HandoffFacts{AsOf: clockNow(), Project: handoffProject(project)}
		// The receiving side, named. "Who owns this work now" answered as a
		// UUID restates the question — and unlike a stakeholder, the owner is a
		// SEAT rather than a record, so it is named through identity's own read
		// rather than the people store's.
		if facts.Project.OwnerID != nil {
			named, err := seats.SeatNames(ctx, []ids.UserID{ids.From[ids.UserKind](*facts.Project.OwnerID)})
			if err != nil {
				return agents.HandoffFacts{}, err
			}
			facts.Project.OwnerName = named[*facts.Project.OwnerID]
		}
		if facts.Deals, facts.DealsTruncated, err = handoffDeals(ctx, dealStore, projectID); err != nil {
			return agents.HandoffFacts{}, err
		}
		if facts.Stakeholders, facts.StakeholdersTruncated, err = handoffStakeholders(ctx, peopleStore, projectID); err != nil {
			return agents.HandoffFacts{}, err
		}
		projectType := string(datasource.RecordProject)
		tasks, truncated, err := taskStore.ListOpenTasks(ctx, activities.ListOpenTasksInput{
			EntityType: &projectType, EntityID: &projectID, Limit: handoffScanLimit,
		})
		if err != nil {
			return agents.HandoffFacts{}, err
		}
		facts.OpenCommitments, facts.CommitmentsTruncated = asCommitments(tasks), truncated
		return facts, nil
	}
}

// handoffProject carries the project row across the seam.
func handoffProject(p crmcontracts.Project) agents.HandoffProject {
	out := agents.HandoffProject{
		ProjectID:      ids.UUID(p.Id),
		Name:           p.Name,
		OrganizationID: (*ids.UUID)(p.OrganizationId),
		OwnerID:        (*ids.UUID)(p.OwnerId),
	}
	if p.Key != nil {
		out.Key = *p.Key
	}
	if p.Phase != nil {
		out.Phase = string(*p.Phase)
	}
	if p.Description != nil {
		out.Description = *p.Description
	}
	if p.StartedAt != nil {
		out.StartedAt = &p.StartedAt.Time
	}
	if p.TargetEndDate != nil {
		out.TargetEndDate = &p.TargetEndDate.Time
	}
	return out
}

// handoffDeals reads what is rolled up to the project. Every deal is carried,
// won or not: which of them counts as sold is the tool's judgement, and a
// filter here would hide the open ones from a reader who has to know a
// pursuit is still running.
func handoffDeals(ctx context.Context, store *deals.Store, projectID ids.UUID) ([]agents.HandoffDeal, bool, error) {
	limit := handoffScanLimit
	project := ids.From[ids.ProjectKind](projectID)
	rows, page, err := store.ListDeals(ctx, deals.ListDealsInput{ProjectID: &project, Limit: &limit})
	if err != nil {
		return nil, false, err
	}
	out := make([]agents.HandoffDeal, 0, len(rows))
	for _, d := range rows {
		out = append(out, agents.HandoffDeal{
			DealID: ids.UUID(d.Id), Name: d.Name, Status: string(d.Status),
			AmountMinor: d.AmountMinor, Currency: d.Currency,
		})
	}
	return out, page.HasMore, nil
}

// handoffStakeholders reads the people attached to the project, through the
// generic relationship list so the edge's own visibility rules apply.
//
// A seat is an id and a role, with no name — the same shape account_coverage
// answers a deal's stakeholder seats in. Naming them would need a gated
// person read per seat, and the caller already has read_record for the one
// they want to reach; the field a handover is judged on is the role, and that
// is here.
func handoffStakeholders(ctx context.Context, store *people.Store, projectID ids.UUID) ([]agents.HandoffStakeholder, bool, error) {
	limit := handoffScanLimit
	project := ids.From[ids.ProjectKind](projectID)
	kind := people.ProjectStakeholderKind
	edges, page, err := store.ListRelationships(ctx, people.ListRelationshipsInput{
		Kind: &kind, ProjectID: &project, Limit: &limit,
	})
	if err != nil {
		return nil, false, err
	}
	seated := make([]ids.PersonID, 0, len(edges))
	for _, e := range edges {
		if e.PersonID != nil {
			seated = append(seated, *e.PersonID)
		}
	}
	names, err := store.PersonNames(ctx, seated)
	if err != nil {
		return nil, false, err
	}
	out := make([]agents.HandoffStakeholder, 0, len(edges))
	for _, e := range edges {
		if e.PersonID == nil {
			continue
		}
		stakeholder := agents.HandoffStakeholder{
			PersonID: e.PersonID.UUID, Name: names[e.PersonID.UUID],
		}
		if e.Role != nil {
			stakeholder.Role = *e.Role
		}
		out = append(out, stakeholder)
	}
	return out, page.HasMore, nil
}
