// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A file may name one company or contact twice under two spellings. The import
// lands its rows one at a time and meets the earlier spelling as a stored
// record. So the test run counts that duplicate before anyone approves.

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

func TestTheTestRunCountsLookAlikesInsideTheFile(t *testing.T) {
	companyMap := map[string]string{"Company": "display_name", "Street": "address.line1", "City": "address.city"}
	contactMap := map[string]string{"Name": "full_name", "Email": "email", "Employer": "company_name"}
	renameMap := map[string]string{"Id": "id", "Company": "display_name"}
	for _, tc := range []struct {
		name, object, onDuplicate, file string
		mapping                         map[string]string
		// stored is a record the CRM holds before the file arrives, written
		// through its create route. The file names it as {stored}.
		stored AnyMap
		// earlier is a file the same importer lands first.
		earlier string
		want    []string
	}{{
		name: "one brand twice, skipped", object: "company", onDuplicate: "skip", mapping: companyMap,
		file: "Company,Street,City\nSkipco Trading,,\nSkipco Trading GmbH,,\nNordwind Logistik,,\n",
		want: []string{"created=2 updated=0 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "one brand twice, kept", object: "company", onDuplicate: "create", mapping: companyMap,
		file: "Company,Street,City\nSkipco Trading,,\nSkipco Trading GmbH,,\nNordwind Logistik,,\n",
		want: []string{"created=3 updated=0 unchanged=0 skipped=0 duplicates=1", "line 3: company_duplicate_found"},
	}, {
		name: "a stored look-alike's twin at one street, kept", object: "company", onDuplicate: "create", mapping: companyMap,
		stored: storedCompany("Kestrel Data", ""),
		file:   "Company,Street,City\nKestrel Data GmbH,Marienplatz 8,München\nKestrel Datta,Marienplatz 8,München\n",
		want:   []string{"created=2 updated=0 unchanged=0 skipped=0 duplicates=2", "line 2: company_duplicate_found", "line 3: company_duplicate_found"},
	}, {
		name: "a stored look-alike's twin at one street, skipped", object: "company", onDuplicate: "skip", mapping: companyMap,
		stored: storedCompany("Kestrel Data", ""),
		file:   "Company,Street,City\nKestrel Data GmbH,Marienplatz 8,München\nKestrel Datta,Marienplatz 8,München\n",
		want:   []string{"created=1 updated=0 unchanged=0 skipped=1 duplicates=1", "line 2: company_duplicate_skipped"},
	}, {
		name: "one contact twice at one employer, skipped", object: "contact", onDuplicate: "skip", mapping: contactMap,
		file: "Name,Email,Employer\nAnna Schmidt,anna.schmidt@twinco.example,\nAnna Schmitt,a.schmitt@twinco.example,\nBo Lind,bo@lindco.example,\n",
		want: []string{"created=2 updated=0 unchanged=0 skipped=1 duplicates=1", "line 3: contact_duplicate_skipped"},
	}, {
		name: "one contact twice, the first at a stored employer's domain", object: "contact", onDuplicate: "skip", mapping: contactMap,
		stored: storedCompany("Twinco", "twinco.example"),
		file:   "Name,Email,Employer\nAnna Schmidt,anna.schmidt@gmail.com,Twinco\nAnna Schmitt,a.schmitt@twinco.example,\n",
		want:   []string{"created=1 updated=0 unchanged=0 skipped=1 duplicates=1", "line 3: contact_duplicate_skipped"},
	}, {
		name: "one contact twice, the first at an employer an earlier row creates", object: "contact", onDuplicate: "skip", mapping: contactMap,
		file: "Name,Email,Employer\nAnna Lind,anna@twinco.example,Twinco\nBob Schmidt,bob.schmidt@gmail.com,Twinco\nBob Schmitt,b.schmitt@twinco.example,\n",
		want: []string{"created=2 updated=0 unchanged=0 skipped=1 duplicates=1", "line 2: employer_will_create", "line 4: contact_duplicate_skipped"},
	}, {
		name: "a twin of a stored company's new name", object: "company", onDuplicate: "skip", mapping: renameMap,
		stored: storedCompany("Kestrel Data", ""),
		file:   "Id,Company\n{stored},Falcon Works\n,Falcon Works GmbH\n",
		want:   []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "a twin of a stored company's old name", object: "company", onDuplicate: "skip", mapping: renameMap,
		stored: storedCompany("Kestrel Data", ""),
		file:   "Id,Company\n{stored},Falcon Works\n,Kestrel Data GmbH\n",
		want:   []string{"created=1 updated=1 unchanged=0 skipped=0 duplicates=0"},
	}, {
		name: "a twin of a stored contact's new name", object: "contact", onDuplicate: "skip",
		mapping: map[string]string{"Ref": "ref", "Name": "full_name", "Email": "email"},
		earlier: "Ref,Name,Email\nc1,Anna Schmidt,anna@gmail.com\n",
		file:    "Ref,Name,Email\nc1,Bea Brandt,bea@brandtco.example\nc2,Bea Brand,b.brand@brandtco.example\n",
		want:    []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: contact_duplicate_skipped"},
	}, {
		name: "a twin of a stored contact's old name", object: "contact", onDuplicate: "skip",
		mapping: map[string]string{"Ref": "ref", "Name": "full_name", "Email": "email"},
		earlier: "Ref,Name,Email\nc1,Anna Schmidt,anna.schmidt@twinco.example\n",
		file:    "Ref,Name,Email\nc1,Bea Brandt,bea@brandtco.example\nc2,Anna Schmitt,a.schmitt@twinco.example\n",
		want:    []string{"created=1 updated=1 unchanged=0 skipped=0 duplicates=0"},
	}, {
		name: "a twin of a renamed stored company at its street", object: "company", onDuplicate: "skip",
		mapping: map[string]string{"Id": "id", "Company": "display_name", "Street": "address.line1", "City": "address.city"},
		stored:  AnyMap{"source": "manual", "display_name": "Kestrel Data", "address": AnyMap{"line1": "Marienplatz 8", "city": "München"}},
		file:    "Id,Company,Street,City\n{stored},Falcon Works,,\n,Falcon Workz,Marienplatz 8,München\n",
		want:    []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "a twin of a renamed contact at its employer's domain", object: "contact", onDuplicate: "skip",
		mapping: map[string]string{"Ref": "ref", "Name": "full_name", "Email": "email", "Employer": "company_name"},
		stored:  storedCompany("Twinco", "twinco.example"),
		earlier: "Ref,Name,Email,Employer\nc1,Anna Schmidt,anna@gmail.com,Twinco\n",
		file:    "Ref,Name,Email,Employer\nc1,Bea Brandt,bea@gmail.com,\nc2,Bea Brand,b.brand@twinco.example,\n",
		want:    []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: contact_duplicate_skipped"},
	}, {
		name: "a twin of a renamed contact at the address it keeps", object: "contact", onDuplicate: "skip",
		mapping: map[string]string{"Ref": "ref", "Name": "full_name", "Email": "email"},
		earlier: "Ref,Name,Email\nc1,Anna Schmidt,anna@twinco.example\n",
		file:    "Ref,Name,Email\nc1,Bea Brandt,bea@gmail.com\nc2,Bea Brand,b.brand@twinco.example\n",
		want:    []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: contact_duplicate_skipped"},
	}, {
		name: "a name spelling a stored company's new domain", object: "company", onDuplicate: "skip",
		mapping: map[string]string{"Id": "id", "Company": "display_name", "Domain": "domain"},
		stored:  storedCompany("Kestrel Data", ""),
		file:    "Id,Company,Domain\n{stored},Kestrel Data,falcon.example\n,falcon.example,\n",
		want:    []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "a name spelling a renamed company's stored domain", object: "company", onDuplicate: "skip", mapping: renameMap,
		stored: storedCompany("Kestrel Data", "kestrel.example"),
		file:   "Id,Company\n{stored},Falcon Works\n,kestrel.example\n",
		want:   []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "a twin of a renamed company's registered name", object: "company", onDuplicate: "skip", mapping: renameMap,
		stored: AnyMap{"source": "manual", "display_name": "Kestrel Data", "legal_name": "Kestrel Data GmbH"},
		file:   "Id,Company\n{stored},Falcon Works\n,Kestrel Data AG\n",
		want:   []string{"created=0 updated=1 unchanged=0 skipped=1 duplicates=1", "line 3: company_duplicate_skipped"},
	}, {
		name: "a twin of an updated contact that names an employer", object: "contact", onDuplicate: "skip",
		mapping: map[string]string{"Ref": "ref", "Name": "full_name", "Email": "email", "Employer": "company_name"},
		stored:  storedCompany("Twinco", "twinco.example"),
		earlier: "Ref,Name,Email,Employer\nc1,Anna Schmidt,anna@gmail.com,\n",
		file:    "Ref,Name,Email,Employer\nc1,Bea Brandt,bea@gmail.com,Twinco\nc2,Bea Brand,b.brand@twinco.example,\n",
		want:    []string{"created=1 updated=1 unchanged=0 skipped=0 duplicates=0"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupImportApp(t)
			file := tc.file
			if tc.stored != nil {
				path := "/v1/companies"
				if tc.stored["full_name"] != nil {
					path = "/v1/contacts"
				}
				file = strings.ReplaceAll(file, "{stored}", createdID(t, e, path, tc.stored))
			}
			if tc.earlier != "" {
				profile, _ := importjobtest.UploadCSV(t, e, tc.object, tc.earlier)
				earlier, _ := createRunOnDuplicate(t, e, tc.object, profile.SourceRef, tc.mapping, tc.onDuplicate)
				approveAndRead(t, e, earlier.ID)
			}
			profile, _ := importjobtest.UploadCSV(t, e, tc.object, file)
			run, status := createRunOnDuplicate(t, e, tc.object, profile.SourceRef, tc.mapping, tc.onDuplicate)
			if status != http.StatusAccepted {
				t.Fatalf("create run → %d, want 202", status)
			}
			predicted := duplicateVerdicts(readImportReport(t, e, run.ID))
			if !slices.Equal(predicted, tc.want) {
				t.Fatalf("test run = %v, want %v", predicted, tc.want)
			}
			approveAndRead(t, e, run.ID)
			if committed := duplicateVerdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
				t.Fatalf("import = %v, test run said %v", committed, predicted)
			}
		})
	}
}

