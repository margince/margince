// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A passport lists and uploads a record's files over REST as it does over MCP,
// with list_documents and attach_document. Reading the bytes, deleting a file
// and the other attachment routes stay with a human.

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// fileRefusal is the slice of a problem body these tests branch on.
type fileRefusal struct {
	Code    string `json:"code"`
	Detail  string `json:"detail"`
	Details struct {
		Errors []struct{ Field, Code, Message string } `json:"errors"`
	} `json:"details"`
}

// uploadAs posts one file to /v1/attachments, as the passport when bearer is
// set and as the signed-in admin when it is nil.
func uploadAs(t *testing.T, e *apptest.AppEnv, bearer map[string]string, entityType, entityID string, content []byte) (int, json.RawMessage) {
	t.Helper()
	form, contentType := multipartAttachment(t, entityType, entityID, "scan.pdf", content)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.TS.URL+"/v1/attachments", form)
	if err != nil {
		t.Fatalf("building the upload: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range bearer {
		req.Header.Set(k, v)
	}
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
	return resp.StatusCode, raw
}

func attachmentIDOf(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var att struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &att); err != nil || att.ID == "" {
		t.Fatalf("the upload answered no attachment id (%v): %s", err, raw)
	}
	return att.ID
}

// restFileIDs reads the attachment ids of one listing route, in order. A deal's
// Files area wraps each attachment with its hidden flag, so its id sits one
// level down.
func (d *twoDoors) restFileIDs(t *testing.T, path string) []string {
	t.Helper()
	var page struct {
		Data []struct {
			ID         string `json:"id"`
			Attachment struct {
				ID string `json:"id"`
			} `json:"attachment"`
		} `json:"data"`
	}
	if err := json.Unmarshal(d.rest(t, "GET", path, nil), &page); err != nil {
		t.Fatalf("GET %s does not decode: %v", path, err)
	}
	out := make([]string, 0, len(page.Data))
	for _, a := range page.Data {
		out = append(out, cmp.Or(a.ID, a.Attachment.ID))
	}
	return out
}

func (d *twoDoors) toolFileIDs(t *testing.T, entityType, entityID string) []string {
	t.Helper()
	var page struct {
		Documents []struct {
			AttachmentID string `json:"attachment_id"`
		} `json:"documents"`
	}
	raw := d.tool(t, "list_documents", map[string]any{"entity_type": entityType, "entity_id": entityID})
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("list_documents does not decode: %v", err)
	}
	out := make([]string, 0, len(page.Documents))
	for _, doc := range page.Documents {
		out = append(out, doc.AttachmentID)
	}
	return out
}

// A passport uploads over REST, and the three listing routes name the same
// files list_documents names. The audit and outbox rows of the upload name
// the agent, not the human who lent it.
func TestAPassportListsAndUploadsFilesOverREST(t *testing.T) {
	d := newTwoDoors(t, "files-doors", "read", "write")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Filing Works", "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := createdID(t, d.e, "/v1/deals", AnyMap{
		"name": "Filing renewal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"company_id": company, "source": "manual",
	})
	if status, raw := uploadAs(t, d.e, nil, "company", company, []byte("PDF-FROM-A-HUMAN")); status != http.StatusCreated {
		t.Fatalf("the admin's upload → %d %s", status, raw)
	}
	status, raw := uploadAs(t, d.e, d.bearer, "deal", deal, []byte("PDF-FROM-AN-AGENT"))
	if status != http.StatusCreated {
		t.Fatalf("the passport's upload → %d %s", status, raw)
	}
	uploaded := attachmentIDOf(t, raw)

	for _, tc := range []struct{ entityType, entityID, path string }{
		{"company", company, "/v1/companies/" + company + "/documents"},
		{"deal", deal, "/v1/deals/" + deal + "/documents"},
		{"deal", deal, "/v1/attachments?entity_type=deal&entity_id=" + deal},
	} {
		rest, tool := d.restFileIDs(t, tc.path), d.toolFileIDs(t, tc.entityType, tc.entityID)
		if !slices.Equal(rest, tool) {
			t.Errorf("GET %s lists %v, list_documents lists %v", tc.path, rest, tool)
		}
	}
	if company := d.restFileIDs(t, "/v1/companies/"+company+"/documents"); len(company) != 2 || !slices.Contains(company, uploaded) {
		t.Errorf("the company's library lists %v, want the human's file and the agent's %s", company, uploaded)
	}
	assertTheUploadNamesTheAgent(t, d.e, uploaded)
}

