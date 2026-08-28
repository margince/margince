// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The migrate-in surface end to end: upload a file, map it, dry-run it,
// approve it — and the promises that make it safe to hand a customer's estate
// to. Every assertion here is one the chapter makes by number:
//
//	IEM-AC-7  — the dry run writes NOTHING, and each object writes its own table
//	IEM-AC-9  — a re-run creates no duplicates
//	IEM-WIRE-5 — approval is valid only from awaiting_approval
//
// The counts are read back over HTTP rather than out of the pool, because what
// the customer sees is the list, not the table.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

type importColumnDTO struct {
	Header   string   `json:"header"`
	FillRate float64  `json:"fill_rate"`
	Samples  []string `json:"samples"`
}

type importProfileDTO struct {
	SourceRef        string            `json:"source_ref"`
	Object           string            `json:"object"`
	Columns          []importColumnDTO `json:"columns"`
	RowsProfiled     int               `json:"rows_profiled"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
	Targets          []string          `json:"targets"`
}

type importRunDTO struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Connector  string `json:"connector"`
	Object     string `json:"object"`
	Checkpoint int    `json:"checkpoint"`
}

type importDispositionDTO struct {
	Created    int  `json:"created"`
	Updated    int  `json:"updated"`
	Unchanged  int  `json:"unchanged"`
	Skipped    int  `json:"skipped"`
	Duplicates *int `json:"duplicates"`
}

type importIssueDTO struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

type importLinksDTO struct {
	Offered    int `json:"offered"`
	Applied    int `json:"applied"`
	Unresolved []struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Reason string `json:"reason"`
	} `json:"unresolved"`
}

type importReportDTO struct {
	Links         *importLinksDTO      `json:"links"`
	RunID         string               `json:"run_id"`
	Status        string               `json:"status"`
	RowsRead      int                  `json:"rows_read"`
	Disposition   importDispositionDTO `json:"disposition"`
	Issues        []importIssueDTO     `json:"issues"`
	SourceKeyUsed string               `json:"source_key_used"`
}

type leadListDTO struct {
	Data []struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Title    string `json:"title"`
	} `json:"data"`
}

func setupImportApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e, _ := setupImportAppWithStore(t)
	return e
}

// setupImportAppWithStore hands the suite the same object store the server
// holds, so a scenario can make an uploaded file disappear the way a retention
// sweep or an operator would.
func setupImportAppWithStore(t *testing.T) (*apptest.AppEnv, blobstore.Store) {
	t.Helper()
	store := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(store))
	e.BootstrapWorkspace(t)
	return e, store
}

// uploadCSV posts one file to the upload operation and returns the profile.
func uploadCSV(t *testing.T, e *apptest.AppEnv, object, body string) (importProfileDTO, int) {
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
	var profile importProfileDTO
	if len(raw) > 0 && resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("decoding %q: %v", raw, err)
		}
	}
	return profile, resp.StatusCode
}

// createRunOnDuplicate is createRunWithMapping for a run that names a duplicate
// policy. Separate rather than a variadic so every existing call site keeps
// saying, in its own text, that it takes the default.
func createRunOnDuplicate(t *testing.T, e *apptest.AppEnv, object, sourceRef string,
	mapping map[string]string, onDuplicate string,
) (importRunDTO, int) {
	t.Helper()
	var run importRunDTO
	status := e.Call(t, http.MethodPost, "/v1/imports", map[string]any{
		"connector": "csv", "object": object, "source_ref": sourceRef,
		"mapping": mapping, "on_duplicate": onDuplicate,
	}, nil, &run)
	return run, status
}

func createRunWithMapping(t *testing.T, e *apptest.AppEnv, object, sourceRef string, mapping map[string]string) (importRunDTO, int) {
	t.Helper()
	var run importRunDTO
	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": object,
		"source_ref": sourceRef, "mapping": mapping,
	}, nil, &run)
	return run, status
}

func leadCount(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var leads leadListDTO
	if status := e.Call(t, http.MethodGet, "/v1/leads?limit=100", nil, nil, &leads); status != http.StatusOK {
		t.Fatalf("GET /v1/leads → %d, want 200", status)
	}
	return len(leads.Data)
}

func importedPersonCount(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var people struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/people?limit=100", nil, nil, &people); status != http.StatusOK {
		t.Fatalf("GET /v1/people → %d, want 200", status)
	}
	return len(people.Data)
}

const prospectCSV = "Email,Full Name,Title\n" +
	"ada@lovelace.example,Ada Lovelace,Analyst\n" +
	"grace@hopper.example,Grace Hopper,Rear Admiral\n" +
	"katherine@johnson.example,Katherine Johnson,Mathematician\n"

// The two promises that make an import safe to run against a real estate: the
// dry run writes NOTHING, and a `lead` run writes LEADS — it does not reach the
// person table at all.
//
// That second half is scoped to `object: lead` deliberately. A file the business
// already knows imports as `object: person` and creates people, which
// TestCSVImportOfKnownPeopleCreatesPeople below asserts. What must never happen
// is one object quietly writing the other's table.
func TestCSVImportDryRunWritesNothingAndCommitsLeads(t *testing.T) {
	e := setupImportApp(t)
	peopleBefore := importedPersonCount(t, e)

	profile, status := uploadCSV(t, e, "lead", prospectCSV)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	if profile.RowsProfiled != 3 || len(profile.Columns) != 3 {
		t.Fatalf("profile = %+v, want 3 rows and 3 columns", profile)
	}
	if profile.SuggestedMapping["Email"] != "email" || profile.SuggestedMapping["Full Name"] != "full_name" {
		t.Fatalf("suggested mapping = %v, want the normalized-name matches", profile.SuggestedMapping)
	}
	if profile.SourceRef == "" {
		t.Fatal("the upload returned no source_ref, so nothing can reference the stored file")
	}

	run, status := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	if run.Status != "awaiting_approval" {
		t.Fatalf("status = %q, want awaiting_approval", run.Status)
	}

	// The whole point of a dry run: it has told us what will happen and has
	// written none of it.
	if got := leadCount(t, e); got != 0 {
		t.Fatalf("the dry run created %d leads; a validation pass writes nothing", got)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 3 {
		t.Fatalf("report = %+v, want 3 rows the commit will create", report)
	}
	if report.SourceKeyUsed != "Email" {
		t.Fatalf("source key = %q, want the column mapped onto email", report.SourceKeyUsed)
	}

	var approved importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if got := leadCount(t, e); got != 3 {
		t.Fatalf("leads after approval = %d, want 3", got)
	}
	if got := importedPersonCount(t, e); got != peopleBefore {
		t.Fatalf("people = %d, was %d — a `lead` run writes leads and does not reach the person table", got, peopleBefore)
	}
}

// IEM-AC-9: the same file twice creates nothing the second time, and a
// CORRECTED file rewrites what changed. The second half is the one a frozen
// snapshot never has to answer — and the one a customer hits the moment they
// fix a typo and upload again.
func TestCSVImportReRunConvergesAndAnEditedFileUpdates(t *testing.T) {
	e := setupImportApp(t)

	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("first approve → %d, want 202", status)
	}
	if got := leadCount(t, e); got != 3 {
		t.Fatalf("leads = %d, want 3", got)
	}

	// The identical file again: recognized, and nothing created.
	same, _ := uploadCSV(t, e, "lead", prospectCSV)
	sameRun, _ := createRunWithMapping(t, e, "lead", same.SourceRef, same.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+sameRun.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("second approve → %d, want 202", status)
	}
	if got := leadCount(t, e); got != 3 {
		t.Fatalf("leads after re-import = %d, want 3 — the record map exists to prevent this", got)
	}

	// The corrected file: one title fixed, and the record must follow it.
	corrected := "Email,Full Name,Title\n" +
		"ada@lovelace.example,Ada Lovelace,Analyst\n" +
		"grace@hopper.example,Grace Hopper,Rear Admiral (upper half)\n" +
		"katherine@johnson.example,Katherine Johnson,Mathematician\n"
	edited, _ := uploadCSV(t, e, "lead", corrected)
	editedRun, _ := createRunWithMapping(t, e, "lead", edited.SourceRef, edited.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+editedRun.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("third approve → %d, want 202", status)
	}

	var leads leadListDTO
	if status := e.Call(t, http.MethodGet, "/v1/leads?limit=100", nil, nil, &leads); status != http.StatusOK {
		t.Fatalf("GET /v1/leads → %d", status)
	}
	if len(leads.Data) != 3 {
		t.Fatalf("leads = %d, want 3 — a correction updates, it does not duplicate", len(leads.Data))
	}
	found := false
	for _, l := range leads.Data {
		if l.Email == "grace@hopper.example" {
			found = true
			if l.Title != "Rear Admiral (upper half)" {
				t.Fatalf("title = %q, want the corrected value — an editable source that reports 'unchanged' loses the customer's fix", l.Title)
			}
		}
	}
	if !found {
		t.Fatal("the corrected lead is missing entirely")
	}
}

// A row nobody can identify is disclosed with its line, and the file it came in
// still lands the rows that ARE identifiable. Half a file imported under a
// success message is the failure this reports its way out of.
func TestCSVImportDisclosesUnidentifiableRows(t *testing.T) {
	e := setupImportApp(t)

	ragged := "Email,Full Name\n" +
		",No Address Here\n" +
		"real@example.test,Real Person\n"
	profile, status := uploadCSV(t, e, "lead", ragged)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, status := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Skipped != 1 || len(report.Issues) != 1 {
		t.Fatalf("report = %+v, want exactly one disclosed skip", report)
	}
	if report.Issues[0].Line != 2 {
		t.Fatalf("skip names line %d, want 2 — a human has to be able to open the file to it", report.Issues[0].Line)
	}
	if report.Disposition.Created != 1 {
		t.Fatalf("created = %d, want the one identifiable row still landing", report.Disposition.Created)
	}
}

// IEM-WIRE-5: approval is valid ONLY from awaiting_approval. Approving twice
// means the second approver is acting on a state nobody judged.
func TestCSVImportApprovalIsValidOnlyOnce(t *testing.T) {
	e := setupImportApp(t)

	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("first approve → %d, want 202", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusConflict {
		t.Fatalf("second approve → %d, want 409", status)
	}
}

// A run this installation does not have answers not-found — the same answer a
// run it may not read gets, which is what keeps existence undisclosed.
func TestCSVImportUnknownRunIsNotFound(t *testing.T) {
	e := setupImportApp(t)

	const absent = "00000000-0000-7000-8000-000000000000"
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+absent, nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("GET an absent run → %d, want 404", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+absent+"/report", nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("report of an absent run → %d, want 404", status)
	}
}

// A mapping that names a field the object does not have is refused at the door,
// not at row 40,000 of a commit.
func TestCSVImportRefusesAnImpossibleMapping(t *testing.T) {
	e := setupImportApp(t)

	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead",
		"source_ref": profile.SourceRef,
		"mapping":    map[string]string{"Email": "email", "Title": "annual_revenue"},
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown target → %d, want 422", status)
	}
}

// A source reference minted for another installation is not a door into this
// one. The blobstore treats keys as opaque bytes by design — the key IS the
// tenant boundary — so this refusal is the only thing between a caller and
// somebody else's uploaded estate.
func TestCSVImportRefusesAForeignSourceReference(t *testing.T) {
	e := setupImportApp(t)

	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	foreign := "11111111-1111-7111-8111-111111111111/import/" + path.Base(profile.SourceRef)

	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead",
		"source_ref": foreign, "mapping": profile.SuggestedMapping,
	}, nil, nil)
	// Not-found, not forbidden: a caller may not learn whether a reference they
	// were never given exists.
	if status != http.StatusNotFound {
		t.Fatalf("a foreign source_ref → %d, want 404", status)
	}
	if got := leadCount(t, e); got != 0 {
		t.Fatalf("leads = %d, want 0 — nothing may land from a foreign source", got)
	}
}

// The dry run says what the commit WILL do, so a second staging of a file
// already imported must predict nothing new — not "will update everything",
// which is what classifying by existence alone would report.
func TestCSVImportPredictsUnchangedRatherThanUpdatingEverything(t *testing.T) {
	e := setupImportApp(t)

	first, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", first.SourceRef, first.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	again, _ := uploadCSV(t, e, "lead", prospectCSV)
	second, _ := createRunWithMapping(t, e, "lead", again.SourceRef, again.SuggestedMapping)

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+second.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Unchanged != 3 || report.Disposition.Created != 0 || report.Disposition.Updated != 0 {
		t.Fatalf("prediction = %+v, want 3 unchanged and nothing else: the file has not changed", report.Disposition)
	}
}

// A run whose validation could not finish is recorded as failed. Left in
// `validating` it would be an orphan: approve refuses it, resume refuses it,
// and nothing else could ever move it.
func TestCSVImportRecordsAValidationThatCouldNotFinish(t *testing.T) {
	e, store := setupImportAppWithStore(t)

	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	if err := store.Delete(context.Background(), profile.SourceRef); err != nil {
		t.Fatalf("removing the stored upload: %v", err)
	}

	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead",
		"source_ref": profile.SourceRef, "mapping": profile.SuggestedMapping,
	}, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("a vanished upload → %d, want 404", status)
	}

	// The run row exists — it was created before the file was read — and it
	// must not be sitting in validating.
	var runs []importRunDTO
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT id::text, status FROM import_run WHERE connector = 'csv'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r importRunDTO
			if err := rows.Scan(&r.ID, &r.Status); err != nil {
				return err
			}
			runs = append(runs, r)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %+v, want the one this attempt opened", runs)
	}
	if runs[0].Status != "failed" {
		t.Fatalf("status = %q, want failed — a validating run nothing can move is an orphan", runs[0].Status)
	}
}

// Every run response carries who opened it. The database stamps it; a surface
// that drops it leaves the governance question "who imported this" unanswerable
// from the API the operator actually reads.
func TestCSVImportRunNamesWhoOpenedIt(t *testing.T) {
	e := setupImportApp(t)

	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	var run struct {
		ID         string `json:"id"`
		CapturedBy string `json:"captured_by"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead",
		"source_ref": profile.SourceRef, "mapping": profile.SuggestedMapping,
	}, nil, &run); status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	if run.CapturedBy == "" {
		t.Fatal("the run does not say who opened it")
	}
}

