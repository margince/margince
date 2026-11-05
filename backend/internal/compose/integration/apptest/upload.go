// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// PostUpload sends one multipart body to a route and returns its status and
// response text. Nil headers send it as the signed-in admin.
func PostUpload(t *testing.T, e *AppEnv, path string, headers map[string]string, form *bytes.Buffer, contentType string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.TS.URL+path, form)
	if err != nil {
		t.Fatalf("building the upload: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	//nolint:bodyclose // CloseBody closes it in the deferred call below, which the checker cannot follow across the helper.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	defer CloseBody(t, resp)
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the upload response: %v", err)
	}
	return resp.StatusCode, string(raw)
}

// MultipartDocument is one file part, named filename and typed contentType.
func MultipartDocument(t *testing.T, filename string, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreatePart(filePartHeader(filename, contentType))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// filePartHeader builds the file part's headers by hand because the content
// type is what a route's type gate reads, and multipart.CreateFormFile
// hard-codes application/octet-stream.
func filePartHeader(filename, contentType string) map[string][]string {
	return map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	}
}

// MultipartAttachment builds a multipart/form-data upload body.
func MultipartAttachment(t *testing.T, entityType, entityID, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("entity_type", entityType); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("entity_id", entityID); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// UploadAs posts one file to /v1/attachments, as the passport when bearer is
// set and as the signed-in admin when it is nil.
func UploadAs(t *testing.T, e *AppEnv, bearer map[string]string, entityType, entityID string, content []byte) (int, json.RawMessage) {
	t.Helper()
	form, contentType := MultipartAttachment(t, entityType, entityID, "scan.pdf", content)
	status, raw := PostUpload(t, e, "/v1/attachments", bearer, form, contentType)
	return status, json.RawMessage(raw)
}

// AttachmentIDOf is the id an attachment upload answered.
func AttachmentIDOf(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var att struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &att); err != nil || att.ID == "" {
		t.Fatalf("the upload answered no attachment id (%v): %s", err, raw)
	}
	return att.ID
}