func assertTheUploadNamesTheAgent(t *testing.T, e *apptest.AppEnv, attachment string) {
	t.Helper()
	var actorType, actorID string
	if err := e.Owner.QueryRow(t.Context(), `SELECT actor_type, actor_id FROM audit_log
		WHERE entity_id = $1 AND action = 'create'`, attachment).Scan(&actorType, &actorID); err != nil {
		t.Fatalf("reading the upload's audit row: %v", err)
	}
	if actorType != "agent" {
		t.Errorf("the upload's audit row names a %s, want the agent", actorType)
	}
	var envelopes int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM event_outbox
		WHERE envelope::text LIKE '%' || $1 || '%' AND envelope::text LIKE '%' || $2 || '%'`,
		attachment, actorID).Scan(&envelopes); err != nil {
		t.Fatalf("reading the upload's outbox row: %v", err)
	}
	if envelopes == 0 {
		t.Errorf("no outbox event of attachment %s names the actor %s its audit row names", attachment, actorID)
	}
}

// A passport lent without `write` is refused the upload by its cap, before the
// file is read.
func TestAReadOnlyPassportCannotUploadAFile(t *testing.T) {
	d := newTwoDoors(t, "files-read-only", "read")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Read Only Works", "source": "manual"})
	status, raw := uploadAs(t, d.e, d.bearer, "company", company, []byte("PDF"))
	var refusal fileRefusal
	if err := json.Unmarshal(raw, &refusal); err != nil {
		t.Fatalf("the refusal does not decode: %v\n%s", err, raw)
	}
	if status != http.StatusForbidden || refusal.Code != scopeRefusalCode {
		t.Errorf("a read-only passport's upload → %d %q, want 403 %s", status, refusal.Code, scopeRefusalCode)
	}
}

// A record the lending human cannot see is absent to the passport. Its files
// list as not found, and a file sent to it lands nowhere.
func TestAFileOnARecordOutsideTheRowScopeIsNotFound(t *testing.T) {
	d := newTwoDoors(t, "files-hidden", "read", "write")
	d.e.DescribeCompany(t)
	colleague := createdID(t, d.e, "/v1/users", AnyMap{
		"email": "colleague@files-hidden.test", "display_name": "Colleague", "role": "rep",
	})
	var hidden string
	if err := d.e.Owner.QueryRow(t.Context(), `
		INSERT INTO contact (full_name, owner_id, visibility, source, captured_by)
		VALUES ('Private Contact', $1, 'owner', 'manual', 'connector:gmail') RETURNING id`,
		colleague).Scan(&hidden); err != nil {
		t.Fatalf("seeding a contact captured into the colleague's mailbox: %v", err)
	}

	if status := d.e.Call(t, "GET", "/v1/attachments?entity_type=contact&entity_id="+hidden, nil, d.bearer, nil); status != http.StatusNotFound {
		t.Errorf("listing a hidden contact's files → %d, want 404", status)
	}
	if status, raw := uploadAs(t, d.e, d.bearer, "contact", hidden, []byte("PDF")); status != http.StatusNotFound {
		t.Errorf("uploading to a hidden contact → %d %s, want 404", status, raw)
	}
	var stored int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM attachment WHERE entity_id = $1`, hidden).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Errorf("%d files landed on a contact the passport cannot see", stored)
	}
}

// Downloading and deleting stay human-only: no tool reads a file's bytes, and
// no route lets a passport do it either.
func TestAPassportCannotDownloadOrDeleteAFile(t *testing.T) {
	d := newTwoDoors(t, "files-human-only", "read", "write")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Kept Works", "source": "manual"})
	status, raw := uploadAs(t, d.e, nil, "company", company, []byte("PDF-KEPT"))
	if status != http.StatusCreated {
		t.Fatalf("the admin's upload → %d %s", status, raw)
	}
	file := attachmentIDOf(t, raw)
	for _, method := range []string{"GET", "DELETE"} {
		var refusal fileRefusal
		if status := d.e.Call(t, method, "/v1/attachments/"+file, nil, d.bearer, &refusal); status != http.StatusForbidden || refusal.Code != "permission_denied" {
			t.Errorf("%s /v1/attachments/{id} as a passport → %d %q, want 403 permission_denied", method, status, refusal.Code)
		}
	}
}

// A file over what attach_document takes is refused on REST in the tool's own
// words, as a 422 on the part that carried it. A human's upload of the same
// file keeps the route's larger limit.
func TestAnAgentUploadOverTheToolsLimitIsRefusedAsTheToolRefusesIt(t *testing.T) {
	d := newTwoDoors(t, "files-over-cap", "read", "write")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Heavy Works", "source": "manual"})
	limit := agents.AttachLimit(uploadCeiling)
	content := fileOf(int(limit) + 1024)

	status, raw := uploadAs(t, d.e, d.bearer, "company", company, content)
	var refusal fileRefusal
	if err := json.Unmarshal(raw, &refusal); err != nil {
		t.Fatalf("the refusal does not decode: %v\n%s", err, raw)
	}
	errs := refusal.Details.Errors
	if status != http.StatusUnprocessableEntity || len(errs) != 1 || errs[0].Field != "file" || errs[0].Code != "validation_error" {
		t.Fatalf("an over-limit agent upload → %d %+v, want 422 validation_error on file", status, errs)
	}
	overLimit := "over the " + httperr.Megabytes(limit) + " an agent may attach"
	if !strings.Contains(errs[0].Message, overLimit) {
		t.Errorf("the REST refusal says %q, want it to say %q", errs[0].Message, overLimit)
	}
	viaTool := d.mcp.CallRefused(t, "attach_document", map[string]any{
		"entity_type": "company", "entity_id": company, "filename": "scan.pdf",
		"content_type": "application/pdf", "content_base64": base64.StdEncoding.EncodeToString(content),
	})
	if !strings.Contains(viaTool, overLimit) {
		t.Errorf("attach_document refused with %q, want it to say %q as REST does", viaTool, overLimit)
	}

	if status, raw := uploadAs(t, d.e, nil, "company", company, content); status != http.StatusCreated {
		t.Errorf("a human's upload of the same file → %d %s, want 201 under the route's own limit", status, bytes.TrimSpace(raw))
	}
}