type organizationListDTO struct {
	Data []struct {
		DisplayName string `json:"display_name"`
		LegalName   string `json:"legal_name"`
		Industry    string `json:"industry"`
		Description string `json:"description"`
	} `json:"data"`
}

func organizations(t *testing.T, e *apptest.AppEnv) organizationListDTO {
	t.Helper()
	var orgs organizationListDTO
	if status := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); status != http.StatusOK {
		t.Fatalf("GET /v1/organizations → %d, want 200", status)
	}
	return orgs
}

// The second object an import can land, end to end — and the fields a create
// path is easy to forget: legal_name and description reach the stored record
// on the FIRST import, not only when a second upload happens to patch them.
func TestCSVImportLandsOrganizationsWithEveryMappedField(t *testing.T) {
	e := setupImportApp(t)

	const file = "Company,Legal Name,Industry,Description\n" +
		"Initech,Initech GmbH,software,They make software\n" +
		"Umbrella,Umbrella AG,biotech,They make other things\n"
	profile, status := uploadCSV(t, e, "organization", file)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	// "Company" matches no organization field by name, so the human maps it.
	mapping := map[string]string{
		"Company": "display_name", "Legal Name": "legal_name",
		"Industry": "industry", "Description": "description",
	}
	run, status := createRunWithMapping(t, e, "organization", profile.SourceRef, mapping)
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	before := len(organizations(t, e).Data)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	orgs := organizations(t, e)
	if len(orgs.Data) != before+2 {
		t.Fatalf("organizations = %d, want %d", len(orgs.Data), before+2)
	}
	var found bool
	for _, o := range orgs.Data {
		if o.DisplayName != "Initech" {
			continue
		}
		found = true
		if o.LegalName != "Initech GmbH" || o.Industry != "software" || o.Description != "They make software" {
			t.Fatalf("stored %+v — a mapped column that lands on neither create nor update is a column the import lied about", o)
		}
	}
	if !found {
		t.Fatal("the imported organization is missing")
	}

	// A corrected file rewrites the fields that changed, on the object whose
	// natural key is its own name.
	const corrected = "Company,Legal Name,Industry,Description\n" +
		"Initech,Initech SE,software,They make software\n" +
		"Umbrella,Umbrella AG,biotech,They make other things\n"
	edited, _ := uploadCSV(t, e, "organization", corrected)
	editedRun, status := createRunWithMapping(t, e, "organization", edited.SourceRef, mapping)
	if status != http.StatusAccepted {
		t.Fatalf("create corrected run → %d, want 202", status)
	}
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+editedRun.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d", status)
	}
	if report.Disposition.Updated != 1 || report.Disposition.Unchanged != 1 {
		t.Fatalf("prediction = %+v, want exactly the one row that changed", report.Disposition)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+editedRun.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve corrected → %d, want 202", status)
	}
	for _, o := range organizations(t, e).Data {
		if o.DisplayName == "Initech" && o.LegalName != "Initech SE" {
			t.Fatalf("legal name = %q, want the corrected value", o.LegalName)
		}
	}
	if got := len(organizations(t, e).Data); got != before+2 {
		t.Fatalf("organizations = %d, want %d — a correction updates, it does not duplicate", got, before+2)
	}
}

