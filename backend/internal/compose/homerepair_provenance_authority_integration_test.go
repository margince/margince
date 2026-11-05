// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedLegacyActivationRequiresTheAuthorityItsActionUses(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		old, update, archive bool
		status               int
	}{
		{"correct with update", false, true, false, http.StatusNoContent},
		{"cannot correct with delete", false, false, true, http.StatusForbidden},
		{"cannot archive with update", true, true, false, http.StatusForbidden},
		{"archive with delete", true, false, true, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := seedLegacyRepair(t, tc.old)
			repairOperator(t, f.e)
			permissions := integration.AdminPerms
			permissions.Objects = maps.Clone(permissions.Objects)
			permissions.Objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
			grant := principal.ObjectGrant{Read: true, Update: tc.update, Delete: tc.archive}
			permissions.Objects["activity"] = grant
			encoded, err := json.Marshal(grant)
			if err != nil {
				t.Fatal(err)
			}
			f.e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,activity}',@grant::jsonb) WHERE key='admin'`, pgx.NamedArgs{"grant": string(encoded)})
			operator := f.e.As(f.e.AdminUser, nil, permissions)
			handler := dataTaskHandlers{pool: f.e.Pool, clock: func() time.Time { return f.now }}
			wire := legacyRepairWire(operator, t, handler)
			system, engine := probeEngine(t, f.e, &f.now, homeRepairDataTask(f.e.Pool, func() time.Time { return f.now }))
			activation := crmcontracts.ActivateHomeRepairJSONBody{Version: 1, Review: wire}
			if wire.RequiresActionReview != nil && *wire.RequiresActionReview {
				actions := []string{string(wire.Tasks[0].Action)}
				activation.ReviewedActions = &actions
			}
			body, err := json.Marshal(activation)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ActivateHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/activate", bytes.NewReader(body)).WithContext(operator))
			if response.Code != tc.status {
				t.Fatalf("activation %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
			switch tc.status {
			case http.StatusNoContent:
				if _, err := engine.Execute(system); err != nil {
					t.Fatal(err)
				}
				query := "SELECT count(*) FROM activity WHERE id=@id AND archived_at IS NOT NULL AND NOT is_done"
				if !tc.old {
					query = "SELECT count(*) FROM activity WHERE id=@id AND source_activity_id=@source AND due_at IS NULL AND NOT is_done AND archived_at IS NULL"
				}
				args := pgx.NamedArgs{"id": f.task}
				if !tc.old {
					args["source"] = f.source
				}
				if f.e.WsCount(t, query, args) != 1 {
					t.Fatal("authorized action was not applied")
				}
			default:
				if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND source_activity_id IS NULL AND due_at IS NOT NULL AND archived_at IS NULL", pgx.NamedArgs{"id": f.task}) != 1 {
					t.Fatal("refused activation changed the task")
				}
			}
		})
	}
}

func legacyRepairWire(operator context.Context, t *testing.T, handler dataTaskHandlers) crmcontracts.HomeRepairReview {
	t.Helper()
	response := httptest.NewRecorder()
	handler.PreviewHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview", bytes.NewBufferString(`{"source":"task","limit":25}`)).WithContext(operator))
	if response.Code != http.StatusOK {
		t.Fatalf("preview %d: %s", response.Code, response.Body.String())
	}
	var wire crmcontracts.HomeRepairReview
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Tasks) == 0 {
		t.Fatal("preview has no rows")
	}
	return wire
}

