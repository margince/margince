// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// toolHealth is the health member of read_project_360, as an agent reads it.
type toolHealth struct {
	Judged       bool   `json:"judged"`
	AssessmentID string `json:"assessment_id"`
	State        string `json:"state"`
	Source       string `json:"source"`
	Note         string `json:"note"`
	// AssessedAt is nil when the member is absent.
	AssessedAt *time.Time `json:"assessed_at"`
}

func projectToolHealth(t *testing.T, e *Env, id ids.ProjectID) *toolHealth {
	t.Helper()
	out, err := compose.NewRegistry(e.Pool, compose.SendPath{}).
		Invoke(e.Admin(), "read_project_360", fmt.Appendf(nil, `{"project_id":%q}`, id.String()))
	if err != nil {
		t.Fatalf("read_project_360: %v", err)
	}
	var envelope struct {
		Data struct {
			Health *toolHealth `json:"health"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("reading the tool's answer: %v\n%s", err, out)
	}
	if envelope.Data.Health == nil {
		t.Fatalf("read_project_360 carries no health:\n%s", out)
	}
	return envelope.Data.Health
}

// An agent reads a project's health from the same read the page does. It sees
// an AI reading while it stands, and "not judged" once a user undid it.
func TestTheProjectToolReadsHealthAsThePageDoes(t *testing.T) {
	e := Setup(t)
	now := wallNow()
	erp := quietDeliveringProject(t, e, "ERP rollout", &e.Rep1, now)
	if got := projectToolHealth(t, e, erp); got.Judged {
		t.Fatalf("a project nobody judged reads %+v", got)
	}
	result, judged := aiHealthWrite(t, e, erp, now)
	if !judged {
		t.Fatalf("the health job wrote no reading: %+v", result)
	}
	got := projectToolHealth(t, e, erp)
	if !got.Judged || got.AssessmentID != result.Assessment.Id.String() ||
		got.State != "at_risk" || got.Source != "ai" || got.Note != "Quiet." ||
		got.AssessedAt == nil || !got.AssessedAt.Equal(result.Assessment.AssessedAt) {
		t.Fatalf("the AI reading reads %+v through the tool, assessed %v", got, result.Assessment.AssessedAt)
	}
	if _, err := e.Projects.UndoAIHealth(e.Admin(), erp, ids.UUID(result.Assessment.Id)); err != nil {
		t.Fatalf("undoing the AI reading: %v", err)
	}
	if currentHealth(t, e, erp) != nil {
		t.Fatal("the page still reads a current judgement after the undo, so the check below proves nothing")
	}
	if got := projectToolHealth(t, e, erp); got.Judged || got.AssessedAt != nil || got.AssessmentID != "" {
		t.Fatalf("an undone AI reading still reads %+v through the tool", got)
	}
}
