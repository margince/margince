// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit's unattended feed running as a service member.
//
// The member row has the shape DeclareServiceMember gives it, and the role is
// the shipped one. Declaring over HTTP is covered where the whole application is
// up. Here the ingest path is under test: the consent check, the live-authority
// check and what survives a colleague leaving.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// declareServiceMemberFor makes a service member serving unit, holding the
// shipped service_ingest role.
func declareServiceMemberFor(t *testing.T, unit string) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning the declaration: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the declaration: %v", err)
		}
	}()
	roleID, err := identity.EnsureServiceIngestRole(ctx, tx)
	if err != nil {
		t.Fatalf("seeding the service role: %v", err)
	}
	member := ids.NewV7()
	if _, err := tx.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name, is_agent, service_extension)
		 VALUES ($1, $2, 'Feed', true, $3)`, member, "service-"+member.String()+"@members.invalid", unit); err != nil {
		t.Fatalf("inserting the service member: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`, roleID, member); err != nil {
		t.Fatalf("assigning the service role: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the declaration: %v", err)
	}
	return member
}

func TestAFeedRunsAsItsServiceMemberAndOffboardingAColleagueDoesNotStopIt(t *testing.T) {
	e := setupIngress(t)
	member := declareServiceMemberFor(t, ingressUnit)
	depositCredential(t, e.extRuntimeEnv, member)
	rt := e.ingestingRuntime()

	result, err := rt.Ingest(context.Background(), extension.UserID(member.String()),
		aProviderRecord("ws-7:9001", "outside@example.test"))
	if err != nil || result.Disposition != extension.DispositionAccepted {
		t.Fatalf("ingest as the service member = %+v, %v, want accepted", result, err)
	}
	var capturedBy string
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT captured_by FROM activity WHERE id = $1`,
			ids.MustParse(result.Ref.ID)).Scan(&capturedBy)
	})
	if want := ingressProbeCapture + ":" + member.String(); capturedBy != want {
		t.Errorf("captured_by = %q, want %q: the row must name the service member that landed it", capturedBy, want)
	}

	// The colleague who set the feed up leaves. Nothing about the feed was
	// filed under them, so the next poll lands as before.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated', archived_at = now() WHERE id = $1`, e.member); err != nil {
		t.Fatalf("offboarding the colleague: %v", err)
	}
	if _, err := rt.Ingest(context.Background(), extension.UserID(member.String()),
		aProviderRecord("ws-7:9002", "outside@example.test")); err != nil {
		t.Fatalf("ingest after the colleague left: %v", err)
	}
}

func TestAServiceMemberDeclaredForAnotherUnitCannotBeActedFor(t *testing.T) {
	e := setupIngress(t)
	member := declareServiceMemberFor(t, "some-other-unit")
	// The credential is on deposit under this unit, so only the declaration
	// can refuse it.
	depositCredential(t, e.extRuntimeEnv, member)

	_, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(member.String()),
		aProviderRecord("ws-7:9003", "outside@example.test"))
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden — one unit borrowed the member another unit's feed runs as", err)
	}
}

func TestAServiceMemberWithNoSecretOnDepositCannotBeActedFor(t *testing.T) {
	e := setupIngress(t)
	member := declareServiceMemberFor(t, ingressUnit)

	_, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(member.String()),
		aProviderRecord("ws-7:9004", "outside@example.test"))
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden — declaring a member is not the consent act, the feed's secret is", err)
	}
}

func TestAnArchivedServiceMemberLandsNothing(t *testing.T) {
	e := setupIngress(t)
	member := declareServiceMemberFor(t, ingressUnit)
	depositCredential(t, e.extRuntimeEnv, member)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated', archived_at = now() WHERE id = $1`, member); err != nil {
		t.Fatalf("archiving the service member: %v", err)
	}

	if _, err := e.ingestingRuntime().Ingest(context.Background(), extension.UserID(member.String()),
		aProviderRecord("ws-7:9005", "outside@example.test")); err == nil {
		t.Fatal("an archived service member landed a record, so archiving does not stop the feed")
	}
	if got := e.countAsWorkspace(t, `SELECT count(*) FROM raw_capture WHERE source_id = $1`, "ws-7:9005"); got != 0 {
		t.Errorf("raw_capture rows = %d for the refused record, want none", got)
	}
}

// The secret is filed through the unit's own port, so the service member holds
// it as a colleague would.
func TestTheFeedsSecretIsReadBackFromTheServiceMember(t *testing.T) {
	e := setupIngress(t)
	member := declareServiceMemberFor(t, ingressUnit)
	depositCredential(t, e.extRuntimeEnv, member)

	got, err := extsecrets.For(ingressUnit, e.Pool, e.vault).GetUser(e.callCtx(e.WS),
		extension.UserID(member.String()), ingressTestSecretKey)
	if err != nil || string(got) != "pat_probe" {
		t.Fatalf("reading the member's secret = %q, %v, want the deposited value", got, err)
	}
}
