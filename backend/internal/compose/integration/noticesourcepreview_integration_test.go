// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type sourceReviewEnv struct {
	*apptest.AppEnv
	contactID      string
	caseID         string
	meetingID      string
	meetingVersion int64
	agent          *apptest.MCPClient
}

func TestSourceEvidenceListsOnlyReadableRelevantRecordsAndRechecksAccess(t *testing.T) {
	f := sourceReviewFixture(t)
	p := &preflightEnv{AppEnv: f.AppEnv, user: apptest.SessionUserID(t, f.AppEnv), ws: apptest.InstallationWorkspaceUUID(context.Background(), t, f.Pool).String()}
	colleague := *p
	colleague.user = inviteReplyReader(t, p)
	store := activities.NewStore(f.DB())
	logged, _, err := store.LogActivity(colleague.privacyAdmin(t), activities.LogActivityInput{
		Kind: "meeting", Subject: new("Private appointment"), Body: new("Private booking details"),
		OccurredAt: new(time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)),
		Links:      []activities.ActivityLinkInput{{EntityType: "contact", EntityID: uuidOf(t, f.contactID)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
	input := f.assessment()
	input["selected_evidence"] = AnyMap{"kind": "activity", "id": id.String()}
	preview := f.preview(t, input)
	if _, err := store.SetAudience(colleague.privacyAdmin(t), id, activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatal(err)
	}
	var page consent.SourceEvidencePage
	if status := f.Call(t, "GET", f.sourcePath(), nil, nil, &page); status != http.StatusOK {
		t.Fatal(status)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor != "" || strings.Contains(string(encoded), "Private") || strings.Contains(string(encoded), id.String()) {
		t.Fatalf("private evidence leaked: %s", encoded)
	}
	input["preview_token"] = preview.PreviewToken
	if status := f.Call(t, "POST", f.sourcePath(), input, nil, nil); status != http.StatusNotFound {
		t.Fatalf("revoked evidence confirmed: %d", status)
	}
	delete(input, "preview_token")
	if status := f.Call(t, "POST", f.sourcePath()+"/preview", input, nil, nil); status != http.StatusNotFound {
		t.Fatalf("private evidence previewed: %d", status)
	}
	input["selected_evidence"] = AnyMap{"kind": "activity", "id": ids.NewV7().String()}
	if status := f.Call(t, "POST", f.sourcePath()+"/preview", input, nil, nil); status != http.StatusNotFound {
		t.Fatalf("unrelated evidence previewed: %d", status)
	}
}

func sourceReviewFixture(t *testing.T) *sourceReviewEnv {
	t.Helper()
	doors := newTwoDoorsWith(t, "source-evidence", nil, "read")
	e := doors.e
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "Source Evidence Example", "source": "manual"}, nil, &contact); status != http.StatusCreated {
		t.Fatal(status)
	}
	apptest.DriveNoticeCase(t, e, contact.ID)
	f := &sourceReviewEnv{AppEnv: e, contactID: contact.ID, agent: doors.mcp}
	if err := e.Owner.QueryRow(context.Background(), `SELECT id FROM privacy_notice_case WHERE contact_id = @contact`, pgx.NamedArgs{"contact": contact.ID}).Scan(&f.caseID); err != nil {
		t.Fatal(err)
	}
	var meeting struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Original appointment", "body": "The customer asked to meet.",
		"occurred_at": "2026-01-01T10:00:00Z", "meeting_status": "held",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact.ID}},
	}, nil, &meeting); status != http.StatusCreated {
		t.Fatal(status)
	}
	f.meetingID, f.meetingVersion = meeting.ID, meeting.Version
	return f
}

func (f *sourceReviewEnv) sourcePath() string {
	return "/v1/privacy/notice-case-reviews/" + f.caseID + "/source"
}

