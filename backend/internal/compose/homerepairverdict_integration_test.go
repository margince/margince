// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedClassificationUsesTrustedPredictionAndNativeWriter(t *testing.T) {
	for _, change := range []string{"unchanged", "human_body_change", "source_access_changed", "uncertain", "human_prior_change"} {
		t.Run(change, func(t *testing.T) { runReviewedClassification(t, change) })
	}
}

func runReviewedClassification(t *testing.T, change string) {
	e := integration.Setup(t)
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	permissions := integration.AdminPerms
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
	operator := e.As(e.AdminUser, nil, permissions)
	e.GrantRole(t, e.AdminUser, "admin")
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,data_task}','{"read":true,"update":true,"create":false,"delete":false}'::jsonb) WHERE key='admin'`)
	system := principal.SystemActing(operator, "system:review-fixture")
	activity, _, err := e.Activities.LogActivity(system, activities.LogActivityInput{Kind: "email", Direction: new("inbound"), Subject: new("Monthly update"), Body: new("This is the monthly report. No response is needed."), Source: "seed", OccurredAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(activity.Id)
	store := activities.NewStore(e.DB())
	wrote, err := store.SetOwedVerdict(system, id, activities.OwedVerdictAsksUs, "old-rules", now)
	if err != nil || !wrote {
		t.Fatalf("seed verdict: %v %v", wrote, err)
	}
	brain := &owedBrainStub{verdict: activities.OwedVerdictInformsUs, confidence: 0.95}
	if change == "uncertain" {
		brain.confidence = 0.4
	}
	if change == "human_prior_change" {
		if _, err := store.UpdateActivity(operator, ids.From[ids.ActivityKind](id), activities.UpdateActivityInput{Subject: new("Human-reviewed monthly report")}); err != nil {
			t.Fatal(err)
		}
	}
	server := &Server{dataTaskHandlers: dataTaskHandlers{pool: e.Pool, clock: func() time.Time { return now }}, log: slog.Default()}
	WithHomeRepairClassifier(brain)(server, e.Pool)
	handler := server.dataTaskHandlers
	response := httptest.NewRecorder()
	handler.PreviewHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview", bytes.NewBufferString(`{"source":"verdict","limit":25}`)).WithContext(operator))
	if response.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
	var wire crmcontracts.HomeRepairReview
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	review, err := homeRepairReviewInput(wire)
	if err != nil {
		t.Fatal(err)
	}
	var scope homeRepairScope
	if err := json.Unmarshal(review.Scope, &scope); err != nil {
		t.Fatal(err)
	}
	if change == "uncertain" || change == "human_prior_change" {
		if len(scope.Tasks) != 1 || scope.Tasks[0].Action != repairHumanReview || scope.Tasks[0].AfterVerdict != "" {
			t.Fatalf("uncertain or human work was changed: %+v", scope)
		}
		if change == "human_prior_change" && len(brain.prompts) != 0 {
			t.Fatal("human-adopted work sent for prediction")
		}
		if change == "uncertain" && len(brain.prompts) != 2 {
			t.Fatalf("ordinary solo confidence fallback not reused: %d", len(brain.prompts))
		}
		return
	}
	if len(scope.Tasks) != 1 || scope.Tasks[0].Action != repairReclassifyMessage || review.PreviewToken == "" {
		t.Fatalf("review: %+v %+v", review, scope)
	}
	if *verdictOf(t, e, id) != activities.OwedVerdictAsksUs {
		t.Fatal("preview changed source")
	}
	task := homeRepairDataTask(e.Pool, func() time.Time { return now })
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(system); err != nil {
		t.Fatal(err)
	}
	forged := review
	forged.PreviewToken = ""
	if err := engine.Activate(operator, homeRepairTaskID, "reviewed_tasks", 1, forged); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unsigned prediction: %v", err)
	}
	tampered := review
	changedScope := scope
	changedScope.Tasks = append([]homeRepairTaskRef(nil), scope.Tasks...)
	changedScope.Tasks[0].AfterVerdict = activities.OwedVerdictAsksUs
	changedScope.Tasks[0].Action = repairLeaveCurrent
	changedScope.Tasks[0].Reason = "classification_current"
	tampered.Scope, err = json.Marshal(changedScope)
	if err != nil {
		t.Fatal(err)
	}
	tampered.Digest, err = homeRepairDigest(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Activate(operator, homeRepairTaskID, "reviewed_tasks", 1, tampered); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("changed trusted outcome: %v", err)
	}
	foreign := e.As(e.Rep1, nil, permissions)
	if err := engine.Activate(foreign, homeRepairTaskID, "reviewed_tasks", 1, review); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("another reader reused review: %v", err)
	}
	if err := engine.Activate(operator, homeRepairTaskID, "reviewed_tasks", 1, review); err != nil {
		t.Fatal(err)
	}
	switch change {
	case "human_body_change":
		if _, err := store.UpdateActivity(operator, ids.From[ids.ActivityKind](id), activities.UpdateActivityInput{Body: new("A human revised this message after review.")}); err != nil {
			t.Fatal(err)
		}
	case "source_access_changed":
		if _, err := store.SetAudience(operator, ids.From[ids.ActivityKind](id), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.Execute(system); err != nil {
		t.Fatal(err)
	}
	if change != "unchanged" {
		if *verdictOf(t, e, id) != activities.OwedVerdictAsksUs {
			t.Fatal("changed source was overwritten")
		}
		if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND actor_type='human' AND evidence->>'repair_reason'='classification_changed'`, id) != 0 {
			t.Fatal("skipped correction emitted write audit")
		}
		return
	}
	if *verdictOf(t, e, id) != activities.OwedVerdictInformsUs {
		t.Fatal("reviewed correction not applied")
	}
	if len(brain.prompts) != 1 {
		t.Fatalf("execution called model: %d", len(brain.prompts))
	}
	if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND actor_type='human' AND evidence->>'repair_reason'='classification_changed'`, id) != 1 {
		t.Fatal("reviewed change lacks provenance")
	}
	wrote, err = store.SetOwedVerdict(system, id, activities.OwedVerdictAsksUs, "newer-rules", now.Add(time.Hour))
	if err != nil || wrote {
		t.Fatalf("ordinary classifier replaced human review: %v %v", wrote, err)
	}
}
