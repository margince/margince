// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// projectFilingAuthority is a member who may file captured mail under a
// project, so only the internal-mail rule keeps the ladder away.
type projectFilingAuthority struct{ internalMailAuthority }

func (projectFilingAuthority) EffectiveRBAC(context.Context, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
		"activity": {Create: true, Read: true, Update: true}, "contact": {Create: true}, "lead": {Create: true}, "project": {Read: true},
	}}}, nil
}

func (r projectFilingAuthority) AdmittedAuthority(ctx context.Context, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, human, r.EffectiveRBAC, r.SeatType)
}

// everyKeyMatcher answers one live project for any bracketed key.
type everyKeyMatcher struct{ project ids.UUID }

func (m everyKeyMatcher) MatchProjectKey(context.Context, pgx.Tx, []string) (ids.UUID, error) {
	return m.project, nil
}

// Internal operational mail is filed under no project and stamped as no
// project's correspondence, even when its subject names a live project.
func TestAdmittedInternalMailIsFiledUnderNoProject(t *testing.T) {
	ctx, connecting, vault, ws, pool := newInternalMailRegistryFixture(t)
	connection, err := connecting.Connect(ctx, "gmail", connector.Auth("fixture-token"))
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.SeatType = principal.SeatFull
	actor.Permissions = principal.Permissions{
		RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
	}
	admin := principal.WithActor(ctx, actor)
	seedInternalMailAnchor(admin, t, pool, actor.UserID)
	project := ids.NewV7()
	if err := database.WithTx(admin, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(admin, `INSERT INTO project (id, name, key, source, captured_by)
			VALUES ($1, 'Payroll', 'OPS-1', 'manual', $2)`, project, actor.ID)
		return err
	}); err != nil {
		t.Fatalf("seeding the project: %v", err)
	}
	raw, cfg := configtest.StoredOverWith(t, pool, capture.Definitions(), capture.ConfigSettings())
	policy, err := capture.NewSettings(raw, cfg).SetInternalMailPolicy(admin, true,
		[]capture.InternalMailSelection{{ConnectionID: connection, Containers: []string{"gmail:INBOX"}}})
	if err != nil {
		t.Fatal(err)
	}

	rec := operationalMailRecord("internal-project-"+ids.NewV7().String(), "colleague@example.test", "colleague@example.test", fixtureOwnerEmail)
	rec.Fields = capture.ActivityFields{Kind: "email", Subject: "[OPS-1] salary review", Body: "the numbers", Direction: connector.DirectionInbound}
	rec.CapturedBy, rec.Source, rec.DeliveredTo = "connector:gmail", "gmail:internal-project", fixtureOwnerEmail
	rec.Containers = []string{"gmail:INBOX"}
	rec.ProviderReceivedAt = policy.Since.Add(time.Second)
	stamps := 0
	db := database.BindTo(pool, ws)
	sink := capture.NewSink(db).WithInternalMailRecorder(activities.MarkInternalMailTx).
		WithAudienceRecompute(activities.RecomputeAudienceTx).
		WithProjectAttribution(capture.ProjectAttribution{
			Keys: everyKeyMatcher{project: project},
			Stamp: func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error {
				stamps++
				return nil
			},
			File: activities.FileOnProjectTx,
		})
	var captured ids.UUID
	reg := capture.NewRegistry(db, sink, projectFilingAuthority{}, vault)
	reg.Register(internalMailLiveProvider{act: func(runCtx context.Context, target connector.Sink) error {
		ref, err := target.Upsert(runCtx, rec)
		captured = ref.ID
		return err
	}})
	if err := reg.SyncOnce(context.Background(), connection); err != nil {
		t.Fatal(err)
	}

	var internal bool
	var links int
	if err := database.WithTx(admin, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin, `SELECT a.internal_mail_authorization IS NOT NULL,
			(SELECT count(*) FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'project')
			FROM activity a WHERE a.id = $1`, captured).Scan(&internal, &links)
	}); err != nil {
		t.Fatalf("reading the captured mail: %v", err)
	}
	if !internal {
		t.Fatal("the mail was not admitted as internal mail, so this proves nothing")
	}
	if links != 0 || stamps != 0 {
		t.Errorf("internal mail is filed under %d project(s) and stamped %d time(s), want neither", links, stamps)
	}
}