func (f *sourceReviewEnv) assessment() AnyMap {
	return AnyMap{
		"kind": "event_or_form", "note": "", "obtained_at": "2026-01-01T10:00:00Z", "timezone": "Europe/Berlin",
		"selected_evidence": AnyMap{"kind": "activity", "id": f.meetingID},
	}
}

func (f *sourceReviewEnv) preview(t *testing.T, input AnyMap) crmcontracts.NoticeSourcePreview {
	t.Helper()
	var preview crmcontracts.NoticeSourcePreview
	if status := f.Call(t, "POST", f.sourcePath()+"/preview", input, nil, &preview); status != http.StatusOK {
		t.Fatalf("preview answered %d", status)
	}
	if len(preview.PreviewToken) != 64 {
		t.Fatalf("preview has no assessment token: %+v", preview)
	}
	return preview
}

func TestTheSourcePreviewAndConfirmationUseTheSameAssessmentAcrossHTTPAndTools(t *testing.T) {
	f := sourceReviewFixture(t)
	var page consent.SourceEvidencePage
	if status := f.Call(t, "GET", f.sourcePath(), nil, nil, &page); status != http.StatusOK {
		t.Fatal(status)
	}
	if page.Original == nil || len(page.Items) != 1 || page.Items[0].Reference.ID.String() != f.meetingID {
		t.Fatalf("source evidence = %+v", page)
	}
	before, audits, events := sourceCaseSnapshot(t, f)
	input := f.assessment()
	preview := f.preview(t, input)
	after, afterAudits, afterEvents := sourceCaseSnapshot(t, f)
	if before != after || audits != afterAudits || events != afterEvents {
		t.Fatal("preview changed the case, audit or outbox")
	}
	if !preview.NoticeRequired || string(preview.Rule) != "art13" || preview.DueAt == nil || preview.DeadlineIsUpperBound {
		t.Fatalf("event or form preview = %+v", preview)
	}
	agent := f.agent
	fromTool := agent.CallOK(t, "read_details", AnyMap{"action": "notice_source_preview", "id": f.caseID, "request_body": input}).Envelope(t).Data
	var toolPreview crmcontracts.NoticeSourcePreview
	if err := json.Unmarshal(fromTool, &toolPreview); err != nil {
		t.Fatal(err)
	}
	if toolPreview.PreviewToken != preview.PreviewToken {
		t.Fatalf("tool and HTTP assessments differ: %+v vs %+v", toolPreview, preview)
	}
	listed := agent.CallOK(t, "read_details", AnyMap{"action": "notice_source_evidence", "id": f.caseID}).Envelope(t).Data
	if !strings.Contains(string(listed), f.meetingID) {
		t.Fatal("tool cannot read the existing source evidence")
	}
	if strings.Contains(string(listed), "captured_by") {
		t.Fatalf("agent learned creator attribution: %s", listed)
	}
	input["preview_token"] = preview.PreviewToken
	var confirmed crmcontracts.NoticeCaseReview
	if status := f.Call(t, "POST", f.sourcePath(), input, nil, &confirmed); status != http.StatusOK {
		t.Fatal(status)
	}
	if string(confirmed.Rule) != string(preview.Rule) || confirmed.DueAt == nil || !confirmed.DueAt.Equal(*preview.DueAt) {
		t.Fatalf("confirmation differs: %+v vs %+v", confirmed, preview)
	}
	var kind, evidence, note string
	if err := f.Owner.QueryRow(context.Background(), `SELECT source_evidence_kind, source_evidence_id, source_review_note FROM privacy_notice_case WHERE id = @id`, pgx.NamedArgs{"id": f.caseID}).Scan(&kind, &evidence, &note); err != nil {
		t.Fatal(err)
	}
	if kind != "activity" || evidence != f.meetingID || note != "" {
		t.Fatalf("saved reference = %s %s, note = %q", kind, evidence, note)
	}
}