// A contact employed at a company of the same upload is met with that
// company's domains, however the row names it.
func TestTheTestRunMeetsAContactAtItsEmployerOfTheSameUpload(t *testing.T) {
	companies := AnyMap{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name", "domain": "domain"}}
	for name, employer := range map[string]string{"company_ref": "k1", "company_name": "Twinco"} {
		t.Run(name, func(t *testing.T) {
			e := setupImportApp(t)
			files := map[string]string{
				"companies.csv": "ref,name,domain\nk1,Twinco,twinco.example\n",
				"contacts.csv": "ref,full_name,email,employer\nc1,Anna Schmidt,anna.schmidt@gmail.com," + employer +
					"\nc2,Anna Schmitt,a.schmitt@twinco.example,\n",
			}
			contacts := AnyMap{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
				"ref": "ref", "full_name": "full_name", "email": "email", "employer": name,
			}}
			run := stageBundle(t, e, uploadZip(t, e, "employer.zip", zipBytes(t, files)).SourceRef, "employer", []AnyMap{companies, contacts})
			predicted := duplicateVerdicts(readImportReport(t, e, run.ID))
			if want := []string{"created=3 updated=0 unchanged=0 skipped=0 duplicates=1", "line 3: contact_duplicate_found"}; !slices.Equal(predicted, want) {
				t.Fatalf("test run = %v, want %v", predicted, want)
			}
			approveAndRead(t, e, run.ID)
			if committed := duplicateVerdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
				t.Fatalf("import = %v, test run said %v", committed, predicted)
			}
		})
	}
}

// duplicateVerdicts is verdicts with the duplicate count, which sits outside
// the four outcomes, and each issue by line and code alone.
func duplicateVerdicts(report importReportDTO) []string {
	d := report.Disposition
	duplicates := 0
	if d.Duplicates != nil {
		duplicates = *d.Duplicates
	}
	out := []string{fmt.Sprintf("created=%d updated=%d unchanged=%d skipped=%d duplicates=%d",
		d.Created, d.Updated, d.Unchanged, d.Skipped, duplicates)}
	for _, issue := range report.Issues {
		out = append(out, fmt.Sprintf("line %d: %s", issue.Line, issue.ReasonCode))
	}
	slices.Sort(out[1:])
	return out
}

// storedCompany is the create body of a company, on domain when one is given.
func storedCompany(name, domain string) AnyMap {
	body := AnyMap{"source": "manual", "display_name": name}
	if domain != "" {
		body["domains"] = []AnyMap{{"domain": domain, "is_primary": true}}
	}
	return body
}
