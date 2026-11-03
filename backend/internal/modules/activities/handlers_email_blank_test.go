// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The HTTP send door refuses a subject or body that is blank, as the tool door
// does, and still refuses a key that is missing.
func TestTheMailSendDoorRefusesABlankSubjectOrBody(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		json      string
		wantField string
	}{
		"spaces for a subject": {`{"subject":"   ","body":"Hello"}`, "subject"},
		"empty subject":        {`{"subject":"","body":"Hello"}`, "subject"},
		"newline for a body":   {`{"subject":"Hi","body":"\n "}`, "body"},
		"missing subject":      {`{"body":"Hello"}`, "subject"},
		"filled in":            {`{"subject":"Hi","body":"Hello"}`, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/emails", strings.NewReader(c.json))
			var req crmcontracts.SendEmailRequest
			if !httperr.Decode(w, r, &req) {
				t.Fatalf("decode refused %s: %s", c.json, w.Body.String())
			}
			if got := requiredSent(w, r, req.Subject, req.Body); got != (c.wantField == "") {
				t.Fatalf("requiredSent = %v for %s, want %v", got, c.json, c.wantField == "")
			}
			if c.wantField == "" {
				return
			}
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), c.wantField) {
				t.Fatalf("status %d body %s, want a 422 naming %s", w.Code, w.Body.String(), c.wantField)
			}
		})
	}
}