func TestSourceEvidencePagesKeepTheOriginalAndDoNotRepeatRecords(t *testing.T) {
	f := sourceReviewFixture(t)
	for range 21 {
		if status := f.Call(t, "POST", "/v1/activities", AnyMap{
			"kind": "meeting", "subject": "Later correspondence", "occurred_at": "2026-02-01T10:00:00Z",
			"links": []AnyMap{{"entity_type": "contact", "entity_id": f.contactID}},
		}, nil, nil); status != http.StatusCreated {
			t.Fatal(status)
		}
	}
	var first, second consent.SourceEvidencePage
	if status := f.Call(t, "GET", f.sourcePath(), nil, nil, &first); status != http.StatusOK {
		t.Fatal(status)
	}
	if len(first.Items) != 20 || first.NextCursor == "" || first.Original == nil {
		t.Fatalf("first page: %+v", first)
	}
	if status := f.Call(t, "GET", f.sourcePath()+"?cursor="+url.QueryEscape(first.NextCursor), nil, nil, &second); status != http.StatusOK {
		t.Fatal(status)
	}
	if len(second.Items) != 2 || second.NextCursor != "" || second.Original == nil || second.Original.Reference != first.Original.Reference {
		t.Fatalf("second page: %+v", second)
	}
	seen := map[ids.UUID]bool{}
	for _, row := range append(first.Items, second.Items...) {
		if seen[row.Reference.ID] {
			t.Fatalf("repeated record %s", row.Reference.ID)
		}
		seen[row.Reference.ID] = true
	}
}

func TestSourceEvidenceRejectsFutureAndUnrelatedRecordsAndUnknownOriginals(t *testing.T) {
	f := sourceReviewFixture(t)
	for _, variant := range []string{"future", "unrelated"} {
		contact := f.contactID
		at := "2026-01-01T10:00:00Z"
		if variant == "future" {
			at = "9999-01-01T10:00:00Z"
		} else {
			var other struct {
				ID string `json:"id"`
			}
			if status := f.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "Unrelated Example", "source": "manual"}, nil, &other); status != http.StatusCreated {
				t.Fatal(status)
			}
			contact = other.ID
		}
		var meeting struct {
			ID string `json:"id"`
		}
		if status := f.Call(t, "POST", "/v1/activities", AnyMap{"kind": "meeting", "subject": variant, "occurred_at": at, "links": []AnyMap{{"entity_type": "contact", "entity_id": contact}}}, nil, &meeting); status != http.StatusCreated {
			t.Fatal(status)
		}
		input := f.assessment()
		input["selected_evidence"] = AnyMap{"kind": "activity", "id": meeting.ID}
		if status := f.Call(t, "POST", f.sourcePath()+"/preview", input, nil, nil); status != http.StatusNotFound {
			t.Fatalf("%s evidence previewed: %d", variant, status)
		}
	}
	var page consent.SourceEvidencePage
	if status := f.Call(t, "GET", f.sourcePath(), nil, nil, &page); status != http.StatusOK {
		t.Fatal(status)
	}
	if len(page.Items) != 1 || page.Original == nil {
		t.Fatalf("evidence: %+v", page)
	}
	input := f.assessment()
	input["selected_evidence"] = page.Original.Reference
	if status := f.Call(t, "POST", f.sourcePath()+"/preview", input, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown original proves a source: %d", status)
	}
}

func sourceCaseSnapshot(t *testing.T, f *sourceReviewEnv) (string, int, int) {
	t.Helper()
	var state string
	var audits, events int
	if err := f.Owner.QueryRow(context.Background(), `SELECT row_to_json(n)::text,
		(SELECT count(*) FROM audit_log WHERE entity_id = @id), (SELECT count(*) FROM event_outbox)
		FROM privacy_notice_case n WHERE id = @id`, pgx.NamedArgs{"id": f.caseID}).Scan(&state, &audits, &events); err != nil {
		t.Fatal(err)
	}
	return state, audits, events
}