func TestReviewedLegacyPreviewKeepsAChangedTaskForReview(t *testing.T) {
	f := seedLegacyRepair(t, false)
	var before activities.HomeRepairTask
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		var err error
		before, err = activities.HomeRepairTaskTx(f.e.Admin(), tx, f.task.UUID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), f.task, activities.UpdateActivityInput{Subject: new("My plan")}); err != nil {
		t.Fatal(err)
	}
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		decision, err := homeRepairTaskDecisionTx(f.e.Admin(), tx, before, f.now, false)
		if err == nil && (decision.Action != repairHumanReview || decision.Reason != "task_changed_since_listed") {
			t.Fatalf("changed row aborted or authorized the preview: %+v", decision)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedLegacyRepairKeepsAnExplicitDateEqualToTheGeneratedDate(t *testing.T) {
	f := seedLegacyRepair(t, false)
	chosen := f.now.Add(time.Hour)
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), f.task, activities.UpdateActivityInput{DueAt: &chosen}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), f.task, activities.UpdateActivityInput{DueAt: &f.now}); err != nil {
		t.Fatal(err)
	}
	review, item := f.preview(t)
	if item.Action != repairHumanReview || item.Reason != "human_adoption_not_excluded" {
		t.Fatalf("explicit date treated as generated: %+v", item)
	}
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		changed, err := applyHomeRepairTask(f.e.Admin(), tx, f.e.Activities, item, datatask.Activation{ReviewRequest: review}, ids.NewV7())
		if changed {
			t.Fatal("explicit date changed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		var due time.Time
		if err := tx.QueryRow(f.e.Admin(), storekit.SQLf("SELECT %s FROM activity a WHERE id=@id", storekit.TaskDueSQL("a")), pgx.NamedArgs{"id": f.task}).Scan(&due); err != nil {
			return err
		}
		if !due.Equal(f.now) {
			t.Fatal("the canonical date reader hid an explicitly chosen equal date")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND due_at=@due AND source_activity_id IS NULL", pgx.NamedArgs{"id": f.task, "due": f.now}) != 1 {
		t.Fatal("human date was cleared")
	}
}

func TestReviewedLegacyRepairRefusesAnEarlierRulesVersion(t *testing.T) {
	f := seedLegacyRepair(t, false)
	operator := repairOperator(t, f.e)
	review, _ := f.preview(t)
	review.RuleVersion = "2:earlier-rules"
	task := homeRepairDataTask(f.e.Pool, func() time.Time { return f.now })
	_, engine := probeEngine(t, f.e, &f.now, task)
	err := engine.Activate(operator, task.ID, task.Steps[0].Name, 1, review)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("earlier rules accepted: %v", err)
	}
}

func TestReviewedLegacyActivationRefusesActionsAnOlderClientDidNotAcknowledge(t *testing.T) {
	for _, mode := range []string{"old client", "partial acknowledgement", "all displayed actions"} {
		t.Run(mode, func(t *testing.T) {
			f := seedLegacyRepair(t, false)
			seedLegacyRepairOn(t, f.e, f.now, true)
			operator := repairOperator(t, f.e)
			handler := dataTaskHandlers{pool: f.e.Pool, clock: func() time.Time { return f.now }}
			wire := legacyRepairWire(operator, t, handler)
			if len(wire.Tasks) != 2 || wire.RequiresActionReview == nil || !*wire.RequiresActionReview {
				t.Fatal("mixed preview lost its client capability boundary")
			}
			task := homeRepairDataTask(f.e.Pool, func() time.Time { return f.now })
			system, engine := probeEngine(t, f.e, &f.now, task)
			activation := crmcontracts.ActivateHomeRepairJSONBody{Version: 1, Review: wire}
			switch mode {
			case "partial acknowledgement":
				activation.ReviewedActions = &[]string{string(wire.Tasks[0].Action)}
			case "all displayed actions":
				actions := make([]string, 0, len(wire.Tasks))
				for _, row := range wire.Tasks {
					actions = append(actions, string(row.Action))
				}
				activation.ReviewedActions = &actions
			}
			body, err := json.Marshal(activation)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ActivateHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/activate", bytes.NewReader(body)).WithContext(operator))
			if mode != "all displayed actions" {
				if response.Code != http.StatusConflict {
					t.Fatalf("unsupported review accepted: %d %s", response.Code, response.Body.String())
				}
				if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NULL AND source_activity_id IS NULL AND due_at IS NOT NULL AND NOT is_done") != 2 {
					t.Fatal("unsupported review changed work")
				}
				return
			}
			if response.Code != http.StatusNoContent {
				t.Fatalf("acknowledged review refused: %d %s", response.Code, response.Body.String())
			}
			if _, err := engine.Execute(system); err != nil {
				t.Fatal(err)
			}
			if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NOT NULL AND NOT is_done") != 1 {
				t.Fatal("mixed review did not archive its obsolete task")
			}
			if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NULL AND source_activity_id IS NOT NULL AND due_at IS NULL AND NOT is_done") != 1 {
				t.Fatal("mixed review did not normalize its current task")
			}
		})
	}
}
