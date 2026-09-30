// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingPublicationDoesNotLockLoginBookkeeping(t *testing.T) {
	f := reportingBusiness(t)
	system := principal.SystemActing(f.human, "system:authority-fence")
	users := identity.NewService(f.env.Pool)
	err := InstallationDB(f.env.Pool).TxIsolated(system, pgx.RepeatableRead, func(tx pgx.Tx) error {
		if _, _, err := users.PublicationAuthority(system, tx, f.env.Rep1); err != nil {
			return err
		}
		update, cancel := context.WithTimeout(system, time.Second)
		defer cancel()
		_, err := f.env.owner.Exec(update, "UPDATE app_user SET failed_login_count=failed_login_count+1 WHERE id=$1", f.env.Rep1)
		return err
	})
	if err != nil {
		t.Fatalf("publication blocked login bookkeeping: %v", err)
	}
}

func TestReportingBudgetedSnapshotCannotPublishAfterAuthorityRevocation(t *testing.T) {
	f := reportingBusiness(t)
	system := principal.SystemActing(f.human, "system:authority-fence")
	users := identity.NewService(f.env.Pool)
	err := InstallationDB(f.env.Pool).Bounded(time.Second).TxIsolated(system, pgx.RepeatableRead, func(tx pgx.Tx) error {
		if _, err := f.env.owner.Exec(system, "UPDATE app_user SET archived_at=now() WHERE id=$1", f.env.Rep1); err != nil {
			return err
		}
		_, _, err := users.PublicationAuthority(system, tx, f.env.Rep1)
		return err
	})
	var serialization *pgconn.PgError
	if !errors.As(err, &serialization) || serialization.Code != "40001" {
		t.Fatalf("stale budgeted snapshot admitted publication: %v", err)
	}
}

func TestReportingLeadTargetWritesRequireLiveTeamMembership(t *testing.T) {
	f := reportingBusiness(t)
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	delete(actor.Permissions.Objects, "team_oversight")
	actor.Permissions.Objects["team_lead"] = principal.ObjectGrant{Read: true}
	lead := principal.WithActor(f.human, actor)
	input := f.target.Definition
	input.Scope = crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(f.env.Team2)}
	if _, err := f.service.CreateTarget(lead, input); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("lead wrote another team target: %v", err)
	}
	input.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep3)}
	if _, err := f.service.CreateTarget(lead, input); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("lead wrote nonmember target: %v", err)
	}
	input.Scope = crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(f.env.Team1)}
	target, err := f.service.CreateTarget(lead, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.owner.Exec(lead, "DELETE FROM team_membership WHERE user_id=$1", f.env.Rep1); err != nil {
		t.Fatal(err)
	}
	input.Value++
	if _, err := f.service.UpdateTarget(lead, ids.UUID(target.Id), target.Version, input); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("stale team claim authorized update: %v", err)
	}
}
