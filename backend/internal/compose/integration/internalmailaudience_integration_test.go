// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestInternalMailHasNoWithheldRowAndSharesOnlyOneMessageWithNamedColleagues(t *testing.T) {
	e := Setup(t)
	participant := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, AdminPerms)
	for _, target := range []*context.Context{&participant, &colleague} {
		actor, ok := principal.Actor(*target)
		if !ok {
			t.Fatal("missing fixture actor")
		}
		actor.SeatType = principal.SeatFull
		*target = principal.WithActor(*target, actor)
	}
	id := seedRestrictedInternalMail(participant, t, e)
	if got := e.WsCount(t, `SELECT count(*) FROM field_provenance WHERE object_type='activity'
 AND object_id=$1 AND field_name='internal_mail_authorization' AND source='imap'
 AND captured_by='connector:imap'`, id); got != 1 {
		t.Fatal("authorization field does not use the shared field-origin ledger")
	}
	assertInternalMailReadable(participant, t, e, id, true)
	assertInternalMailReadable(colleague, t, e, id, false)
	assertInternalMailReadable(e.Admin(), t, e, id, false)
	system := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	assertInternalMailReadable(system, t, e, id, false)
	company := e.SeedCompany(t, "Internal discussion account", nil)
	if _, err := e.Activities.RelinkActivity(participant, ids.From[ids.ActivityKind](id), activities.RelinkActivityInput{EntityType: "company", EntityID: company}); err != nil {
		t.Fatal(err)
	}
	assertInternalMailReadable(colleague, t, e, id, false)
	assertInternalMailReadable(e.Admin(), t, e, id, false)
	if got := e.WsCount(t, `SELECT count(*) FROM company WHERE id=$1 AND last_activity_at IS NOT NULL`, company); got != 0 {
		t.Fatal("internal mail advanced the customer activity clock")
	}
	original, err := e.Activities.GetActivity(participant, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil || original.CanShareInternal == nil || !*original.CanShareInternal {
		t.Fatalf("participant share capability: %+v %v", original.CanShareInternal, err)
	}
	presentation, err := e.Activities.GetEmailPresentation(participant, ids.From[ids.ActivityKind](id), nil)
	if err != nil || !presentation.Access.CanChange || presentation.Access.ChangeMode != "message_audience" {
		t.Fatalf("drawer share capability: %+v %v", presentation.Access, err)
	}
	in := activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}
	if _, err := e.Activities.SetAudience(participant, ids.From[ids.ActivityKind](id), in); err != nil {
		t.Fatal(err)
	}
	assertInternalMailReadable(colleague, t, e, id, true)
	shared, err := e.Activities.GetEmailPresentation(colleague, ids.From[ids.ActivityKind](id), nil)
	if err != nil || shared.Access.CanChange {
		t.Fatalf("selected recipient can reshare: %+v %v", shared.Access, err)
	}

	assertInternalMailReadable(participant, t, e, id, true)
	if _, err := e.Activities.SetAudience(colleague, ids.From[ids.ActivityKind](id), activities.SetAudienceInput{Audience: "participants"}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("selected recipient reshared: %v", err)
	}
	if _, err := e.Activities.SetAudience(participant, ids.From[ids.ActivityKind](id), activities.SetAudienceInput{Audience: "workspace"}); err == nil {
		t.Fatal("workspace share accepted")
	}
	if _, err := e.Activities.SetAudience(participant, ids.From[ids.ActivityKind](id), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "team", SubjectID: e.Team1}}}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("team share accepted: %v", err)
	}
	if err := database.WithWorkspaceTx(participant, e.Pool, func(tx pgx.Tx) error {
		return activities.RecomputeAudienceTx(participant, tx, ids.From[ids.ActivityKind](id))
	}); err != nil {
		t.Fatal(err)
	}
	assertInternalMailReadable(colleague, t, e, id, true)
	if _, err := e.Activities.SetAudience(participant, ids.From[ids.ActivityKind](id), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatal(err)
	}
	assertInternalMailReadable(colleague, t, e, id, false)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var evidence bool
		if err := tx.QueryRow(e.Admin(), `SELECT `+auth.ActivityExternalEvidenceClause("a")+` FROM activity a WHERE a.id = $1`, id).Scan(&evidence); err != nil {
			return err
		}
		if evidence {
			t.Error("internal mail counted as customer evidence")
		}
		_, err := tx.Exec(e.Admin(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertInternalMailReadable(participant, t, e, id, false)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		reader, err := auth.ActivityHasAReaderTx(e.Admin(), tx, id)
		if err == nil && reader {
			t.Error("departed participant kept a reader")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func assertInternalMailReadable(ctx context.Context, t *testing.T, e *Env, id ids.UUID, want bool) {
	t.Helper()
	_, err := e.Activities.GetActivity(ctx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if want && err != nil {
		t.Fatalf("authorized reader: %v", err)
	}
	if !want && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("hidden message disclosed a row: %v", err)
	}
}

// This fixture isolates the audience predicate; capture admission has its own connector tests.
func seedRestrictedInternalMail(participant context.Context, t *testing.T, e *Env) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	actor, ok := principal.Actor(participant)
	if !ok {
		t.Fatal("missing participant actor")
	}
	actor.Type, actor.ID = principal.PrincipalConnector, "connector:imap"
	connector := principal.WithActor(participant, actor)
	mailbox := ids.NewV7()
	provenance := json.RawMessage(`{"connection_id":"` + mailbox.String() + `","owner_user_id":"` + actor.UserID.String() + `","provider":"imap","generation":1,"enrolled_at":"2026-01-01T00:00:00Z","enrolled_by":"human:admin","policy_since":"2026-01-01T00:00:00Z"}`)
	if err := database.WithWorkspaceTx(connector, e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(connector, `INSERT INTO activity (id, kind, occurred_at, source, captured_by, audience, body)
   VALUES ($1, 'email', now(), 'imap', 'connector:imap', 'workspace', 'Internal account discussion')`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(connector, `INSERT INTO activity_participant (activity_id, user_id, role, address)
   VALUES ($1, $2, 'to', 'rep@team.test')`, id, e.Rep1); err != nil {
			return err
		}
		return activities.MarkInternalMailTx(connector, tx, ids.From[ids.ActivityKind](id), provenance, mailbox, 1)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRetentionControllerCanPinInternalMailWithoutReadingIt(t *testing.T) {
	e := Setup(t)
	participant := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	id := seedRestrictedInternalMail(participant, t, e)
	controller := controllerCtx(e, principal.ObjectGrant{Read: true, Update: true})
	actor, ok := principal.Actor(controller)
	if !ok {
		t.Fatal("missing controller")
	}
	actor.UserID, actor.ID = e.Rep2, "human:"+e.Rep2.String()
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true}
	controller = principal.WithActor(controller, actor)
	assertInternalMailReadable(controller, t, e, id, false)
	reason, err := privacy.ParseStatedReason("Operational correspondence subject to a statutory retention obligation")
	if err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(e.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).PinToFloor(controller, id, reason); err != nil {
		t.Fatal(err)
	}
	assertInternalMailReadable(participant, t, e, id, false)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND restricted_at IS NOT NULL AND internal_mail_authorization IS NOT NULL`, id); got != 1 {
		t.Fatalf("restricted internal rows=%d", got)
	}
	if err := privacy.NewEraser(e.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).WithRawCapturePurger(compose.RawCapturePurgerFor(e.DB())).ReleaseRestriction(controller, id, reason); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL
 AND internal_mail_authorization IS NOT NULL AND internal_mail_authorization->>'connection_id' IS NULL
 AND NOT internal_mail_authorization ? 'containers'`, id); got != 1 {
		t.Fatal("erasure kept source identities or removed the durable restriction")
	}
}