// The upload refuses what it cannot profile, and says which part of the request
// was wrong rather than failing somewhere later with a run already created.
func TestCSVImportUploadRefusesWhatItCannotUse(t *testing.T) {
	e := setupImportApp(t)

	if _, status := uploadCSV(t, e, "deal", prospectCSV); status != http.StatusUnprocessableEntity {
		t.Fatalf("an unsupported object → %d, want 422", status)
	}
	if _, status := uploadCSV(t, e, "lead", ""); status != http.StatusUnprocessableEntity {
		t.Fatalf("an empty file → %d, want 422", status)
	}
	if _, status := uploadCSV(t, e, "lead", "Email,Email\na@x.test,b@x.test\n"); status != http.StatusUnprocessableEntity {
		t.Fatalf("a duplicate header → %d, want 422", status)
	}
}

// What a CSV row does when a company of that name is ALREADY in the CRM, and
// what an out-of-enum size_band does. Both were reported as import defects on
// 2026-08-23 after a run through a chat host; this test is what settles each
// against the running stack rather than against a reading of the code.
func TestCSVImportMeetsAnExistingCompanyAndABadSizeBand(t *testing.T) {
	e := setupImportApp(t)

	// The company arrives the way real ones do — created directly, NOT by a
	// previous CSV run. That distinction is the whole test: the importer's
	// identity map only remembers rows IT wrote, so a company captured from
	// mail, a connector or a seed is invisible to it.
	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Akeneo", "industry": "retail"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}

	// A spreadsheet naming that same company.
	const second = "Company,Industry\nAkeneo,retail software\n"
	profile2, _ := uploadCSV(t, e, "organization", second)
	run2, _ := createRunWithMapping(t, e, "organization", profile2.SourceRef,
		map[string]string{"Company": "display_name", "Industry": "industry"})

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run2.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	// The preview must SAY the company is already there. It does not refuse
	// the row — the commit creates it and files a review pair, the same as a
	// manual create — but "create 1" with nothing else said is what let a
	// chat host report "no duplicates" to a user on 2026-08-23.
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d row(s) read; the contract's invariant is that the four sum to rows_read",
			got, report.RowsRead)
	}
	if len(report.Issues) == 0 {
		t.Errorf("the preview reported %d create(s) and no issue for a row naming a company "+
			"the CRM already holds; a human approving this is told nothing about the duplicate",
			report.Disposition.Created)
	}
	// The number a person decides on: "1 company, 1 already here".
	if report.Disposition.Duplicates == nil || *report.Disposition.Duplicates != 1 {
		t.Errorf("duplicates = %v, want 1 — this is the count the human is shown before approving",
			report.Disposition.Duplicates)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run2.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve 2 → %d, want 202", status)
	}

	named := 0
	for _, o := range organizations(t, e).Data {
		if o.DisplayName == "Akeneo" {
			named++
		}
	}
	// The duplicate IS created — that is the existing create-path policy, and
	// changing it is a separate decision — but it must not be silent.
	var pairs int
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'organization'`).Scan(&pairs)
	}); err != nil {
		t.Fatalf("counting dedupe pairs: %v", err)
	}
	if named != 2 || pairs != 1 {
		t.Errorf("after importing a name the CRM already held: %d companies named Akeneo and %d review pair(s); "+
			"want 2 and 1 — the row lands and the pair is filed for a human", named, pairs)
	}

	// size_band is a closed enum with a database CHECK constraint. A headcount
	// column mapped onto it cannot be written, so the PREVIEW is the place to
	// say so — a dry run that reports a clean create for a row the commit
	// cannot land has told the user the opposite of what will happen.
	const bands = "Company,Employees\nNordwind Logistik,240\n"
	profile3, _ := uploadCSV(t, e, "organization", bands)
	run3, status3 := createRunWithMapping(t, e, "organization", profile3.SourceRef,
		map[string]string{"Company": "display_name", "Employees": "size_band"})

	if status3 != http.StatusAccepted {
		t.Fatalf("create run 3 → %d, want 202", status3)
	}
	var r3 importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run3.ID+"/report", nil, nil, &r3); status != http.StatusOK {
		t.Fatalf("report 3 → %d, want 200", status)
	}
	if r3.Disposition.Created != 0 || r3.Disposition.Skipped != 1 || len(r3.Issues) != 1 {
		t.Fatalf("a headcount mapped onto size_band previewed as created=%d skipped=%d issues=%d; "+
			"want 0/1/1 — the CHECK constraint would refuse it, so the dry run must say so",
			r3.Disposition.Created, r3.Disposition.Skipped, len(r3.Issues))
	}

	// And the commit must agree with the preview it showed.
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run3.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve 3 → %d, want 202", status)
	}
	for _, o := range organizations(t, e).Data {
		if o.DisplayName == "Nordwind Logistik" {
			t.Error("the commit landed a row the preview disclosed as skipped")
		}
	}
}

// Importing ONE address column must not erase the rest of a company's address.
// The patch builder writes all six address columns whenever an Address is
// given, so a file carrying only a City would otherwise blank the street,
// postal code and country a human had entered.
//
// The re-import path is what exercises this: the row must MATCH the incumbent
// rather than mint a twin, so the file is imported twice — the first landing
// the company under this importer's own identity, the second carrying only a
// City.
func TestCSVImportOfOneAddressColumnKeepsTheRestOfTheAddress(t *testing.T) {
	e := setupImportApp(t)

	// Land the company through the import, with a full address.
	const full = "Company,Street,City,Postal,Country\n" +
		"Baqend,Stresemannstr. 23,Hamburg,22769,DE\n"
	p1, _ := uploadCSV(t, e, "organization", full)
	r1, _ := createRunWithMapping(t, e, "organization", p1.SourceRef, map[string]string{
		"Company": "display_name", "Street": "address.line1", "City": "address.city",
		"Postal": "address.postal_code", "Country": "address.country",
	})
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+r1.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve 1 → %d, want 202", status)
	}

	// A corrected file carrying the company and ONE address field.
	const partial = "Company,City\nBaqend,Berlin\n"
	profile, _ := uploadCSV(t, e, "organization", partial)
	run, _ := createRunWithMapping(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "City": "address.city"})
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	var orgs struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Address     struct {
				Line1      string `json:"line1"`
				City       string `json:"city"`
				PostalCode string `json:"postal_code"`
				Country    string `json:"country"`
			} `json:"address"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); status != http.StatusOK {
		t.Fatalf("listing → %d, want 200", status)
	}
	for _, o := range orgs.Data {
		if o.DisplayName != "Baqend" {
			continue
		}
		if o.Address.City != "Berlin" {
			t.Errorf("the corrected file did not reach the record: city=%q, want Berlin", o.Address.City)
		}
		if o.Address.Line1 == "" || o.Address.PostalCode == "" || o.Address.Country == "" {
			t.Errorf("importing a City column erased the rest of the address: line1=%q postal=%q country=%q",
				o.Address.Line1, o.Address.PostalCode, o.Address.Country)
		}
	}
}

