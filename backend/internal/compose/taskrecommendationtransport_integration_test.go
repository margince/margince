// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

func TestSuggestedTaskReceiptsReachTheHTTPAndToolWriters(t *testing.T) {
	for _, transport := range []string{"http", "tool"} {
		for _, stale := range []bool{false, true} {
			t.Run(transport+map[bool]string{false: "/current", true: "/changed"}[stale], func(t *testing.T) {
				e := setupVerdicts(t)
				reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
				at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
				service := newDealStatusService(e.Pool, func() time.Time { return at })
				card, err := service.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
				if err != nil || card.Next == nil {
					t.Fatalf("current suggestion: %+v %v", card.Next, err)
				}
				encoded, err := json.Marshal(card.Next.Arguments)
				if err != nil {
					t.Fatal(err)
				}
				if stale {
					subject := "An agreed step"
					if _, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "task", Subject: &subject, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.calm}}}); err != nil {
						t.Fatal(err)
					}
				}
				before := e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task'")
				guard := taskRecommendationFrom(service)
				switch transport {
				case "http":
					handler := activities.NewHandlers(e.DB()).WithTaskRecommendation(guard)
					request := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(encoded)).WithContext(reader)
					response := httptest.NewRecorder()
					handler.CreateTask(response, request, crmcontracts.CreateTaskParams{})
					want := http.StatusCreated
					if stale {
						want = http.StatusConflict
					}
					if response.Code != want {
						t.Fatalf("task status=%d want%d: %s", response.Code, want, response.Body.String())
					}
				default:
					var fields map[string]any
					if err := json.Unmarshal(encoded, &fields); err != nil {
						t.Fatal(err)
					}
					fields["kind"] = "task"
					provider := activities.NewProvider(e.DB()).WithTaskRecommendation(guard)
					_, err := provider.Create(reader, datasource.CreateInput{EntityType: datasource.EntityActivity, Source: "manual", Fields: fields})
					if stale && !errors.Is(err, apperrors.ErrVersionSkew) {
						t.Fatalf("stale tool receipt: %v", err)
					}
					if !stale && err != nil {
						t.Fatal(err)
					}
				}
				want := before + 1
				if stale {
					want = before
				}
				if got := e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task'"); got != want {
					t.Fatalf("tasks=%d want%d", got, want)
				}
			})
		}
	}
}

func TestTheServedExtensionRejectsReceiptWritesInItsWeakTransaction(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	service := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := service.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
	if err != nil || card.Next == nil {
		t.Fatalf("current suggestion: %+v %v", card.Next, err)
	}
	encoded, err := json.Marshal(card.Next.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	var request crm.CreateActivityRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	request.Kind = crm.CreateActivityRequestKindTask
	before := e.WsCount(t, "SELECT count(*) FROM activity")
	runtime := runtimeFor(reader, coreUnit, coreVersion, coreVia, extensionRuntimeBinding{pool: e.Pool})
	err = runtime.Tx(reader, func(ctx context.Context, tx extension.Tx) error {
		_, err := tx.Core().Activities().Create(ctx, request)
		return err
	})
	if !errors.Is(err, extension.ErrInvalid) || e.WsCount(t, "SELECT count(*) FROM activity") != before {
		t.Fatalf("unsupported extension transaction wrote work or invited a refresh: %v", err)
	}
}

func TestExhaustedSuggestedTaskSerializationReturnsAConflictWithoutWrites(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	service := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := service.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
	if err != nil || card.Next == nil {
		t.Fatalf("current suggestion: %+v %v", card.Next, err)
	}
	encoded, err := json.Marshal(card.Next.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(t.Context(), `CREATE FUNCTION reject_demo_task() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN IF NEW.kind='task' THEN RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='demo serialization failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), "DROP TRIGGER IF EXISTS reject_demo_task ON activity; DROP FUNCTION IF EXISTS reject_demo_task()"); err != nil {
			t.Errorf("remove own database failure fixture: %v", err)
		}
	})
	if _, err := owner.Exec(t.Context(), "CREATE TRIGGER reject_demo_task BEFORE INSERT ON activity FOR EACH ROW EXECUTE FUNCTION reject_demo_task()"); err != nil {
		t.Fatal(err)
	}
	tasks := e.WsCount(t, "SELECT count(*) FROM activity")
	audits := e.WsCount(t, "SELECT count(*) FROM audit_log")
	events := e.WsCount(t, "SELECT count(*) FROM event_outbox")
	handler := activities.NewHandlers(e.DB()).WithTaskRecommendation(taskRecommendationFrom(service))
	request := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(encoded)).WithContext(reader)
	response := httptest.NewRecorder()
	handler.CreateTask(response, request, crmcontracts.CreateTaskParams{})
	if response.Code != http.StatusConflict {
		t.Fatalf("exhausted serialization status=%d: %s", response.Code, response.Body.String())
	}
	if e.WsCount(t, "SELECT count(*) FROM activity") != tasks || e.WsCount(t, "SELECT count(*) FROM audit_log") != audits || e.WsCount(t, "SELECT count(*) FROM event_outbox") != events {
		t.Fatal("a failed retry left domain, audit or outbox writes")
	}
}