func TestASourcePreviewCannotConfirmChangedEvidenceOrAChangedAnswer(t *testing.T) {
	for _, change := range []string{"answer", "note", "evidence", "assignment", "archived", "meeting assertion"} {
		t.Run(change, func(t *testing.T) {
			f := sourceReviewFixture(t)
			input := f.assessment()
			if change == "meeting assertion" {
				input["kind"] = "in_person_permission"
			}
			preview := f.preview(t, input)
			switch change {
			case "meeting assertion":
				input["met_in_person"] = true
			case "answer":
				input["kind"] = "referral"
			case "note":
				input["note"] = "A different explanation."
			case "evidence":
				if status := f.Call(t, "PATCH", "/v1/activities/"+f.meetingID, AnyMap{"body": "A changed source account."}, map[string]string{"If-Match": strconv.FormatInt(f.meetingVersion, 10)}, nil); status != http.StatusOK {
					t.Fatal(status)
				}
			case "assignment":
				if status := f.Call(t, "POST", "/v1/privacy/notice-case-reviews/"+f.caseID+"/assign", AnyMap{"owner_user_id": apptest.SessionUserID(t, f.AppEnv)}, nil, nil); status != http.StatusOK {
					t.Fatal(status)
				}
			case "archived":
				if status := f.Call(t, "DELETE", "/v1/activities/"+f.meetingID, nil, nil, nil); status != http.StatusOK {
					t.Fatal(status)
				}
			}
			input["preview_token"] = preview.PreviewToken
			status := f.Call(t, "POST", f.sourcePath(), input, nil, nil)
			if status != http.StatusUnprocessableEntity && status != http.StatusNotFound {
				t.Fatalf("changed %s confirmed: %d", change, status)
			}
			var state string
			if err := f.Owner.QueryRow(context.Background(), `SELECT state FROM privacy_notice_case WHERE id = @id`, pgx.NamedArgs{"id": f.caseID}).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "source_review" {
				t.Fatalf("refused confirmation changed state to %s", state)
			}
		})
	}
}

func TestASelectedSourceRecordsTheSameFaceToFaceMeetingAsTheSourceAnswer(t *testing.T) {
	f := sourceReviewFixture(t)
	var created time.Time
	if err := f.Owner.QueryRow(context.Background(), `SELECT created_at FROM privacy_notice_case WHERE id = @id`, pgx.NamedArgs{"id": f.caseID}).Scan(&created); err != nil {
		t.Fatal(err)
	}
	met := created.Add(-time.Hour)
	if status := f.Call(t, "PATCH", "/v1/activities/"+f.meetingID, AnyMap{"occurred_at": met.Format(time.RFC3339Nano)}, map[string]string{"If-Match": strconv.FormatInt(f.meetingVersion, 10)}, nil); status != http.StatusOK {
		t.Fatal(status)
	}
	input := f.assessment()
	input["kind"], input["met_in_person"], input["obtained_at"] = "in_person_permission", true, met.Format(time.RFC3339Nano)
	preview := f.preview(t, input)
	if preview.NoticeRequired || string(preview.Rule) != "none" || preview.DueAt != nil {
		t.Fatalf("meeting preview: %+v", preview)
	}
	input["preview_token"] = preview.PreviewToken
	if status := f.Call(t, "POST", f.sourcePath(), input, nil, nil); status != http.StatusOK {
		t.Fatal(status)
	}
	var note string
	if err := f.Owner.QueryRow(context.Background(), `SELECT note FROM consent_qualifying_event WHERE contact_id = @contact AND kind = 'in_person'`, pgx.NamedArgs{"contact": f.contactID}).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != "activity:"+f.meetingID {
		t.Fatalf("meeting evidence: %q", note)
	}
	var typedNote string
	if err := f.Owner.QueryRow(context.Background(), `SELECT source_review_note FROM privacy_notice_case WHERE id = @id`, pgx.NamedArgs{"id": f.caseID}).Scan(&typedNote); err != nil || typedNote != "" {
		t.Fatalf("typed note: %q, %v", typedNote, err)
	}
}
