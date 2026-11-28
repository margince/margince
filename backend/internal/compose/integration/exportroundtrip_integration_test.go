// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Export CSV writes the file Data import reads back. For each record type the
// export names, the test seeds records through the importer and exports them.
// It then uploads the file as that type. Every row must come back "same", and
// every column must map without the reader's help.

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// exportAll posts the export of every record of one type and answers the file.
func exportAll(t *testing.T, e *apptest.AppEnv, object string) (body []byte, disposition string) {
	t.Helper()
	filter := AnyMap{"field": "created_at", "op": "gte", "value": "2000-01-01"}
	if object == "project" {
		// The project list is not filterable by its creation day.
		filter = AnyMap{"field": "phase", "op": "neq", "value": "closed"}
	}
	raw, err := json.Marshal(AnyMap{"object": object, "filter": filter, "format": "csv"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, e.TS.URL+"/v1/exports", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	//nolint:bodyclose // apptest.CloseBody closes it in the deferred call below.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("export request: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the export: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export of %s → %d: %s", object, resp.StatusCode, body)
	}
	return body, resp.Header.Get("Content-Disposition")
}

// roundTrip uploads an exported file as its type and answers the test run's
// report together with the header and the columns the importer mapped itself.
func roundTrip(t *testing.T, e *apptest.AppEnv, object string, file []byte) (importReportDTO, []string, importjobtest.Profile) {
	t.Helper()
	profile, status := importjobtest.UploadCSV(t, e, object, string(file))
	if status != http.StatusOK {
		t.Fatalf("uploading the exported %s file → %d", object, status)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(file, []byte("\xEF\xBB\xBF")))).ReadAll()
	if err != nil || len(records) == 0 {
		t.Fatalf("the exported %s file is not CSV: %v", object, err)
	}
	run, status := importjobtest.CreateRunWithMapping(t, e, object, profile.SourceRef, profile.SuggestedMapping)
	if status != http.StatusAccepted {
		t.Fatalf("test run of the exported %s file → %d", object, status)
	}
	return readImportReport(t, e, run.ID), records[0], profile
}

// wantEverythingSame fails unless the report calls every one of rows rows
// unchanged, and every header was mapped by the importer's own suggestion.
func wantEverythingSame(t *testing.T, object string, report importReportDTO, header []string, profile importjobtest.Profile, rows int) {
	t.Helper()
	d := report.Disposition
	if d.Unchanged != rows || d.Created != 0 || d.Updated != 0 || d.Skipped != 0 {
		t.Errorf("round trip = created %d updated %d same %d skipped %d, want %d same; issues %+v",
			d.Created, d.Updated, d.Unchanged, d.Skipped, rows, report.Issues)
	}
	for _, column := range header {
		if _, mapped := profile.SuggestedMapping[column]; !mapped {
			t.Errorf("the importer does not map the exported column %q on its own", column)
		}
	}
	if header[len(header)-1] != "id" {
		t.Errorf("the id is not the last column: %v", header)
	}
	// The other direction: no field the import takes goes missing from the file.
	// The exceptions are the author, which the import writes on create only, and a
	// deal's stakeholder edge. A project's deal and tags, a contact's employer ref
	// and the custom fields are exceptions too.
	notStored := map[string]bool{"author": true, "ref": true, "contact_ref": true, "deal_ref": true}
	for _, target := range profile.Targets {
		if notStored[target] || strings.HasPrefix(target, "cf_") || (object == "contact" && target == "company_ref") || (object == "project" && target == "tags") {
			continue
		}
		if !slices.Contains(header, target) {
			t.Errorf("the import takes %q and the exported file has no such column: %v", target, header)
		}
	}
	for _, column := range header {
		if !slices.Contains(profile.Targets, column) {
			t.Errorf("the exported column %q is not an import field; the import takes %v", column, profile.Targets)
		}
	}
}

