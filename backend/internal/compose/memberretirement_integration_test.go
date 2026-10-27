// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func memberManagerFor(t *testing.T, e *integration.Env) memberManager {
	t.Helper()
	manager := newMemberManager(e.DB())
	admin := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	if err := manager.service.ChangeUserRole(e.As(e.AdminUser, nil, integration.AdminWithSignals), admin, admin.UserID, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, user := range []ids.UUID{e.Rep1, e.Rep2} {
		if err := manager.service.ChangeUserRole(e.As(e.AdminUser, nil, integration.AdminWithSignals), admin, ids.From[ids.UserKind](user), "rep"); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

func TestAMemberMergeMovesOwnedWorkAndKeepsTheHistoricalIdentity(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Account merge example", pipeline, stage, &e.Rep1)
	task := assignedTask(t, e, "Follow up", e.Rep1, false)
	manager := memberManagerFor(t, e)
	preview, err := manager.Preview(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Owned.Deal != 1 || preview.Owned.Task != 1 || len(preview.Blockers) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if err := manager.Merge(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT owner_id::text FROM deal WHERE id=$1`, deal); got != e.Rep2.String() {
		t.Fatalf("deal owner = %s", got)
	}
	if got := assigneeOf(t, e, task); got != e.Rep2 {
		t.Fatalf("task assignee = %s", got)
	}
	if got := e.WsScalar(t, `SELECT merged_into_user_id::text FROM app_user WHERE id=$1 AND removed_at IS NOT NULL AND status='deactivated'`, e.Rep1); got != e.Rep2.String() {
		t.Fatalf("merge lineage = %s", got)
	}
	names, err := manager.service.SeatNames(e.As(e.AdminUser, nil, integration.AdminWithSignals), []ids.UserID{ids.From[ids.UserKind](e.Rep1)})
	if err != nil {
		t.Fatal(err)
	}
	if names[e.Rep1] == "" {
		t.Fatal("historical identity lost its name")
	}
	list, err := manager.service.ListUsers(e.As(e.AdminUser, nil, integration.AdminWithSignals), identity.ListUsersInput{IncludeInactive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range list.Users {
		if member.ID == e.Rep1 {
			t.Fatal("merged source remains in the roster")
		}
	}
}

func TestAChangedMemberMergePreviewCannotRemoveTheAccount(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	preview, err := manager.Preview(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "New work after preview", pipeline, stage, &e.Rep1)
	if err := manager.Merge(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, e.Rep2, preview.Token); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale merge = %v", err)
	}
	if got := e.WsScalar(t, `SELECT owner_id::text FROM deal WHERE id=$1`, deal); got != e.Rep1.String() {
		t.Fatal("stale merge moved work")
	}
	if got := e.WsScalar(t, `SELECT (removed_at IS NULL)::text FROM app_user WHERE id=$1`, e.Rep1); got != "true" {
		t.Fatal("stale merge removed the source")
	}
}

func TestMemberRemovalCannotBypassOwnershipOrAdministratorChecks(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	if _, err := manager.Preview(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.AdminUser, nil, false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("self-removal = %v", err)
	}
	if _, err := manager.Preview(e.As(e.Rep1, nil, integration.AccountRepPerms), e.Rep2, nil, false); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("non-admin preview = %v", err)
	}
	pipeline, stage, _ := integration.DealFixture(t, e)
	e.SeedDeal(t, "Work needs a new owner", pipeline, stage, &e.Rep1)
	preview, err := manager.Preview(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) == 0 {
		t.Fatal("removal omitted the required handover")
	}
	actor, err := manager.service.MemberAdministrator(e.As(e.AdminUser, nil, integration.AdminWithSignals))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.service.RetireMember(e.As(e.AdminUser, nil, integration.AdminWithSignals), actor, ids.From[ids.UserKind](e.Rep1), nil, preview.Token, false); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("removal with work = %v", err)
	}
}

func TestAMemberMergePreservesPrivateCorrespondenceWithoutChangingItsAuthor(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	sourceCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	targetCtx := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	subject, body := "Private note", "Historical text"
	at := time.Date(2026, time.June, 10, 9, 0, 0, 0, time.UTC)
	row, _, err := e.Activities.LogActivity(sourceCtx, activities.LogActivityInput{Kind: "note", Subject: &subject, Body: &body, OccurredAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := e.Activities.SetAudience(sourceCtx, id, activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.GetActivityContent(targetCtx, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private note before merge = %v", err)
	}
	author := e.WsScalar(t, `SELECT captured_by FROM activity WHERE id=$1`, id)
	preview, err := manager.Preview(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Merge(e.As(e.AdminUser, nil, integration.AdminWithSignals), e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.GetActivityContent(targetCtx, id, storekit.LiveOnly); err != nil {
		t.Fatalf("private note after merge = %v", err)
	}
	if got := e.WsScalar(t, `SELECT captured_by FROM activity WHERE id=$1`, id); got != author {
		t.Fatal("merge rewrote the original author")
	}
}

func TestRemovingADeactivatedMemberIsIdempotentAndCannotBecomeAMerge(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	source := ids.From[ids.UserKind](e.Rep1)
	if err := manager.service.DeactivateUser(ctx, actor, identity.DeactivateUserInput{UserID: source}); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) != 0 {
		t.Fatalf("blockers = %v", preview.Blockers)
	}
	for range 2 {
		if err := manager.service.RetireMember(ctx, actor, source, nil, preview.Token, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("removal changed into merge: %v", err)
	}
	if err := manager.service.ReactivateUser(ctx, actor, source); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("removed member reactivated: %v", err)
	}
}

func TestAMemberMergeRetryKeepsTheSurvivorsAuthority(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	roles := e.WsScalar(t, `SELECT string_agg(role_id::text, ',' ORDER BY role_id) FROM role_assignment WHERE user_id=$1`, e.Rep2)
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsScalar(t, `SELECT string_agg(role_id::text, ',' ORDER BY role_id) FROM role_assignment WHERE user_id=$1`, e.Rep2); got != roles {
		t.Fatalf("survivor roles changed: %s -> %s", roles, got)
	}
	if err := manager.Merge(ctx, e.Rep1, e.AdminUser, preview.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("retry changed survivor: %v", err)
	}
}

func TestARetiredMembersCompletedTaskCannotBeReopenedOntoTheHiddenAccount(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	task := assignedTask(t, e, "Completed before retirement", e.Rep1, true)
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	_, err = e.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](task), activities.UpdateActivityInput{IsDone: new(false)})
	if constraint, ok := storekit.CheckViolation(err); !ok || constraint != "member_work_reference_retired" {
		t.Fatalf("reopen on retired member: %v", err)
	}
	if got := e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id=$1`, task); got != "true" {
		t.Fatal("failed reopen changed the task")
	}
}

func TestAMergedMemberKeepsItsPersonalWriteShare(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	company := e.SeedCompany(t, "Shared work", &e.AdminUser)
	if _, err := manager.service.CreateRecordGrant(ctx, identity.CreateGrantInput{RecordType: "company", RecordID: company, SubjectType: "user", SubjectID: e.Rep1, Access: "write"}); err != nil {
		t.Fatal(err)
	}
	target := e.As(e.Rep2, nil, integration.AccountRepPerms)
	writable := func(ctx context.Context) error {
		return e.DB().Tx(ctx, func(tx pgx.Tx) error { return auth.EnsureWritable(ctx, tx, "company", company) })
	}
	if err := writable(target); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("write before merge: %v", err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	if err := writable(target); err != nil {
		t.Fatalf("merged write share: %v", err)
	}
}

func TestAMergedMemberCanReadItsHistoricalPrivateSignal(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	company := e.SeedCompany(t, "Historical alert", &e.AdminUser)
	stamp := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{Kind: "risk", CompanyID: company, Summary: "A private observation", Severity: "warn", Fingerprint: "historical-member-alert", PrivateTo: e.Rep1}, stamp)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM signal WHERE fingerprint=$1`, "historical-member-alert"))
	if err != nil {
		t.Fatal(err)
	}
	store := signals.NewStore(e.DB(), nil)
	source := e.As(e.Rep1, nil, integration.AdminWithSignals)
	target := e.As(e.Rep2, nil, integration.AdminWithSignals)
	if _, err := store.UpdateSignal(source, ids.From[ids.SignalKind](id), signals.UpdateSignalInput{Status: new("dismissed")}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSignal(target, ids.From[ids.SignalKind](id), storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("signal before merge: %v", err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSignal(target, ids.From[ids.SignalKind](id), storekit.LiveOnly); err != nil {
		t.Fatalf("historical signal after merge: %v", err)
	}
}

func TestAMemberMergeRefusesAUsedSourceAndTheActingAdministratorsAccount(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	preview, err := manager.Preview(ctx, e.Rep1, &e.AdminUser, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Blockers, "self_survivor") {
		t.Fatal("acting administrator offered as survivor")
	}
	if err := manager.Merge(ctx, e.Rep1, e.AdminUser, preview.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("self survivor = %v", err)
	}
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	token, _, err := manager.service.IssuePasswordLink(ctx, actor, ids.From[ids.UserKind](e.Rep1))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.service.RedeemPasswordReset(ctx, token, "a protected account password"); err != nil {
		t.Fatal(err)
	}
	preview, err = manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Blockers, "source_used") {
		t.Fatal("used account offered as merge source")
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("used source = %v", err)
	}
}

func TestAnUnusedMembersUnreadableTaskMovesWithoutDisclosingItsContent(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	source := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	to := ids.From[ids.UserKind](e.Rep1)
	subject := "Private imported follow up"
	row, _, err := e.Activities.LogActivity(source, activities.LogActivityInput{Kind: "task", Subject: &subject, AssigneeID: &to, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := e.Activities.SetAudience(source, id, activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	if _, err := e.Activities.GetActivityContent(ctx, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private task before merge = %v", err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Owned.Task != 1 || len(preview.Blockers) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	if assigneeOf(t, e, id.UUID) != e.Rep2 {
		t.Fatal("private task was left on the retired account")
	}
	if _, err := e.Activities.GetActivityContent(ctx, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private task after merge = %v", err)
	}
}

func TestReimportingAMergedMemberWhoseSurvivorWasRemovedRefusesPrecisely(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	email := e.WsScalar(t, `SELECT email FROM app_user WHERE id=$1`, e.Rep1)
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	survivor := ids.From[ids.UserKind](e.Rep2)
	if err := manager.service.DeactivateUser(ctx, actor, identity.DeactivateUserInput{UserID: survivor}); err != nil {
		t.Fatal(err)
	}
	preview, err = manager.Preview(ctx, e.Rep2, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.service.RetireMember(ctx, actor, survivor, nil, preview.Token, false); err != nil {
		t.Fatal(err)
	}
	store := contacts.NewStore(e.DB()).WithInstallationRenamer(installationRenamer(e.Pool))
	if _, err := store.SaveCompany(ctx, contacts.SaveCompanyInput{DisplayName: "Example company"}); err != nil {
		t.Fatal(err)
	}
	identity.NewHandlers(manager.service).WithInstallationDescribed(installationDescribed(store))
	_, err = manager.service.CreateFormerMember(ctx, actor, identity.FormerMemberInput{Email: email, DisplayName: "Imported colleague"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("reimport removed survivor = %v", err)
	}
}
