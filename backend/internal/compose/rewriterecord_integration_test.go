// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A rewrite naming a record the caller may not read answers as an unknown
// id. So the endpoint never says whether a hidden record exists or is
// restricted. A record the caller reads goes on to the model check.
func TestPolishAnswersAnUnreadableRecordAsAnUnknownOne(t *testing.T) {
	e := integration.Setup(t)
	colleagues := e.SeedContact(t, "Private to a colleague", &e.Rep2)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, colleagues)
	own := e.SeedContact(t, "Anna Berg", &e.Rep1)
	rep := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "activity": {Create: true, Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	handlers := newRewriteHandlers(e.Pool, nil)
	ask := func(contact ids.UUID) (int, string) {
		body := `{"envelope": "note", "verb": "polish", "subject": "call anna", "language": "en",
			"about": {"entity_type": "contact", "entity_id": "` + contact.String() + `"}}`
		req := httptest.NewRequestWithContext(rep, http.MethodPost, "/v1/rewrites", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handlers.RewriteText(rec, req)
		return rec.Code, rec.Body.String()
	}
	hiddenCode, hidden := ask(colleagues)
	unknownCode, unknown := ask(ids.NewV7())
	if hiddenCode != http.StatusNotFound || hiddenCode != unknownCode || stripTrace(hidden) != stripTrace(unknown) {
		t.Errorf("a hidden contact answered %d %s, an unknown one %d %s; want one 404 for both",
			hiddenCode, hidden, unknownCode, unknown)
	}
	if code, answer := ask(own); code != http.StatusConflict || !strings.Contains(answer, "ai_not_configured") {
		t.Errorf("the caller's own contact answered %d %s, want it admitted to the model check", code, answer)
	}
}

// stripTrace drops the per-request ids a problem body carries, so two answers
// compare on what they say.
func stripTrace(body string) string {
	var kept []string
	for field := range strings.SplitSeq(body, ",") {
		if !strings.Contains(field, `"trace_id"`) && !strings.Contains(field, `"instance"`) && !strings.Contains(field, `"correlation_id"`) {
			kept = append(kept, field)
		}
	}
	return strings.Join(kept, ",")
}

// restrictionCheckedLane answers only after the restriction check the router
// runs, the real one over the stops the consent module records.
type restrictionCheckedLane struct{ check subjectRestrictions }

func (l restrictionCheckedLane) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if req.Subjects == nil {
		return model.Response{}, model.ErrSubjectsUndeclared
	}
	restricted, err := l.check.AnyRestricted(ctx, *req.Subjects)
	if err != nil || restricted {
		return model.Response{}, errors.Join(model.ErrSubjectRestricted, err)
	}
	return model.Response{Text: `{"subject": "Call Anna about pricing", "body": ""}`}, nil
}

// A note polished on a restricted lead is refused before a model reads it. No
// consent door writes a lead's stop yet, so the row is written as the table holds it.
func TestARewriteOnALeadHonoursItsRestriction(t *testing.T) {
	e := integration.Setup(t)
	lead := func(name, address string) ids.UUID {
		t.Helper()
		created, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new(name), Email: new(address), Source: "manual"})
		if err != nil {
			t.Fatalf("creating the lead: %v", err)
		}
		return ids.UUID(created.Id)
	}
	restricted, free := lead("Anna Weber", "anna@lead.example"), lead("Ben Roth", "ben@lead.example")
	e.WsExec(t, `INSERT INTO communication_suppression (lead_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, restricted)
	handlers := newRewriteHandlers(e.Pool, restrictionCheckedLane{check: subjectRestrictions{pool: e.Pool}})
	ask := func(about ids.UUID) (int, string) {
		body := `{"envelope": "note", "verb": "polish", "subject": "call anna re pricing", "language": "en",
			"about": {"entity_type": "lead", "entity_id": "` + about.String() + `"}}`
		req := httptest.NewRequestWithContext(e.Admin(), http.MethodPost, "/v1/rewrites", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handlers.RewriteText(rec, req)
		return rec.Code, rec.Body.String()
	}
	if code, answer := ask(restricted); code != http.StatusConflict || !strings.Contains(answer, "subject_restricted") {
		t.Errorf("a restricted lead answered %d %s, want 409 subject_restricted", code, answer)
	}
	if code, answer := ask(free); code != http.StatusOK {
		t.Errorf("an unrestricted lead answered %d %s, want 200", code, answer)
	}
}
