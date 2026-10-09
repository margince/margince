// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// yearTenThousand is an instant encoding/json refuses to render.
var yearTenThousand = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestAResponseThatCannotBeEncodedAnswersAFiveHundredProblem(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Location", "/v1/tasks/x")
	w.Header().Set("ETag", `"7"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	WriteJSON(w, http.StatusCreated, struct {
		Data []time.Time `json:"data"`
	}{Data: []time.Time{yearTenThousand}})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: an unencodable body must not answer a success", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type %q, want application/problem+json", got)
	}
	var body problem
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != "internal" {
		t.Fatalf("body %q (decode err %v), want the internal problem", w.Body.String(), err)
	}
	// The headers described the created record, which the 500 does not hand over.
	if w.Header().Get("Location") != "" || w.Header().Get("ETag") != "" {
		t.Fatalf("the 500 kept the success headers: %v", w.Header())
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("the 500 dropped a header every response carries")
	}
}

func TestAnEncodableResponseIsWrittenWithItsStatus(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]string{"id": "x"})

	if w.Code != http.StatusCreated || strings.TrimSpace(w.Body.String()) != `{"id":"x"}` {
		t.Fatalf("got %d %q, want 201 {\"id\":\"x\"}", w.Code, w.Body.String())
	}
}

// A problem whose details cannot be encoded still answers its own status and
// code, because those are the half a client branches on.
func TestAProblemWithUnencodableDetailsKeepsItsStatusAndCode(t *testing.T) {
	w := httptest.NewRecorder()
	writeProblem(w, problem{Status: http.StatusConflict, Code: "stale", Details: map[string]any{"at": yearTenThousand}})

	var body problem
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q does not decode: %v", w.Body.String(), err)
	}
	if w.Code != http.StatusConflict || body.Code != "stale" || body.Details != nil {
		t.Fatalf("got %d %+v, want 409 stale without details", w.Code, body)
	}
}

func TestADecodedDateTimeNoZoneCanRenderIsRefusedWithItsField(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/tasks", strings.NewReader(`{"due_at":"9999-12-31T23:59:59Z"}`))
	var into struct {
		DueAt *time.Time `json:"due_at"`
	}
	if Decode(w, r, &into) {
		t.Fatal("a due_at no zone can render was accepted")
	}
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"field":"due_at"`) ||
		!strings.Contains(w.Body.String(), `"out_of_range"`) {
		t.Fatalf("got %d %s, want 422 out_of_range naming due_at", w.Code, w.Body.String())
	}
}
