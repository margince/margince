// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
)

// previewAndCommit stages one bundle, reads the test run's report, approves it
// and reads the commit's report.
func previewAndCommit(t *testing.T, files map[string]string) (preview, commit importReportDTO) {
	t.Helper()
	e := setupImportApp(t)
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", contactAndActivityParts())
	preview = readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	return preview, readImportReport(t, e, run.ID)
}

// A note that links one record of the upload by its ref and again by its email
// is one note twice. The dry run says so as the commit does.
func TestTheTestRunNamesAsRepeatsTheNotesTheCommitCollapsesWhenARecordIsSpelledTwoWays(t *testing.T) {
	t.Parallel()
	preview, commit := previewAndCommit(t, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@twin-spell.example,Jo\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Fair recap,Met at the fair,2025-03-02,contact:P-1,,,\n" +
			"A-2,note,Fair recap,Met at the fair,2025-03-02,contact:jo@twin-spell.example,,,\n",
	})
	if got := issuesWith(commit, "note_repeated"); got != 1 {
		t.Fatalf("commit names %d repeats, want 1: %+v", got, commit.Issues)
	}
	if got := issuesWith(preview, "note_repeated"); got != 1 || preview.Disposition.Created != 2 {
		t.Fatalf("test run = %+v, %d repeats, want the contact, one note and one repeat as the commit", preview.Disposition, got)
	}
}

// Two notes of one text on two records the upload creates stay two notes, in
// the test run and in the commit.
func TestTheTestRunKeepsApartNotesOnDifferentRecordsOfTheSameUpload(t *testing.T) {
	t.Parallel()
	preview, commit := previewAndCommit(t, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@twin-apart.example,Jo\nP-2,li@twin-apart.example,Li\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Fair recap,Met at the fair,2025-03-02,contact:P-1,,,\n" +
			"A-2,note,Fair recap,Met at the fair,2025-03-02,contact:li@twin-apart.example,,,\n",
	})
	if issuesWith(commit, "note_repeated") != 0 || issuesWith(preview, "note_repeated") != 0 {
		t.Fatalf("repeats: test run %+v, commit %+v, want none", preview.Issues, commit.Issues)
	}
	if preview.Disposition.Created != 4 || commit.Disposition.Created != 4 {
		t.Fatalf("created: test run %d, commit %d, want 4", preview.Disposition.Created, commit.Disposition.Created)
	}
}

// A repeat that falls in a later slice than its twin is named by the test run
// as often as the commit collapses it.
func TestTheTestRunNamesARepeatInALaterSliceAsTheCommitDoes(t *testing.T) {
	t.Parallel()
	var rows strings.Builder
	rows.WriteString("A-1,note,Fair recap,Met at the fair,2025-03-02,contact:P-1,,,\n")
	for i := 2; i <= 500; i++ {
		fmt.Fprintf(&rows, "A-%d,note,Filler %d,Body,2025-03-03,contact:P-1,,,\n", i, i)
	}
	rows.WriteString("A-501,note,Fair recap,Met at the fair,2025-03-02,contact:jo@twin-slice.example,,,\n")
	preview, commit := previewAndCommit(t, map[string]string{
		"contacts.csv":   "ref,email,name\nP-1,jo@twin-slice.example,Jo\n",
		"activities.csv": activityHeader + "\n" + rows.String(),
	})
	if issuesWith(commit, "note_repeated") != 1 || issuesWith(preview, "note_repeated") != 1 {
		t.Fatalf("repeats: test run %d, commit %d, want 1 each", issuesWith(preview, "note_repeated"), issuesWith(commit, "note_repeated"))
	}
	if preview.Disposition.Created != commit.Disposition.Created {
		t.Fatalf("created: test run %d, commit %d", preview.Disposition.Created, commit.Disposition.Created)
	}
}

// A company whose ref reads like the identity of a company named by its name
// is still a different company. Notes of one text on the two stay two notes.
func TestTheTestRunKeepsApartACompanyRefThatLooksLikeANamedCompany(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	parts := append([]AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}},
	}, activityImportParts()...)
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{
		"companies.csv": "ref,name\nname:acme,Other Works\nC-9,Acme\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Visit,Went by,2025-03-02,company:name:acme,,,\n" +
			"A-2,note,Visit,Went by,2025-03-02,company:Acme,,,\n",
	})).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	commit := readImportReport(t, e, run.ID)
	if issuesWith(preview, "note_repeated") != 0 || issuesWith(commit, "note_repeated") != 0 {
		t.Fatalf("repeats: test run %+v, commit %+v, want none", preview.Issues, commit.Issues)
	}
	if preview.Disposition.Created != commit.Disposition.Created {
		t.Fatalf("created: test run %d, commit %d", preview.Disposition.Created, commit.Disposition.Created)
	}
}