// seedBundle imports one ZIP of rows, so the records exist the way an import makes them.
func seedBundle(t *testing.T, e *apptest.AppEnv, files map[string]string, parts []AnyMap) {
	t.Helper()
	run := stageBundle(t, e, uploadZip(t, e, "seed.zip", zipBytes(t, files)).SourceRef, "seed", parts)
	approveAndRead(t, e, run.ID)
	if report := readImportReport(t, e, run.ID); report.Disposition.Skipped != 0 {
		t.Fatalf("seeding skipped rows: %+v", report.Issues)
	}
}

// identityPart maps a file's columns onto the targets of the same names.
func identityPart(file, object, header string) AnyMap {
	mapping := AnyMap{}
	for column := range strings.SplitSeq(header, ",") {
		mapping[column] = column
	}
	return AnyMap{"file": file, "object": object, "mapping": mapping}
}

func TestAnExportedCompanyFileUploadsAsSame(t *testing.T) {
	e := setupImportApp(t)
	header := "ref,display_name,legal_name,industry,size_band,description,domain,linkedin_url,phone,email,owner_email,tags,address.line1,address.city,address.postal_code,address.country"
	seedBundle(t, e, map[string]string{"companies.csv": header + "\n" +
		"K1,Müller & Söhne,Müller GmbH,software,11-50,\"A \"\"quoted\"\" note; two parts\",mueller.example,https://www.linkedin.com/company/mueller,+49 30 1234567,hallo@mueller.example,ada@example.com,vip; nord,Hauptstr. 1,Köln,50667,DE\n" +
		"K2,Plain Co,,,,,,,,,,,,,,\n"},
		[]AnyMap{identityPart("companies.csv", "company", header)})

	file, disposition := exportAll(t, e, "company")
	if !bytes.HasPrefix(file, []byte("\xEF\xBB\xBF")) {
		t.Errorf("the file has no byte order mark, so Excel reads Müller as mojibake")
	}
	if !strings.Contains(disposition, "company-") || !strings.Contains(disposition, ".csv") {
		t.Errorf("Content-Disposition = %q, want a file named for the type and the day", disposition)
	}
	report, header2, profile := roundTrip(t, e, "company", file)
	wantEverythingSame(t, "company", report, header2, profile, 2)
}

func TestAnExportedContactFileUploadsAsSame(t *testing.T) {
	e := setupImportApp(t)
	header := "ref,full_name,first_name,last_name,email,phone,linkedin_url,title,owner_email,tags,address.line1,address.city,address.postal_code,address.country,company_name"
	seedBundle(t, e, map[string]string{"contacts.csv": header + "\n" +
		"C1,Anna Müller,Anna,Müller,anna@mueller.example,+49 30 1234567,https://www.linkedin.com/in/anna-mueller,CTO,ada@example.com,vip; nord,Hauptstr. 1,Köln,50667,DE,Müller & Söhne\n" +
		"C2,No Mail,,,,,,,,,,,,,\n" +
		"C3,Formula Fan,,,fan@x.example,,,=1+1,,,,,,,\n"},
		[]AnyMap{identityPart("contacts.csv", "contact", header)})

	file, _ := exportAll(t, e, "contact")
	if !strings.Contains(string(file), "'=1+1") {
		t.Errorf("a title starting with = is not quoted against a spreadsheet formula:\n%s", file)
	}
	report, header2, profile := roundTrip(t, e, "contact", file)
	wantEverythingSame(t, "contact", report, header2, profile, 3)
}

func TestAnExportedLeadFileUploadsAsSame(t *testing.T) {
	e := setupImportApp(t)
	header := "ref,full_name,email,phone,title,company_name,linkedin_url,status,origin,origin_detail,owner_email,tags"
	seedBundle(t, e, map[string]string{"leads.csv": header + "\n" +
		"L1,Lena Lead,lena@lead.example,+49 30 7654321,Head of IT,Lead AG,https://www.linkedin.com/in/lena-lead,new,Messe,Stand B4,ada@example.com,messe; 2026\n" +
		"L2,Phone Only,,+49 30 5550000,,,,,,,,\n"},
		[]AnyMap{identityPart("leads.csv", "lead", header)})

	file, _ := exportAll(t, e, "lead")
	report, header2, profile := roundTrip(t, e, "lead", file)
	wantEverythingSame(t, "lead", report, header2, profile, 2)
}

