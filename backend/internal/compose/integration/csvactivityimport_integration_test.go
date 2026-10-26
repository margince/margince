// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// activityImportParts maps activityHeader onto the targets of the same names,
// which is how every column of these files is spelled.
func activityImportParts() []AnyMap {
	mapping := AnyMap{}
	for column := range strings.SplitSeq(activityHeader, ",") {
		mapping[column] = column
	}
	return []AnyMap{{"file": "activities.csv", "object": "activity", "mapping": mapping}}
}

// contactAndActivityParts is a contacts file and an activities file.
func contactAndActivityParts() []AnyMap {
	return append(bundleParts()[:1], activityImportParts()...)
}

// importedActivity is one activity as the importer left it, read from its table.
type importedActivity struct {
	ID, Kind                           string
	OccurredAt                         time.Time
	DurationSeconds                    *int
	MeetingStatus, Direction, SourceID *string
	SourceSystem, AuthorName           *string
	AuthorID, HostID                   *string
	Links                              int
}

func activityNamed(t *testing.T, e *apptest.AppEnv, subject string) importedActivity {
	t.Helper()
	var a importedActivity
	err := e.Owner.QueryRow(context.Background(), `
		SELECT a.id::text, a.kind, a.occurred_at, a.duration_seconds, a.meeting_status, a.direction, a.source_id,
		       a.source_system, a.source_author_name, a.source_author_id::text, a.host_user_id::text,
		       (SELECT count(*) FROM activity_link l WHERE l.activity_id = a.id)
		  FROM activity a WHERE a.subject = $1 AND a.archived_at IS NULL`, subject).
		Scan(&a.ID, &a.Kind, &a.OccurredAt, &a.DurationSeconds, &a.MeetingStatus, &a.Direction, &a.SourceID,
			&a.SourceSystem, &a.AuthorName, &a.AuthorID, &a.HostID, &a.Links)
	if err != nil {
		t.Fatalf("reading activity %q: %v", subject, err)
	}
	return a
}

