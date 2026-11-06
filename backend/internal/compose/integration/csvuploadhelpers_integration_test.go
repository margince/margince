// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// uploadFieldError is one field refusal in an upload's problem document.
type uploadFieldError struct {
	Code  string `json:"code"`
	Field string `json:"field"`
}

// uploadRefusal posts a file and returns the status with the field refusals.
func uploadRefusal(t *testing.T, e *apptest.AppEnv, object, body string) (int, []uploadFieldError) {
	t.Helper()
	raw, status := importjobtest.PostCSVFile(t, e, object, body)
	var problem struct {
		Details struct {
			Errors []uploadFieldError `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(raw, &problem); err != nil {
		t.Fatalf("decoding %q: %v", raw, err)
	}
	return status, problem.Details.Errors
}
