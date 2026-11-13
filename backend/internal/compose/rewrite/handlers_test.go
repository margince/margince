// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rewrite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// restrictingLane refuses every request the way the router refuses one
// carrying a restricted contact.
type restrictingLane struct{}

func (restrictingLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, model.ErrSubjectRestricted
}

func post(t *testing.T, svc *Service, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(rep(true), http.MethodPost, "/v1/rewrites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewHandlers(svc).RewriteText(rec, req)
	return rec.Code, rec.Body.String()
}

const about = `"about": {"entity_type": "company", "entity_id": "018f3a1b-0000-7000-8000-000000000020"}`

func TestTheEndpointNamesWhyItCannotRewrite(t *testing.T) {
	note := `{"envelope": "note", "verb": "polish", "subject": "call anna", "language": "en", ` + about + `}`
	for name, tc := range map[string]struct {
		svc         *Service
		body        string
		status      int
		answerHolds string
	}{
		"no model bound":      {NewService(nil, asCompany), note, http.StatusConflict, `"ai_not_configured"`},
		"a restricted record": {NewService(restrictingLane{}, asCompany), note, http.StatusConflict, `"subject_restricted"`},
		"an empty note":       {NewService(nil, asCompany), `{"envelope": "note", "verb": "polish", "subject": " ", "language": "en", ` + about + `}`, http.StatusUnprocessableEntity, `"body"`},
		"an unknown language": {NewService(nil, asCompany), `{"envelope": "note", "verb": "polish", "subject": "x", "language": "fr", ` + about + `}`, http.StatusUnprocessableEntity, `"language"`},
		"a free instruction":  {NewService(nil, asCompany), `{"envelope": "note", "verb": "polish", "instruction": "say it is signed", "subject": "x", "language": "en", ` + about + `}`, http.StatusUnprocessableEntity, `instruction`},
		"a verb out of list":  {NewService(nil, asCompany), `{"envelope": "note", "verb": "say it is signed", "subject": "x", "language": "en", ` + about + `}`, http.StatusUnprocessableEntity, `"verb"`},
		"no record named":     {NewService(nil, asCompany), `{"envelope": "note", "verb": "polish", "subject": "x", "language": "en"}`, http.StatusUnprocessableEntity, `"about"`},
	} {
		status, answer := post(t, tc.svc, tc.body)
		if status != tc.status || !strings.Contains(answer, tc.answerHolds) {
			t.Errorf("%s: %d %s, want %d naming %s", name, status, answer, tc.status, tc.answerHolds)
		}
	}
}