// liveActivities counts the live activities carrying a subject.
func liveActivities(t *testing.T, e *apptest.AppEnv, subject string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity WHERE subject = $1 AND archived_at IS NULL`, subject).Scan(&n); err != nil {
		t.Fatalf("counting activities %q: %v", subject, err)
	}
	return n
}

// importActivities uploads an activities file with any other files, stages it,
// commits it and answers the finished report.
func importActivities(t *testing.T, e *apptest.AppEnv, files map[string]string, parts []AnyMap) (importRunDTO, importReportDTO) {
	t.Helper()
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", parts)
	approveAndRead(t, e, run.ID)
	return run, readImportReport(t, e, run.ID)
}

const activityHeader = "ref,type,subject,body,occurred_at,links,duration_minutes,meeting_status,direction"

func TestAnActivitiesFileLandsAtItsPastDatesUnderItsLinks(t *testing.T) {
	e := setupImportApp(t)
	_, report := importActivities(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@act-land.example,Jo Tran\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Kick-off note,Met at the fair,2025-03-02,contact:P-1,,,\n" +
			"A-2,call,Pricing call,Asked for a quote,2025-03-04T09:30:00+07:00,contact:jo@act-land.example,25,,outbound\n" +
			"A-3,meeting,Site visit,,2025-03-09,contact:P-1,90,held,\n" +
			"A-4,email,Quote sent,See attached,2025-03-11T10:00:00Z,contact:P-1,,,outbound\n",
	}, append(bundleParts()[:1], activityImportParts()...))
	if report.Disposition.Created != 5 || report.Disposition.Skipped != 0 {
		t.Fatalf("disposition = %+v, issues %+v, want the contact and four activities created", report.Disposition, report.Issues)
	}
	call := activityNamed(t, e, "Pricing call")
	if call.Kind != "call" || call.DurationSeconds == nil || *call.DurationSeconds != 1500 ||
		!call.OccurredAt.Equal(time.Date(2025, 3, 4, 2, 30, 0, 0, time.UTC)) || call.Links != 1 {
		t.Fatalf("call = %+v, want a 25 minute call on 2025-03-04 02:30 UTC under one link", call)
	}
	if call.SourceSystem == nil || !strings.HasPrefix(*call.SourceSystem, "mirror:csv") || call.SourceID != nil {
		t.Fatalf("call provenance = %+v, want the importer's namespace and no source key", call)
	}
	meeting := activityNamed(t, e, "Site visit")
	if meeting.Kind != "meeting" || meeting.MeetingStatus == nil || *meeting.MeetingStatus != "held" ||
		!meeting.OccurredAt.Equal(time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("meeting = %+v, want a held meeting at midnight UTC of its day", meeting)
	}
}

func TestAnImportedActivityStillCountsAsEngagement(t *testing.T) {
	e := setupImportApp(t)
	contactID := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Engaged Eli",
		"emails": []AnyMap{{"email": "eli@act-engage.example", "email_type": "work", "is_primary": true}},
	})
	lastActivity := func() *time.Time {
		var at *time.Time
		if err := e.Owner.QueryRow(context.Background(),
			`SELECT last_activity_at FROM contact WHERE id = $1`, contactID).Scan(&at); err != nil {
			t.Fatalf("reading the contact's last activity: %v", err)
		}
		return at
	}
	run, _ := importActivities(t, e, map[string]string{"activities.csv": activityHeader + "\n" +
		fmt.Sprintf("A-1,note,Older note,,2024-01-10,contact:%s,,,\n", contactID) +
		fmt.Sprintf("A-2,call,Newest call,,2024-06-20,contact:%s,10,,\n", contactID) +
		fmt.Sprintf("A-3,meeting,Called off,,2025-02-01,contact:%s,,canceled,\n", contactID)}, activityImportParts())
	want := time.Date(2024, 6, 20, 0, 0, 0, 0, time.UTC)
	if got := lastActivity(); got == nil || !got.Equal(want) {
		t.Fatalf("last activity = %v, want the newest imported one that counts, %v: a called-off meeting is no activity", got, want)
	}
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	if got := lastActivity(); got != nil {
		t.Fatalf("last activity after the undo = %v, want it recomputed to none", got)
	}
}

func TestAReUploadedActivityIsNotDuplicated(t *testing.T) {
	e := setupImportApp(t)
	files := func(subject string) map[string]string {
		return map[string]string{
			"contacts.csv":   "ref,email,name\nP-1,jo@act-reup.example,Jo\n",
			"activities.csv": activityHeader + "\n" + fmt.Sprintf("A-1,note,%s,,2025-03-02,contact:P-1,,,\n", subject),
		}
	}
	parts := contactAndActivityParts()
	importActivities(t, e, files("First wording"), parts)
	_, same := importActivities(t, e, files("First wording"), parts)
	if same.Disposition.Created != 0 || same.Disposition.Unchanged != 2 {
		t.Fatalf("same file again = %+v, want everything unchanged", same.Disposition)
	}
	_, edited := importActivities(t, e, files("Second wording"), parts)
	if edited.Disposition.Updated != 1 || edited.Disposition.Created != 0 {
		t.Fatalf("edited file = %+v, want the note updated, not made twice", edited.Disposition)
	}
	if liveActivities(t, e, "Second wording") != 1 || liveActivities(t, e, "First wording") != 0 {
		t.Fatal("want the one note under its new subject and none under the old")
	}
}

func TestAnActivityReImportedAfterUndoLandsAsANewRow(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"contacts.csv":   "ref,email,name\nP-1,jo@act-undo.example,Jo\n",
		"activities.csv": activityHeader + "\nA-1,note,Come back,,2025-03-02,contact:P-1,,,\n",
	}
	parts := contactAndActivityParts()
	run, _ := importActivities(t, e, files, parts)
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	if n := liveActivities(t, e, "Come back"); n != 0 {
		t.Fatalf("after the undo %d live copies, want none", n)
	}
	_, report := importActivities(t, e, files, parts)
	if report.Disposition.Skipped != 0 || liveActivities(t, e, "Come back") != 1 {
		t.Fatalf("after re-importing: report %+v and %d live copies, want one new live activity", report.Disposition, liveActivities(t, e, "Come back"))
	}
}

func TestAnInvisibleLinkTargetSkipsTheRowNotTheRun(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	colleague := createdID(t, e, "/v1/users", AnyMap{"email": "colleague@act-hidden.test", "display_name": "Colleague", "role": "rep"})
	hidden := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Hidden Hana",
		"emails": []AnyMap{{"email": "hana@act-hidden.test", "email_type": "work", "is_primary": true}},
	})
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden, colleague); err != nil {
		t.Fatalf("making the contact private to the colleague: %v", err)
	}
	visible := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Visible Vic",
		"emails": []AnyMap{{"email": "vic@act-hidden.test", "email_type": "work", "is_primary": true}},
	})
	_, report := importActivities(t, e, map[string]string{"activities.csv": activityHeader + "\n" +
		fmt.Sprintf("A-1,note,About the hidden one,,2025-03-02,contact:%s,,,\n", hidden) +
		fmt.Sprintf("A-2,note,About the visible one,,2025-03-02,contact:%s,,,\n", visible)}, activityImportParts())
	if report.Disposition.Created != 1 || report.Disposition.Skipped != 1 {
		t.Fatalf("report = %+v, want the hidden row skipped and the other landed", report.Disposition)
	}
	if liveActivities(t, e, "About the visible one") != 1 || liveActivities(t, e, "About the hidden one") != 0 {
		t.Fatal("want only the activity about the contact the importer may see")
	}
}

func TestTheDryRunGivesTheReasonTheCommitGivesForEachRefusedActivityRow(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@act-refused.example,Jo\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,message,A chat,,2025-03-02,contact:P-1,,,\n" +
			"A-2,task,A to-do,,2025-03-02,contact:P-1,,,\n" +
			"A-3,note,No zone,,2025-03-02 10:00,contact:P-1,,,\n" +
			"A-4,note,Timed note,,2025-03-02,contact:P-1,30,,\n" +
			"A-5,note,Loose end,,2025-03-02,,,,\n" +
			"A-6,note,Wrong link,,2025-03-02,project:Apollo,,,\n" +
			"A-7,note,Fine,,2025-03-02,contact:P-1,,,\n",
	}
	parts := contactAndActivityParts()
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if committed.Disposition.Created != 2 || committed.Disposition.Skipped != 6 {
		t.Fatalf("commit = %+v, issues %+v, want the contact and one activity landed, six rows skipped", committed.Disposition, committed.Issues)
	}
	if preview.Disposition.Created != committed.Disposition.Created || preview.Disposition.Skipped != committed.Disposition.Skipped {
		t.Fatalf("dry run = %+v, commit = %+v, want the same counts", preview.Disposition, committed.Disposition)
	}
	reasons := func(r importReportDTO) map[int]string {
		out := map[int]string{}
		for _, issue := range r.Issues {
			out[issue.Line] = issue.Reason
		}
		return out
	}
	got, want := reasons(preview), reasons(committed)
	for line, reason := range want {
		if reason == "" || got[line] != reason {
			t.Errorf("line %d: dry run says %q, commit says %q", line, got[line], reason)
		}
	}
}

func TestAnActivityLinksToADealAndAContactInTheSameZip(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	_, report := importActivities(t, e, map[string]string{
		"companies.csv": "ref,name\nCO-1,Five Files GmbH\n",
		"contacts.csv":  "ref,email,name\nP-1,jo@five-files.example,Jo Tran\n",
		"leads.csv":     "email,name\nlena@five-files.example,Lena Lead\n",
		"deals.csv":     fmt.Sprintf("ref,name,pipeline,stage,company_ref,contact_ref\nD-1,Five files deal,%s,%s,CO-1,P-1\n", pipeline, open),
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Deal and contact,,2025-03-02,deal:D-1; contact:P-1,,,\n" +
			"A-2,email,To the company,,2025-03-03,company:CO-1,,,outbound\n" +
			"A-3,call,A deal named like a company,,2025-03-04,deal:Five Files GmbH,,,\n",
	}, []AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}},
		{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{"ref": "ref", "email": "email", "name": "full_name"}},
		{"file": "leads.csv", "object": "lead", "mapping": AnyMap{"email": "email", "name": "full_name"}},
		{"file": "deals.csv", "object": "deal", "mapping": AnyMap{"ref": "ref", "name": "name", "pipeline": "pipeline", "stage": "stage", "company_ref": "company_ref", "contact_ref": "contact_ref"}},
		activityImportParts()[0],
	})
	if report.Disposition.Created != 6 || report.Disposition.Skipped != 1 {
		t.Fatalf("report = %+v, issues %+v, want five records and two activities, and the deal named as a company skipped", report.Disposition, report.Issues)
	}
	both := activityNamed(t, e, "Deal and contact")
	if both.Links != 2 {
		t.Fatalf("activity = %+v, want it filed under the deal and the contact", both)
	}
	var dealLinked bool
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM activity_link l JOIN deal d ON d.id = l.deal_id
		                WHERE l.activity_id = $1 AND d.name = 'Five files deal')`, both.ID).Scan(&dealLinked); err != nil || !dealLinked {
		t.Fatalf("deal link present = %v (%v), want the activity linked to the imported deal", dealLinked, err)
	}
	if n := liveActivities(t, e, "A deal named like a company"); n != 0 {
		t.Fatalf("%d live copies of the activity whose deal link names a company, want none", n)
	}
}