func TestAnExportedDealFileUploadsAsSame(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, won := seededStageNames(t, e)
	companies := "ref,display_name"
	deals := "ref,name,pipeline,stage,amount,currency,close_date,company_ref,owner_email,tags,description"
	seedBundle(t, e, map[string]string{
		"companies.csv": companies + "\nK1,Müller & Söhne\n",
		"deals.csv": deals + "\n" +
			"D1,Rollout 2027," + pipeline + "," + open + ",12000.50,EUR,2099-01-31,K1,ada@example.com,vip; nord,\"Two years; \"\"quoted\"\"\"\n" +
			"D2,Won once," + pipeline + "," + won + ",500,EUR,2025-06-30,,,,\n",
	}, []AnyMap{
		identityPart("companies.csv", "company", companies),
		identityPart("deals.csv", "deal", deals),
	})

	file, _ := exportAll(t, e, "deal")
	text := string(file)
	if !strings.Contains(text, "12000.50") || strings.Contains(text, "1200050") {
		t.Errorf("the amount is not in major units:\n%s", text)
	}
	if !strings.Contains(text, "Müller & Söhne") || !strings.Contains(text, pipeline) || !strings.Contains(text, open) {
		t.Errorf("the company, pipeline and stage are not written by name:\n%s", text)
	}
	report, header2, profile := roundTrip(t, e, "deal", file)
	wantEverythingSame(t, "deal", report, header2, profile, 2)
}

// The file is named after the view it exports and the day, not after the table.
func TestAnExportOfASavedViewIsNamedAfterTheViewAndTheDay(t *testing.T) {
	e := setupImportApp(t)
	var view struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/views", AnyMap{
		"resource": "contacts", "name": "VIP CTOs (Nord)",
		"query": AnyMap{"filter": AnyMap{"field": "created_at", "op": "gte", "value": "2000-01-01"}},
	}, nil, &view); status != http.StatusCreated {
		t.Fatalf("create saved view → %d, want 201", status)
	}
	raw, err := json.Marshal(AnyMap{"view_id": view.ID, "format": "csv"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, e.TS.URL+"/v1/exports", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	//nolint:bodyclose // apptest.CloseBody closes it in the deferred call below.
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("export request: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export of the view → %d, want 200", resp.StatusCode)
	}
	name := resp.Header.Get("Content-Disposition")
	if !strings.Contains(name, "vip-ctos-nord-") || !strings.Contains(name, ".csv") {
		t.Errorf("Content-Disposition = %q, want the view's name and the day", name)
	}
}

