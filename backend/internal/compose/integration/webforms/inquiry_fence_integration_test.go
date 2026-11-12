// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webforms"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestAHandlingCutoverRefusesAnIntakeWithoutAcknowledgingOrLosingIt(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	id, err := ids.Parse(form.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := e.DB().Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(t.Context(), workspace.UUID)
	body := url.Values{"email": {"waiting@visitor.example"}, "message": {"Please call me"}, "_mrg_id": {"stable-request-01"}}
	held, err := e.DB().WithWorkLease(ctx, provenance.WebFormSource, id, slog.New(slog.DiscardHandler), func(context.Context) error {
		answer := browserPost(t, e, form.FormKey, body, siteOrigin)
		if answer.Status != http.StatusServiceUnavailable {
			t.Fatalf("contended submission = %d %s", answer.Status, answer.Body)
		}
		if answer.Header.Get("Access-Control-Allow-Origin") != siteOrigin {
			t.Fatal("the allowed browser cannot read the retry response")
		}
		if count, _, _ := leadsWithEmail(t, e, "waiting@visitor.example"); count != 0 {
			t.Fatalf("unacknowledged submission created %d leads", count)
		}
		return nil
	})
	if err != nil || !held {
		t.Fatalf("hold form handling: held=%v, error=%v", held, err)
	}
	answer := browserPost(t, e, form.FormKey, body, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("retried submission = %d %s", answer.Status, answer.Body)
	}
	if count, _, _ := leadsWithEmail(t, e, "waiting@visitor.example"); count != 1 {
		t.Fatalf("retry created %d leads", count)
	}
	if notes := notesOnLeadWithEmail(t, e, "waiting@visitor.example"); len(notes) != 1 {
		t.Fatalf("retry created %d notes", len(notes))
	}
}

func TestLegacyLeadHandlersWaitForTheAcceptedWebsiteNote(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	for _, key := range []string{"route_lead", "assign_lead_owner"} {
		var instance struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		}
		if status := e.Call(t, "POST", "/v1/automations", integration.AnyMap{"key": key, "name": key, "params": integration.AnyMap{}}, nil, &instance); status != http.StatusCreated {
			t.Fatalf("create %s: %d", key, status)
		}
		if status := e.Call(t, "PATCH", "/v1/automations/"+instance.ID, integration.AnyMap{"status": "enabled"}, map[string]string{"If-Match": strconv.FormatInt(instance.Version, 10)}, nil); status != http.StatusOK {
			t.Fatalf("enable %s: %d", key, status)
		}
	}
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"legacy@visitor.example"}, "message": {"Please call"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	var raw json.RawMessage
	if err := e.Owner.QueryRow(t.Context(), `SELECT envelope FROM event_outbox WHERE envelope->>'type'='lead.created' AND envelope->'entity'->>'id'=(SELECT id::text FROM lead WHERE email='legacy@visitor.example')`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event kevents.Envelope
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	workspace, err := e.DB().Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(t.Context(), workspace.UUID)
	id, err := ids.Parse(form.ID)
	if err != nil {
		t.Fatal(err)
	}
	engine := compose.NewWorkflowEngine(e.DB())
	held, err := e.DB().WithWorkLease(ctx, provenance.WebFormSource, id, slog.New(slog.DiscardHandler), func(context.Context) error {
		if err := engine.HandleEvent(ctx, event); !errors.Is(err, webforms.ErrHandlingBusy) {
			t.Fatalf("legacy firing during intake: %v", err)
		}
		var count int
		if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM workflow_run WHERE handler IN ('route_lead','assign_lead_owner')`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("blocked legacy handlers claimed %d runs", count)
		}
		return nil
	})
	if err != nil || !held {
		t.Fatalf("hold intake: %v %v", held, err)
	}
	for range 2 {
		if err := engine.HandleEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	var tasks int
	if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id=a.id WHERE a.kind='task' AND l.lead_id=$1`, event.Entity.ID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 {
		t.Fatalf("legacy replay created %d tasks, want one", tasks)
	}
}