func TestACorrectedFileWithNoTypeColumnHoldsADurationToTheStoredKind(t *testing.T) {
	e := setupImportApp(t)
	contacts := "ref,email,name\nP-1,jo@act-fix.example,Jo\n"
	parts := contactAndActivityParts()
	importActivities(t, e, map[string]string{
		"contacts.csv":   contacts,
		"activities.csv": activityHeader + "\nA-1,note,Fix note,,2025-03-02,contact:P-1,,,\n",
	}, parts)
	untyped := []AnyMap{parts[0], {
		"file": "activities.csv", "object": "activity",
		"mapping": AnyMap{"ref": "ref", "subject": "subject", "occurred_at": "occurred_at", "links": "links", "duration_minutes": "duration_minutes"},
	}}
	files := map[string]string{
		"contacts.csv":   contacts,
		"activities.csv": "ref,subject,occurred_at,links,duration_minutes\nA-1,Fix note,2025-03-02,contact:P-1,30\n",
	}
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", untyped)
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	for name, r := range map[string]importReportDTO{"preview": preview, "commit": committed} {
		if r.Disposition.Updated != 0 || r.Disposition.Skipped != 1 {
			t.Errorf("%s = %+v, issues %+v, want the note skipped and not rewritten", name, r.Disposition, r.Issues)
		}
	}
	if note := activityNamed(t, e, "Fix note"); note.DurationSeconds != nil {
		t.Fatalf("note duration = %d seconds, want none: the row was refused whole", *note.DurationSeconds)
	}
}

