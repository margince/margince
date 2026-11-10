// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestInquiryAssignmentAndItsDurableChoiceCommitTogether(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"inquiry@visitor.example"}, "message": {"A private request"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission = %d %s", answer.Status, answer.Body)
	}
	ctx := t.Context()
	workspace, err := e.DB().Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var colleague, lead, note ids.UUID
	var key string
	if err := e.Owner.QueryRow(ctx, `SELECT id FROM app_user WHERE status='active' AND NOT is_agent AND archived_at IS NULL ORDER BY created_at LIMIT 1`).Scan(&colleague); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(ctx, `SELECT id FROM lead WHERE email='inquiry@visitor.example'`).Scan(&lead); err != nil {
		t.Fatal(err)
	}
	if err := e.Owner.QueryRow(ctx, `SELECT id,source_id FROM activity WHERE kind='note' AND source_system=$1`, provenance.WebFormSource).Scan(&note, &key); err != nil {
		t.Fatal(err)
	}
	var payload json.RawMessage
	if err := e.Owner.QueryRow(ctx, `SELECT envelope->'payload' FROM event_outbox WHERE envelope->>'type'='activity.captured' AND envelope->'entity'->>'id'=$1`, note.String()).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event struct {
		Key *string `json:"website_submission_key"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Key == nil || *event.Key != key {
		t.Fatalf("accepted event has no canonical submission identity: %s", payload)
	}
	scoped := principal.WithWorkspaceID(context.Background(), workspace.UUID)
	scoped = principal.WithCorrelationID(scoped, ids.NewV7())
	live, err := identity.NewService(e.Pool).EffectiveRBAC(scoped, workspace.UUID, colleague)
	if err != nil {
		t.Fatal(err)
	}
	admit := principal.WithActor(scoped, principal.Principal{Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + colleague.String(), UserID: colleague, Permissions: live.Permissions, TeamIDs: live.TeamIDs})
	policy, err := automation.NewAutomationStore(e.DB()).Create(admit, automation.CreateAutomationInput{Key: "route_lead", Name: "Inquiry assignment test", Params: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(scoped, "system:inquiry-test")
	claims := automation.NewEffectClaims(e.DB())
	router := contacts.NewStore(e.DB())
	cfg := contacts.RoutingConfig{Owners: []ids.UserID{ids.From[ids.UserKind](colleague)}}
	var beforeAudit, beforeEvents, beforeVersion int64
	if err := e.Owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_log), (SELECT count(*) FROM event_outbox), version FROM lead WHERE id=@id`, pgx.NamedArgs{"id": lead}).Scan(&beforeAudit, &beforeEvents, &beforeVersion); err != nil {
		t.Fatal(err)
	}
	preview, err := router.PreviewLeadRouting(admit, ids.From[ids.LeadKind](lead), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Assigned || preview.OwnerID.UUID != colleague {
		t.Fatalf("preview recipient: %+v", preview)
	}
	var afterAudit, afterEvents, afterVersion int64
	var previewOwner *ids.UUID
	if err := e.Owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_log), (SELECT count(*) FROM event_outbox), version, owner_id FROM lead WHERE id=@id`, pgx.NamedArgs{"id": lead}).Scan(&afterAudit, &afterEvents, &afterVersion, &previewOwner); err != nil {
		t.Fatal(err)
	}
	if beforeAudit != afterAudit || beforeEvents != afterEvents || beforeVersion != afterVersion || previewOwner != nil {
		t.Fatalf("preview wrote data: audit %d/%d, events %d/%d, version %d/%d, owner %v", beforeAudit, afterAudit, beforeEvents, afterEvents, beforeVersion, afterVersion, previewOwner)
	}
	choose := func(bound context.Context) (automation.InquiryPlan, error) {
		decision, err := router.RouteLeadAs(bound, admit, ids.From[ids.LeadKind](lead), cfg)
		if err != nil {
			return automation.InquiryPlan{}, err
		}
		if decision != preview {
			return automation.InquiryPlan{}, errors.New("live routing disagrees with preview")
		}
		if !decision.Assigned {
			return automation.InquiryPlan{}, errors.New("lead was not assigned")
		}
		return automation.InquiryPlan{PolicyID: policy.ID.UUID, PolicyVersion: policy.Version, SourceActivityID: note, Target: datasource.EntityRef{Type: datasource.EntityLead, ID: lead}, Recipient: decision.OwnerID.UUID, DueAt: time.Date(2026, 11, 12, 23, 59, 59, 0, time.UTC), Reason: decision.Reason}, nil
	}
	injected := errors.New("assignment could not be recorded")
	_, err = claims.FreezeInquiry(system, key, func(bound context.Context) (automation.InquiryPlan, error) {
		p, err := choose(bound)
		if err != nil {
			return p, err
		}
		return p, injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("freeze failure = %v", err)
	}
	var owner *ids.UUID
	if err := e.Owner.QueryRow(ctx, `SELECT owner_id FROM lead WHERE id=$1`, lead).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != nil {
		t.Fatalf("failed plan left the lead assigned to %s", owner.String())
	}
	var count int
	if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM automation_effect_claim WHERE occurrence_key=$1`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed plan left %d durable claims", count)
	}
	first, err := claims.FreezeInquiry(system, key, choose)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := claims.FreezeInquiry(system, key, func(context.Context) (automation.InquiryPlan, error) {
		return automation.InquiryPlan{}, errors.New("retry selected a new recipient")
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first || replayed.Recipient != colleague || replayed.SourceActivityID != note {
		t.Fatalf("retry changed its plan: first %+v, replay %+v", first, replayed)
	}
	if err := e.Owner.QueryRow(ctx, `SELECT owner_id FROM lead WHERE id=$1`, lead).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner == nil || *owner != colleague {
		t.Fatalf("committed owner = %v, want %s", owner, colleague)
	}
}