// A row that names a contact, lead or deal by the id the export wrote updates
// that record. An id nobody can see is refused, not created.
func TestARowNamingAnIDUpdatesThatContactLeadOrDeal(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _ := seededStageNames(t, e)
	seedBundle(t, e, map[string]string{
		"contacts.csv": "ref,full_name,title\nC1,No Mail,Intern\n",
		"leads.csv":    "ref,full_name,email,title\nL1,Lena Lead,lena@lead.example,Intern\n",
		"deals.csv":    "ref,name,pipeline,stage\nD1,Rollout," + pipeline + "," + open + "\n",
	}, []AnyMap{
		identityPart("contacts.csv", "contact", "ref,full_name,title"),
		identityPart("leads.csv", "lead", "ref,full_name,email,title"),
		identityPart("deals.csv", "deal", "ref,name,pipeline,stage"),
	})
	for _, tc := range []struct{ object, path, field, value, jsonField string }{
		{"contact", "/v1/contacts/", "title", "Chief", "title"},
		{"lead", "/v1/leads/", "title", "Chief", "title"},
		{"deal", "/v1/deals/", "name", "Rollout 2027", "name"},
	} {
		t.Run(tc.object, func(t *testing.T) {
			file, _ := exportAll(t, e, tc.object)
			id := CSVColumn(t, file, "id")[0]
			missing := "0192f3a0-0000-7000-8000-00000000dead"
			body := "id," + tc.field + "\n" + id + "," + tc.value + "\n" + missing + ",Ghost\nnot-an-id,Ghost\n"
			profile, status := importjobtest.UploadCSV(t, e, tc.object, body)
			if status != http.StatusOK {
				t.Fatalf("upload → %d", status)
			}
			run, status := importjobtest.CreateRunWithMapping(t, e, tc.object, profile.SourceRef,
				map[string]string{"id": "id", tc.field: tc.field})
			if status != http.StatusAccepted {
				t.Fatalf("test run → %d", status)
			}
			report := readImportReport(t, e, run.ID)
			if d := report.Disposition; d.Updated != 1 || d.Created != 0 || d.Skipped != 2 {
				t.Fatalf("report = %+v issues %+v, want the named record updated and the other two refused", d, report.Issues)
			}
			codes := []string{}
			for _, issue := range report.Issues {
				codes = append(codes, issue.ReasonCode)
			}
			if !slices.Contains(codes, "id_record_not_visible") || !slices.Contains(codes, "id_not_a_record_id") {
				t.Errorf("issue codes = %v, want id_record_not_visible and id_not_a_record_id", codes)
			}
			approveAndRead(t, e, run.ID)
			var record map[string]any
			if status := e.Call(t, http.MethodGet, tc.path+id, nil, nil, &record); status != http.StatusOK || record[tc.jsonField] != tc.value {
				t.Errorf("GET %s%s → %d, %s = %v, want %q", tc.path, id, status, tc.jsonField, record[tc.jsonField], tc.value)
			}
		})
	}
}

// The rows follow the sort the view stores, as the list screen does, not the id order.
func TestAnExportOfASortedViewFollowsTheViewsSort(t *testing.T) {
	e := setupImportApp(t)
	seedBundle(t, e, map[string]string{"contacts.csv": "ref,full_name,email\nC1,Charlie Zed,c@x.example\nC2,Alice Able,a@x.example\nC3,Bob Baker,b@x.example\n"},
		[]AnyMap{identityPart("contacts.csv", "contact", "ref,full_name,email")})
	for sort, want := range map[string][]string{
		"full_name":  {"Alice Able", "Bob Baker", "Charlie Zed"},
		"-full_name": {"Charlie Zed", "Bob Baker", "Alice Able"},
	} {
		var view struct {
			ID string `json:"id"`
		}
		if status := e.Call(t, http.MethodPost, "/v1/views", AnyMap{
			"resource": "contacts", "name": "By name " + sort,
			"query": AnyMap{
				"filter": AnyMap{"field": "created_at", "op": "gte", "value": "2000-01-01"},
				"list":   AnyMap{"q": "", "sort": sort, "includeArchived": false, "filters": AnyMap{}},
			},
		}, nil, &view); status != http.StatusCreated {
			t.Fatalf("create saved view → %d, want 201", status)
		}
		raw, err := json.Marshal(AnyMap{"view_id": view.ID, "format": "csv"})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, e.TS.URL+"/v1/exports", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		//nolint:bodyclose // apptest.CloseBody closes it right below.
		resp, err := e.Client.Do(req)
		if err != nil {
			t.Fatalf("export request: %v", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		apptest.CloseBody(t, resp)
		if readErr != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("export → %d %v", resp.StatusCode, readErr)
		}
		if got := CSVColumn(t, body, "full_name"); !slices.Equal(got, want) {
			t.Errorf("sort %q: names = %v, want %v", sort, got, want)
		}
	}
}
