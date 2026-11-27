// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/taskexplain"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// scriptedExplanation answers with one fact citing the first ref the request
// showed, and keeps what it was asked.
type scriptedExplanation struct {
	text  string
	calls int
	saw   string
}

func (s *scriptedExplanation) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.calls++
	s.saw = req.Messages[0].Content
	return model.Response{Text: `{"sentences":[{"text":"` + s.text +
		`","nature":"fact","evidence":[{"entity_type":"activity","entity_id":"a1"}]}]}`}, nil
}

// A task filed from a mail is explained from that mail, once per reader and
// input. A reader who may not open the task gets no explanation.
func TestATaskIsExplainedFromTheMailItCameFrom(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	contact := seedBareContact(t, e)
	now := clocktest.Now(t).UTC()
	link := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	subject, body, direction := "Re: Acceptance", "Once the shift leads sign off on the rota export I will send the signed acceptance.", "inbound"
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &now,
		Source: "manual", Links: link,
	})
	if err != nil {
		t.Fatal(err)
	}
	title, due, source := "Follow up on the acceptance", now.Add(48*time.Hour), ids.UUID(mail.Id)
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &title, DueAt: &due, Source: "manual", SourceActivityID: &source, Links: link,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	lane := &scriptedExplanation{text: "Jonas promised the signed acceptance once his shift leads sign off on the rota export."}
	svc := taskexplain.NewService(e.Pool, activities.NewStore(e.DB()), taskOpenService(e, now), lane,
		func() string { return "test" }, func() time.Time { return now }).WithEmailSummaries(activities.NewStore(e.DB()))
	handlers := taskexplain.NewHandlers(svc)

	var got crmcontracts.TaskExplanation
	for range 2 {
		rec := httptest.NewRecorder()
		handlers.GetTaskExplanation(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(e.Admin()), task.Id)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
	}
	if lane.calls != 1 {
		t.Errorf("the model was asked %d times about one unchanged task, want once", lane.calls)
	}
	if !strings.Contains(lane.saw, "rota export") {
		t.Errorf("the request never showed the source mail's words: %s", lane.saw)
	}
	if got.GeneratedBy != crmcontracts.WrittenByModel || len(got.Sentences) != 1 || got.Sentences[0].Text != lane.text {
		t.Fatalf("explanation = %+v, want the model's sentence", got)
	}

	contactOnly := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	if _, err := svc.Get(contactOnly, taskID); !errors.Is(err, apperrors.ErrPermissionDenied) && !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a reader who may not open the task got %v, want a refusal", err)
	}
}
