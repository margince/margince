// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam behind review_commitments, and the row carries prepare_handoff
// shares with it (handoffseam.go reads the rest).
//
// It is assembled here because it crosses modules. An open promise is an
// activity row or a claim read off a conversation, and a module never imports
// a sibling (ADR-0054). Every read below is a module's own gated store
// path, so object RBAC and row scope apply as they do on the HTTP surface.
// There is no raw SQL here and no second spelling of a filter.
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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// commitmentAboutContact is the entity type a promise's "about" row carries when
// the record it names is a contact.
//
// Its own constant rather than flipsource.go's entityContact: that one names
// an object in the system-of-record FLIP, and borrowing it here would tie two
// vocabularies together that are free to move apart.
const commitmentAboutContact = "contact"

// commitmentLister serves the open-promise set from the activities module's
// own gated read.
func commitmentLister(pool *pgxpool.Pool) agents.CommitmentLister {
	db := InstallationDB(pool)
	tasks := activities.NewStore(db)
	claims := contacts.NewStore(db)
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
			// This sweep answers "are we behind on our promises", and a
			// reminder the clock minted is nobody's promise. The record
			// pages' moment cards hold the same line, so the tool and the
			// screen cannot disagree about what is owed.
			ExcludeSystemMinted: true,
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
// message filed against a contact — so "this owner's promises" and "this
// project's promises" are questions the claim table cannot answer, and a claim
// admitted under either filter would be a row the caller did not ask for.
// Returning none is the honest answer; the tool's own copy says the narrowed
// answer covers recorded tasks alone.
func extractedCommitments(
	ctx context.Context, pool *pgxpool.Pool, store *contacts.Store,
	in agents.CommitmentQuery, limit int,
) ([]contacts.CompanyCommitment, bool, error) {
	if in.AssigneeID != nil || in.WithinProjectID != nil {
		return nil, false, nil
	}
	var out []contacts.CompanyCommitment
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
func asExtracted(claims []contacts.CompanyCommitment) []agents.OpenCommitment {
	out := make([]agents.OpenCommitment, 0, len(claims))
	for _, claim := range claims {
		claimID, activityID := claim.ID, claim.ActivityID
		out = append(out, agents.OpenCommitment{
			Source: agents.CommitmentFromConversation,
			// A claim names no owner: it records what was said, not who was
			// handed it, so the answer says nobody owns it rather than naming
			// the contact it was promised TO as though they owed it.
			ClaimID: &claimID, SourceActivityID: &activityID,
			Quote: claim.SourceQuote, Subject: claim.Body, DueAt: claim.DueAt,
			FiledAt: claim.OccurredAt,
			About: []agents.CommitmentAbout{{
				EntityType: commitmentAboutContact,
				EntityID:   claim.ContactID.UUID, Name: claim.ContactName,
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

// handoffProject carries the project row across the seam.
func handoffProject(p crmcontracts.Project) agents.HandoffProject {
	out := agents.HandoffProject{
		ProjectID: ids.UUID(p.Id),
		Name:      p.Name,
		CompanyID: (*ids.UUID)(p.CompanyId),
		OwnerID:   (*ids.UUID)(p.OwnerId),
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