// on_duplicate: skip — the preview and the commit must give the SAME answer.
// A preview that says "create" while the approved run skips is the defect this
// whole change set exists to remove, in a new place.
func TestCSVImportSkipDuplicatesPreviewsWhatItWillDo(t *testing.T) {
	e := setupImportApp(t)

	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Kestrel Data"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}
	orgsBefore := len(organizations(t, e).Data)

	const file = "Company\nKestrel Data\nNordwind Logistik\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunOnDuplicate(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name"}, "skip")
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 1 || report.Disposition.Skipped != 1 {
		t.Fatalf("preview = created %d skipped %d, want 1 and 1 — the duplicate is skipped, the new company lands",
			report.Disposition.Created, report.Disposition.Skipped)
	}
	if report.Disposition.Duplicates == nil || *report.Disposition.Duplicates != 1 {
		t.Errorf("duplicates = %v, want 1", report.Disposition.Duplicates)
	}
	if got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped; got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read", got, report.RowsRead)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	// Exactly one company added, and no second Kestrel.
	after := organizations(t, e).Data
	if len(after) != orgsBefore+1 {
		t.Errorf("companies went from %d to %d; the preview promised one new company", orgsBefore, len(after))
	}
	kestrels := 0
	for _, o := range after {
		if o.DisplayName == "Kestrel Data" {
			kestrels++
		}
	}
	if kestrels != 1 {
		t.Errorf("%d companies named Kestrel Data; the run was asked to skip duplicates", kestrels)
	}
}

