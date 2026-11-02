// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var repairAdviceNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func createRepairAdviceCompany(t *testing.T, e *integration.Env, name string) ids.CompanyID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func logRepairAdviceMail(t *testing.T, e *integration.Env, company ids.CompanyID, direction, key string, at time.Time) ids.ActivityID {
	t.Helper()
	subject, body, address := "Proposal", "Let us discuss the proposal.", "buyer@counterparty.example"
	var links []activities.ActivityLinkInput
	if !company.IsZero() {
		links = []activities.ActivityLinkInput{{EntityType: "company", EntityID: company.UUID}}
	}
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at,
		CounterpartyEmail: address, CounterpartyOutboundAttested: direction == "outbound",
		ThreadKey: key, Source: "manual", Links: links,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(mail.Id))
}

func repairAdviceService(e *integration.Env) *company360.Service {
	return company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return repairAdviceNow })
}

func TestCompanyAdviceDoesNotFallBackPastAnUnavailableAnswer(t *testing.T) {
	e := integration.Setup(t)
	company := createRepairAdviceCompany(t, e, "Repair advice")
	other := createRepairAdviceCompany(t, e, "Other advice")
	logRepairAdviceMail(t, e, company, "outbound", "old-proposal", repairAdviceNow.AddDate(0, 0, -20))
	service := repairAdviceService(e)
	unanswered, err := service.UndismissedAdvice(e.Admin(), company)
	if err != nil {
		t.Fatal(err)
	}
	chase := false
	for _, suggestion := range unanswered {
		if suggestion.Kind == "no_reply" {
			chase = true
		}
	}
	if !chase {
		t.Fatal("the old outbound message must raise advice before its answer")
	}
	logRepairAdviceMail(t, e, company, "inbound", "new-answer", repairAdviceNow.AddDate(0, 0, -1))
	before, err := service.UndismissedAdvice(e.Admin(), company)
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range before {
		if suggestion.Kind == "no_reply" {
			t.Fatal("the visible answer did not settle the old proposal")
		}
	}
	run := beginUnavailableThread(t, e, "new-answer")
	if _, err := service.UndismissedAdvice(e.Admin(), company); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("advice during repair: %v", err)
	}
	if _, err := service.Assemble(e.Admin(), company); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("account state during repair: %v", err)
	}
	if _, err := service.Assemble(e.Admin(), other); err != nil {
		t.Fatalf("unrelated account: %v", err)
	}
	finishUnavailableThread(t, e, run)
	if _, err := service.Assemble(e.Admin(), company); err != nil {
		t.Fatalf("account after publication: %v", err)
	}
}

func TestCompanyAdviceWaitsForALinkedTasksUnavailableSource(t *testing.T) {
	e := integration.Setup(t)
	company := createRepairAdviceCompany(t, e, "Task evidence")
	source := logRepairAdviceMail(t, e, ids.CompanyID{}, "inbound", "unlinked-task-source", repairAdviceNow.AddDate(0, 0, -1))
	subject := "Answer the proposal"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", SourceActivityID: &source.UUID,
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
	service := repairAdviceService(e)
	if _, err := service.UndismissedAdvice(e.Admin(), company); err != nil {
		t.Fatal(err)
	}
	beginUnavailableThread(t, e, "unlinked-task-source")
	if _, err := service.UndismissedAdvice(e.Admin(), company); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("advice with a pending task source: %v", err)
	}
}

func TestRepairReadinessDoesNotDiscloseAnotherOwnersMessage(t *testing.T) {
	e := integration.Setup(t)
	owner := ids.From[ids.UserKind](e.AdminUser)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Hidden evidence", Source: "manual", OwnerID: &owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	visibility := "owner"
	if _, err := e.Contacts.UpdateCompany(e.Admin(), companyID, contacts.UpdateCompanyInput{Visibility: &visibility}); err != nil {
		t.Fatal(err)
	}
	mail := logRepairAdviceMail(t, e, companyID, "inbound", "other-owners-message", repairAdviceNow)
	perms := integration.AccountRepPerms
	perms.RowScope = principal.RowScopeOwn
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		return auth.EnsureActivityVisible(reader, tx, mail.UUID)
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the reader must not discover the fixture message: %v", err)
	}
	beginUnavailableThread(t, e, "other-owners-message")
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want bool
	}{{"owner", e.Admin(), true}, {"other reader", reader, false}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := database.WithWorkspaceTx(tc.ctx, e.Pool, func(tx pgx.Tx) error {
				args := []any{mail}
				arg := func(value any) int { args = append(args, value); return len(args) }
				idPosition := len(args)
				clause, err := auth.ActivityRepairPendingClause(tc.ctx, "a", arg)
				if err != nil {
					return err
				}
				var pending bool
				if err := tx.QueryRow(tc.ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM activity a WHERE a.id=$%d AND (%s))`, idPosition, clause), args...).Scan(&pending); err != nil {
					return err
				}
				if pending != tc.want {
					t.Fatalf("pending=%v, want %v", pending, tc.want)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
