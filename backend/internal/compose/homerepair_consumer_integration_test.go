// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedRepairArchivesHistoricalWorkWithoutRecreatingIt(t *testing.T) {
	t.Run("untouched automatic work", func(t *testing.T) { runReviewedRepair(t, false) })
	t.Run("human edit after review", func(t *testing.T) { runReviewedRepair(t, true) })
}

func runReviewedRepair(t *testing.T, editAfterReview bool) {
	e := integration.Setup(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -90)
	body := "I will prepare the account review."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: new("Account review"), Body: &body, Source: "manual", OccurredAt: &old})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	commitment := Commitment{Meaning: openCommitmentMeaning("", body), AsOf: old.Add(time.Hour), ReadMessages: []ids.UUID{sourceID}, Extractor: signalScanActor, SourceActivityID: sourceID, Summary: "Prepare the account review", Quote: body, Seat: &e.Rep1, Confidence: 0.95, Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), body), Body: body}
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		outcome, _, err := dispatcher.DispatchTx(principal.SystemActing(e.Admin(), "system:repair-fixture"), tx, commitment, ids.NewV7())
		if err == nil && outcome != CommitmentTaskWritten {
			t.Fatalf("original work=%s", outcome)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	permissions := integration.AdminPerms
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
	operator := e.As(e.AdminUser, nil, permissions)
	e.GrantRole(t, e.AdminUser, "admin")
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,data_task}','{"read":true,"update":true,"create":false,"delete":false}'::jsonb) WHERE key='admin'`)
	h := dataTaskHandlers{pool: e.Pool, clock: func() time.Time { return now }}
	response := httptest.NewRecorder()
	h.PreviewHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview", bytes.NewBufferString(`{"limit":10}`)).WithContext(operator))
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
	if review.Limit != 1 {
		t.Fatalf("reviewed rows=%d", review.Limit)
	}
	task := homeRepairDataTask(e.Pool)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	system := principal.WithCorrelationID(principal.SystemActing(operator, "system:home-repair-test"), ids.NewV7())
	if err := engine.Seed(system); err != nil {
		t.Fatal(err)
	}
	activationBody, err := json.Marshal(crmcontracts.ActivateHomeRepairJSONBody{Version: 1, Review: wire})
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	h.ActivateHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/activate", bytes.NewReader(activationBody)).WithContext(operator))
	if response.Code != http.StatusNoContent {
		t.Fatalf("activation: %d %s", response.Code, response.Body.String())
	}

	if editAfterReview {
		var scope homeRepairScope
		if err := json.Unmarshal(review.Scope, &scope); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Activities.UpdateActivity(operator, ids.From[ids.ActivityKind](scope.Tasks[0].ID), activities.UpdateActivityInput{Subject: new("Keep this personal follow-up"), IfVersion: &scope.Tasks[0].Version}); err != nil {
			t.Fatal(err)
		}
	}
	expectedChanges := int64(1)
	if editAfterReview {
		expectedChanges = 0
	}
	if _, err := engine.Execute(system); err != nil {
		t.Fatal(err)
	}
	progress, err := engine.List(operator)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].Changed != expectedChanges || progress[0].Status != datatask.Done {
		t.Fatalf("repair progress=%+v", progress)
	}
	if e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NOT NULL AND NOT is_done") != int(expectedChanges) {
		t.Fatal("repair did not preserve human changes or archive automatic work")
	}
	if e.WsCount(t, "SELECT count(*) FROM audit_log WHERE entity_type='activity' AND action='archive' AND evidence->>'rule_version' IS NOT NULL AND before ? 'archived_at' AND after ? 'archived_at'") != int(expectedChanges) {
		t.Fatal("repair omitted reasoned before/after evidence")
	}
	if editAfterReview && e.WsCount(t, "SELECT count(*) FROM system_log WHERE action='home_repair.skipped' AND detail->>'reason'='changed_since_review'") != 1 {
		t.Fatal("concurrent edit was not reported")
	}
	commitment.AsOf = now
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, _, err := dispatcher.DispatchTx(principal.SystemActing(e.Admin(), "system:repair-fixture"), tx, commitment, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task'") != 1 || e.WsCount(t, "SELECT count(*) FROM approval") != 0 {
		t.Fatal("another extraction recreated repaired noise")
	}
}