func TestATestRunResolvesAnActivityLinkByTheNaturalKeyOfARowInTheSameUpload(t *testing.T) {
	e := setupImportApp(t)
	parts := append([]AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name", "domain": "domain"}},
		{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{"ref": "ref", "email": "email", "name": "full_name"}},
	}, activityImportParts()...)
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{
		"companies.csv": "ref,name,domain\nC-1,Natural Key Works,natural-key.example\n",
		"contacts.csv":  "ref,email,name\nP-1,Jo@Natural-Key.example,Jo Tran\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,By ref,,2025-03-02,contact:P-1,,,\n" +
			"A-2,note,By email,,2025-03-03,contact:jo@natural-key.example,,,\n" +
			"A-3,note,By name,,2025-03-04,company:Natural Key Works,,,\n" +
			"A-4,note,By domain,,2025-03-05,company:natural-key.example,,,\n",
	})).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	landed := readImportReport(t, e, run.ID)
	if len(preview.Issues) != 0 || preview.Disposition.Skipped != 0 {
		t.Fatalf("test run = %+v, issues %+v, want no broken link", preview.Disposition, preview.Issues)
	}
	if !reflect.DeepEqual(preview.Disposition, landed.Disposition) {
		t.Fatalf("test run %+v and commit %+v disagree", preview.Disposition, landed.Disposition)
	}
}

// issuesWith counts a report's issues that carry one reason code.
func issuesWith(r importReportDTO, code string) int {
	n := 0
	for _, issue := range r.Issues {
		if issue.ReasonCode == code {
			n++
		}
	}
	return n
}

func TestOneImportFilesOneCopyOfANoteItsExportRepeats(t *testing.T) {
	e := setupImportApp(t)
	var rows strings.Builder
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&rows, "A-%d,note,Fair recap,Met at the fair,2025-03-02,contact:P-1,,,\n", i)
	}
	files := map[string]string{
		"contacts.csv":   "ref,email,name\nP-1,jo@act-twin.example,Jo\n",
		"activities.csv": activityHeader + "\n" + rows.String(),
	}
	parts := contactAndActivityParts()
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	report := readImportReport(t, e, run.ID)
	if n := liveActivities(t, e, "Fair recap"); n != 1 {
		t.Fatalf("%d live copies of the repeated note, want 1", n)
	}
	if report.Disposition.Created != 2 || report.Disposition.Skipped != 3 || issuesWith(report, "note_repeated") != 3 {
		t.Fatalf("commit = %+v, issues %+v, want the contact and one note, three rows named as repeats", report.Disposition, report.Issues)
	}
	if preview.Disposition.Created != 2 || issuesWith(preview, "note_repeated") != 3 {
		t.Fatalf("dry run = %+v, issues %+v, want the same three repeats the commit names", preview.Disposition, preview.Issues)
	}
	_, again := importActivities(t, e, files, parts)
	if again.Disposition.Created != 0 || liveActivities(t, e, "Fair recap") != 1 {
		t.Fatalf("same file again = %+v, want nothing filed a second time", again.Disposition)
	}
	files["activities.csv"] = activityHeader + "\nA-9,note,Fair recap,Met at the fair,2025-03-02,contact:P-1,,,\n"
	if _, later := importActivities(t, e, files, parts); liveActivities(t, e, "Fair recap") != 2 {
		t.Fatalf("a later run = %+v, want its own row filed: runs do not collapse into each other", later.Disposition)
	}
}

