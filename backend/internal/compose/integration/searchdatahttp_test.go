// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSearchDataRefusesMissingGrantsAndFallsBackWithoutAModel(t *testing.T) {
	for _, test := range []struct {
		name, body string
		grants     map[string]principal.ObjectGrant
		status     int
	}{
		{"blank", `{"question":" "}`, nil, http.StatusUnprocessableEntity},
		{"no read grant", `{"question":"How many deals?"}`, nil, http.StatusForbidden},
		{"no installation grant", `{"question":"How many deals?"}`, map[string]principal.ObjectGrant{"deal": {Read: true}}, http.StatusForbidden},
		{"no model", `{"question":"How many deals?"}`, map[string]principal.ObjectGrant{"deal": {Read: true}, "installation_settings": {Read: true}}, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/search/data-answer", strings.NewReader(test.body))
			ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: test.grants}})
			rec := httptest.NewRecorder()
			(compose.Server{}).AskSearchData(rec, req.WithContext(ctx))
			if rec.Code != test.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if test.status == http.StatusOK && !strings.Contains(rec.Body.String(), `"outcome":"unsupported"`) {
				t.Fatal(rec.Body.String())
			}
		})
	}
}
