// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// settleTries bounds how often CallUntilSettled sends one request.
const settleTries = 10

// CallUntilSettled is Call, sent again while the server answers a 503 whose
// code it lists as transient. Such an answer says nothing was written and the
// same request will succeed later. Seeds that run in parallel meet them. A test
// about the refusal itself uses Call.
//
//craft:ignore naked-any the test transport seam: body and out are whichever shapes the route takes
func (e *AppEnv) CallUntilSettled(t *testing.T, method, path string, body any, headers map[string]string, out any) int {
	t.Helper()
	var raw json.RawMessage
	status := e.Call(t, method, path, body, headers, &raw)
	for try := 1; try < settleTries && sendAgain(status, raw); try++ {
		raw = nil
		status = e.Call(t, method, path, body, headers, &raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decoding %q: %v", method, path, raw, err)
		}
	}
	return status
}

// sendAgain reports the answer CallUntilSettled repeats a request on.
func sendAgain(status int, raw json.RawMessage) bool {
	var problem struct {
		Code string `json:"code"`
	}
	return status == http.StatusServiceUnavailable && json.Unmarshal(raw, &problem) == nil &&
		httperr.TransientCode(problem.Code)
}