func TestARepeatedTextStaysApartAtAnotherTimeOnAnotherRecordOrOfAnotherKind(t *testing.T) {
	e := setupImportApp(t)
	_, report := importActivities(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@act-apart.example,Jo\nP-2,li@act-apart.example,Li\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Weekly sync,Went through the plan,2025-03-02,contact:P-1,,,\n" +
			"A-2,note,Weekly sync,Went through the plan,2025-03-09,contact:P-1,,,\n" +
			"A-3,note,Weekly sync,Went through the plan,2025-03-02,contact:P-2,,,\n" +
			"A-4,call,Weekly sync,Went through the plan,2025-03-02,contact:P-1,,,\n" +
			"A-5,call,Weekly sync,Went through the plan,2025-03-02,contact:P-1,,,\n",
	}, contactAndActivityParts())
	if report.Disposition.Skipped != 0 || liveActivities(t, e, "Weekly sync") != 5 {
		t.Fatalf("report = %+v, issues %+v, want all five activities filed", report.Disposition, report.Issues)
	}
}

func TestARepeatedNoteStaysApartUnderAnotherAuthorOrOwner(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	createdID(t, e, "/v1/users", AnyMap{"email": "owner@act-attrib.test", "display_name": "Owner", "role": "rep"})
	parts := contactAndActivityParts()
	for _, part := range parts {
		if mapping, isActivities := part["mapping"].(AnyMap); isActivities && part["object"] == "activity" {
			mapping["author"], mapping["owner_email"] = "author", "owner_email"
		}
	}
	_, report := importActivities(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@act-attrib.example,Jo\n",
		"activities.csv": activityHeader + ",author,owner_email\n" +
			"A-1,note,Board recap,Agreed the budget,2025-03-02,contact:P-1,,,,Ana Ruiz,\n" +
			"A-2,note,Board recap,Agreed the budget,2025-03-02,contact:P-1,,,,Ben Ode,\n" +
			"A-3,note,Board recap,Agreed the budget,2025-03-02,contact:P-1,,,,Ana Ruiz,owner@act-attrib.test\n",
	}, parts)
	if report.Disposition.Skipped != 0 || liveActivities(t, e, "Board recap") != 3 {
		t.Fatalf("report = %+v, issues %+v, want three notes: another author or owner is another note", report.Disposition, report.Issues)
	}
}

func TestARowIdShapedLikeANoteKeyIsAnOrdinaryRow(t *testing.T) {
	e := setupImportApp(t)
	keyShaped := "activity_note_twin:" + strings.Repeat("0", 64)
	_, report := importActivities(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@act-keyshape.example,Jo\n",
		"activities.csv": activityHeader + "\n" +
			keyShaped + ",note,Shaped row,Its own note,2025-03-02,contact:P-1,,,\n" +
			strings.Repeat("0", 64) + ",note,Bare hash row,Its own note,2025-03-02,contact:P-1,,,\n" +
			"A-1,note,Real note,Met at the fair,2025-03-02,contact:P-1,,,\n" +
			"A-2,note,Real note,Met at the fair,2025-03-02,contact:P-1,,,\n",
	}, contactAndActivityParts())
	if report.Disposition.Created != 4 || issuesWith(report, "note_repeated") != 1 {
		t.Fatalf("report = %+v, issues %+v, want the contact, both key-shaped rows and one real note, and the real repeat collapsed", report.Disposition, report.Issues)
	}
	for _, subject := range []string{"Shaped row", "Bare hash row", "Real note"} {
		if n := liveActivities(t, e, subject); n != 1 {
			t.Fatalf("%d live %q notes, want 1", n, subject)
		}
	}
}