// An unknown policy is refused rather than silently treated as create.
func TestCSVImportRefusesAnUnknownDuplicatePolicy(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "organization", "Company\nInitech\n")
	// Decoded into nothing: a refusal answers a problem document, not a run.
	status := e.Call(t, http.MethodPost, "/v1/imports", map[string]any{
		"connector": "csv", "object": "organization", "source_ref": profile.SourceRef,
		"mapping": map[string]string{"Company": "display_name"}, "on_duplicate": "merge",
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown on_duplicate → %d, want 422", status)
	}
}

// TestCSVImportCountsTheDuplicatesTheCommitMetNotTheOnesThePreviewPredicted is
// the case that separates a report from a prediction.
//
// The estate moves between the preview and the approval — a colleague creates
// one of the companies, an earlier run lands it — and a finished report is
// supposed to describe what happened. Counting only what the dry run saw makes
// "0 duplicates" a statement about a database that no longer exists, and the
// person reading it has already approved on the strength of it.
//
// The four load-bearing counts are checked either side, because the fix must
// not disturb them: a duplicate that lands is counted in Created and nowhere
// else, and `duplicates` sits outside the sum by design.
func TestCSVImportCountsTheDuplicatesTheCommitMetNotTheOnesThePreviewPredicted(t *testing.T) {
	e := setupImportApp(t)

	const file = "Company,Industry\nZephyr Freight,logistics\n"
	profile, _ := uploadCSV(t, e, "organization", file)
	run, status := createRunWithMapping(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "Industry": "industry"})
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}

	var preview importReportDTO
	if code := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &preview); code != http.StatusOK {
		t.Fatalf("preview report → %d, want 200", code)
	}
	if preview.Disposition.Duplicates == nil || *preview.Disposition.Duplicates != 0 {
		t.Fatalf("the preview found %v duplicate(s) against an estate that holds none; "+
			"this test needs a clean prediction to have something to disagree with later",
			duplicatesSaid(preview))
	}

	// A colleague creates the company, after the preview and before the
	// approval. Directly, the way real ones arrive: the importer's identity map
	// remembers only rows IT wrote, so this one is invisible to it.
	if code := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Zephyr Freight", "industry": "logistics"}, nil, nil); code != http.StatusCreated {
		t.Fatalf("creating the company between preview and approval → %d, want 201", code)
	}

	if code := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); code != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", code)
	}

	var final importReportDTO
	if code := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &final); code != http.StatusOK {
		t.Fatalf("final report → %d, want 200", code)
	}
	if final.Disposition.Duplicates == nil || *final.Disposition.Duplicates != 1 {
		t.Errorf("the finished run reports %v duplicate(s); the commit met one, and a report that "+
			"repeats the prediction describes an estate that no longer exists",
			duplicatesSaid(final))
	}
	if got := final.Disposition.Created + final.Disposition.Updated +
		final.Disposition.Unchanged + final.Disposition.Skipped; got != final.RowsRead {
		t.Errorf("the finished disposition sums to %d for %d row(s) read; a duplicate is counted in "+
			"Created and nowhere else, so the four must still sum to rows_read", got, final.RowsRead)
	}
	if final.Disposition.Created != 1 {
		t.Errorf("created = %d, want 1 — the duplicate lands and the review queue picks up the pair",
			final.Disposition.Created)
	}
}

// duplicatesSaid renders the duplicate count for a failure message. The field
// is a pointer because "not reported" and "none found" are different answers,
// and %v on it prints an address — which tells a reader of a failed run
// nothing at all about what the report said.
func duplicatesSaid(report importReportDTO) string {
	if report.Disposition.Duplicates == nil {
		return "no"
	}
	return strconv.Itoa(*report.Disposition.Duplicates)
}
