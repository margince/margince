// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestResponseTraceNamesTheServerScopeAndAccessLog(t *testing.T) {
	var output strings.Builder
	logger := slog.New(WithCorrelation(slog.NewJSONHandler(&output, nil)))
	var scoped string
	handler := Correlate(AccessLog(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := principal.CorrelationID(r.Context())
		if !ok {
			t.Fatal("request has no server trace scope")
		}
		scoped = id.String()
		w.WriteHeader(http.StatusNoContent)
	})))
	previous := ""
	for range 2 {
		output.Reset()
		request := httptest.NewRequest(http.MethodGet, "/v1/brief", nil)
		request.Header.Set("X-Correlation-ID", "untrusted-client-trace")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		trace := response.Header().Get("X-Correlation-ID")
		if trace == "" || trace != scoped || trace == previous || trace == "untrusted-client-trace" {
			t.Fatalf("response trace %q does not name this server request's fresh scope %q", trace, scoped)
		}
		var logged struct {
			CorrelationID string `json:"correlation_id"`
		}
		if err := json.Unmarshal([]byte(output.String()), &logged); err != nil {
			t.Fatal(err)
		}
		if logged.CorrelationID != trace {
			t.Fatalf("access log trace %q differs from response %q", logged.CorrelationID, trace)
		}
		previous = trace
	}
}
