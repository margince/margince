// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// uploadCSV posts one file to the upload operation and returns the profile.
func uploadCSV(t *testing.T, e *apptest.AppEnv, object, body string) (importProfileDTO, int) {
	t.Helper()
	raw, status := postCSVFile(t, e, object, body)
	var profile importProfileDTO
	if len(raw) > 0 && status == http.StatusOK {
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("decoding %q: %v", raw, err)
		}
	}
	return profile, status
}

// uploadFieldError is one field refusal in an upload's problem document.
type uploadFieldError struct {
	Code  string `json:"code"`
	Field string `json:"field"`
}

// uploadRefusal posts a file and returns the status with the field refusals.
func uploadRefusal(t *testing.T, e *apptest.AppEnv, object, body string) (int, []uploadFieldError) {
	t.Helper()
	raw, status := postCSVFile(t, e, object, body)
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

// postCSVFile sends one file to the upload operation and returns the raw answer.
func postCSVFile(t *testing.T, e *apptest.AppEnv, object, body string) ([]byte, int) {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	if err := form.WriteField("object", object); err != nil {
		t.Fatalf("writing the object field: %v", err)
	}
	part, err := form.CreateFormFile("file", "estate.csv")
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := part.Write([]byte(body)); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.TS.URL+"/v1/imports/sources", &buf)
	if err != nil {
		t.Fatalf("building the upload: %v", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	//nolint:bodyclose // apptest.CloseBody closes it in the deferred call below, which the checker cannot follow across the helper.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	defer apptest.CloseBody(t, resp)

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the upload response: %v", err)
	}
	return raw, resp.StatusCode
}
