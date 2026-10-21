// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPreviouslyAdmittedClaimsNeedCurrentSourceOrHumanAdoption(t *testing.T) {
	for _, kind := range []string{"commitment_ours", "commitment_theirs"} {
		states := []string{"old", "renewed", "manual"}
		if kind == "commitment_ours" {
			states = append(states, "edited task", "pinned task")
		}
		for _, state := range states {
			t.Run(kind+"/"+state, func(t *testing.T) {
				e := integration.Setup(t)
				now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
				old := now.AddDate(-2, 0, 0)
				e.Contacts.WithClock(func() time.Time { return now })
				contact := ids.From[ids.ContactKind](e.SeedContact(t, "Promise contact", &e.AdminUser))
				company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Promise account", &e.AdminUser))
				if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contact, CompanyID: &company, IsCurrentPrimary: new(true), Source: "manual"}); err != nil {
					t.Fatal(err)
				}
				quote := "I will send the account plan."
				source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &old, MeetingStatus: new("held"), Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
				actor := principal.SystemActing(e.Admin(), "system:signal_scan")
				if state == "manual" {
					actor = e.Admin()
				}
				claim := e.RecordClaim(actor, t, contacts.ClaimInput{ContactID: contact, Kind: kind, Body: quote, ActivityID: sourceID.UUID, Quote: quote, DueAt: &old, Source: "manual", Locator: "account-plan"})
				decision := crmcontracts.CommitmentAdmission{Locator: "account-plan", Summary: quote, SourceQuote: quote, CurrentWork: true, RuleVersion: activities.CommitmentAdmissionVersion, EvidenceIds: []crmcontracts.Id{source.Id}, EvaluatedAt: old, Meaning: &crmcontracts.CommitmentMeaning{State: "open", DeadlinePrecision: "none"}}
				if state == "renewed" {
					renewed, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &now, MeetingStatus: new("held"), Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
					decision.RenewedBy = &renewed.Id
					decision.RenewedAt = &now
					decision.EvaluatedAt = now
				}
				if err := e.DB().Tx(actor, func(tx pgx.Tx) error { return activities.RecordCommitmentAdmissionTx(actor, tx, sourceID, decision) }); err != nil {
					t.Fatal(err)
				}
				adopted := state == "edited task" || state == "pinned task"
				if adopted {
					adoptHistoricalClaimTask(t, e, claim, sourceID, state)
				}
				if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
					history, err := e.Contacts.ClaimsForContact(e.Admin(), tx, contact, nil, 20)
					if err != nil {
						return err
					}
					want := state != "old"
					if len(history) != 1 || history[0].Id != claim.Id || history[0].NeedsReview == want {
						t.Errorf("state=%s history=%+v", state, history)
					}
					current, err := e.Contacts.OpenCommitmentsForContact(e.Admin(), tx, contact, nil, 20)
					if err != nil {
						return err
					}
					if (len(current) == 1) != (want && kind == "commitment_ours") {
						t.Errorf("state=%s current contact=%+v", state, current)
					}
					readCompany := e.Contacts.OpenCommitmentsForCompany
					if kind == "commitment_theirs" {
						readCompany = e.Contacts.OpenTheirCommitmentsForCompany
					}
					companyRows, complete, err := readCompany(e.Admin(), tx, company.UUID, 20)
					if err != nil {
						return err
					}
					if (len(companyRows) == 1) != (want && !adopted) || !complete {
						t.Errorf("state=%s company=%+v complete=%v", state, companyRows, complete)
					}
					count, readable, err := e.Contacts.CountAccountCommitments(e.Admin(), tx, company.UUID)
					if err == nil && ((count == 1) != (want && !adopted) || !readable) {
						t.Errorf("state=%s count=%d readable=%v", state, count, readable)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func adoptHistoricalClaimTask(t *testing.T, e *integration.Env, claim crmcontracts.ConversationClaim, source ids.ActivityID, state string) {
	t.Helper()
	actor := principal.SystemActing(e.Admin(), "system:signal_scan")
	assignee := ids.From[ids.UserKind](e.AdminUser)
	task, _, err := e.Activities.LogActivity(actor, activities.LogActivityInput{Kind: "task", Subject: new("Send the account plan"), Source: "manual", SourceActivityID: &source.UUID, SourceSystem: new(activities.CommitmentTaskSource), SourceID: new("account-plan"), AssigneeID: &assignee})
	if err != nil {
		t.Fatal(err)
	}
	taskID := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	if state == "edited task" {
		_, err = e.Activities.UpdateActivity(e.Admin(), taskID, activities.UpdateActivityInput{Subject: new("My account plan")})
	} else {
		err = e.Activities.PinWorklistRow(e.Admin(), activities.WorklistRowRef{Source: "task", RowID: taskID.String()})
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return e.Contacts.SetClaimTaskTx(e.Admin(), tx, ids.UUID(claim.Id), taskID.UUID)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticCommitmentPinFollowsAssigneeAcrossViews(t *testing.T) {
	for _, pinByAssignee := range []bool{true, false} {
		t.Run(map[bool]string{true: "assignee", false: "manager"}[pinByAssignee], func(t *testing.T) {
			e := integration.Setup(t)
			now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
			old := now.AddDate(-2, 0, 0)
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", OccurredAt: &old, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			assignee := ids.From[ids.UserKind](e.Rep1)
			task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:signal_scan"), activities.LogActivityInput{Kind: "task", Subject: new("Send plan"), Source: "manual", SourceSystem: new(activities.CommitmentTaskSource), SourceActivityID: new(ids.UUID(source.Id)), SourceID: new("old-plan"), AssigneeID: &assignee})
			if err != nil {
				t.Fatal(err)
			}
			rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			pinner := e.Admin()
			if pinByAssignee {
				pinner = rep
			}
			if err := e.Activities.PinWorklistRow(pinner, activities.WorklistRowRef{Source: "task", RowID: task.Id.String()}); err != nil {
				t.Fatal(err)
			}
			feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
			mine, err := feed.Worklist(rep, "mine", "all", ids.Nil, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			selected, err := feed.Worklist(e.Admin(), "all", "all", e.Rep1, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, page := range []crmcontracts.Worklist{mine, selected} {
				found := false
				for _, row := range page.Queue {
					if row.Id == task.Id.String() {
						found = true
					}
				}
				if found != pinByAssignee {
					t.Errorf("assignee pin=%v found=%v", pinByAssignee, found)
				}
			}
		})
	}
}
