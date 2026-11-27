// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// archiveFixture is archived mail of one contact. One old message nothing
// keeps. One on a company under legal hold. One opt-out receipt inside its
// floor. One archived last month, and one live message.
type archiveFixture struct {
	contact                           ids.UUID
	free, held, receipt, recent, live ids.UUID
}

func seedArchive(t *testing.T, e *Env) archiveFixture {
	t.Helper()
	f := archiveFixture{
		contact: e.SeedContact(t, "Archived Subject", nil),
		free:    ids.NewV7(), held: ids.NewV7(), receipt: ids.NewV7(), recent: ids.NewV7(), live: ids.NewV7(),
	}
	company := e.SeedCompany(t, "Held Company", nil)
	e.WsExec(t, `UPDATE company SET legal_hold = true WHERE id = $1`, company)
	insert := func(id ids.UUID, subject, age string, archived bool) {
		e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, archived_at, source, captured_by)
			VALUES ($1, 'email', $2, 'What was said.', now() - $3::interval,
			        CASE WHEN $4 THEN now() END, 'manual', 'human:x')`, id, subject, age, archived)
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, id, f.contact)
	}
	insert(f.free, "Old offer", "4 years", true)
	insert(f.held, "Disputed invoice", "4 years", true)
	insert(f.receipt, "Your refusal is recorded", "4 years", true)
	insert(f.recent, "Last month", "30 days", true)
	insert(f.live, "Still live", "4 years", false)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, f.held, company)
	e.WsExec(t, `
		INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		                            consent_purpose, status, sent_at, kept_until)
		VALUES ($1, $2, $3, 'gmail', $4, jsonb_build_array('archived@example.test'::text), '[]'::jsonb,
		        'Your refusal is recorded', 'We will not write again.', 'transactional', 'sent',
		        now() - interval '4 years', now() + interval '1 year')`,
		ids.NewV7(), f.receipt, e.Rep1, ids.NewV7().String()+"@margince.test")
	return f
}

// archiveReviewerCtx is a signed-in user holding grant on the retention
// authority, plus reads, any further object grants.
func archiveReviewerCtx(e *Env, grant principal.ObjectGrant, reads ...string) context.Context {
	objects := map[string]principal.ObjectGrant{"retention_policy": grant}
	for _, object := range reads {
		objects[object] = principal.ObjectGrant{Read: true}
	}
	return e.As(ids.NewV7(), nil, principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll})
}

func listArchive(t *testing.T, h interface {
	ListArchivedActivities(http.ResponseWriter, *http.Request, crmcontracts.ListArchivedActivitiesParams)
}, as *http.Request, params crmcontracts.ListArchivedActivitiesParams,
) (int, []crmcontracts.ArchivedActivity) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ListArchivedActivities(rec, as, params)
	var page struct {
		Data []crmcontracts.ArchivedActivity `json:"data"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode archive page: %v", err)
		}
	}
	return rec.Code, page.Data
}

// TestTheArchiveReviewListsOldArchivedMailWithWhatKeepsIt reads the archive as
// the controller filters it: by age and by contact, each record with its keeper.
func TestTheArchiveReviewListsOldArchivedMailWithWhatKeepsIt(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	f := seedArchive(t, e)
	h := compose.NewPrivacyHandlers(e.Pool)
	admin := httptest.NewRequest(http.MethodGet, "/v1/retention/archive", nil).
		WithContext(archiveReviewerCtx(e, principal.ObjectGrant{Read: true, Delete: true}))

	yearAgo := clocktest.Now(t).AddDate(-1, 0, 0)
	contact := openapi_types.UUID(f.contact)
	code, rows := listArchive(t, h, admin, crmcontracts.ListArchivedActivitiesParams{Before: &yearAgo, ContactId: &contact})
	if code != http.StatusOK {
		t.Fatalf("list = %d, want 200", code)
	}
	kept := map[ids.UUID]string{}
	for _, row := range rows {
		reason := ""
		if row.Kept != nil {
			reason = string(*row.Kept)
		}
		kept[ids.UUID(row.ActivityId)] = reason
	}
	want := map[ids.UUID]string{f.free: "", f.held: "legal_hold", f.receipt: "opt_out_record"}
	if len(kept) != len(want) {
		t.Fatalf("listed %v, want the three archived before a year ago: %v", kept, want)
	}
	for id, reason := range want {
		if got, ok := kept[id]; !ok || got != reason {
			t.Errorf("%s kept = %q (listed %v), want %q", id, got, ok, reason)
		}
	}

	other := openapi_types.UUID(e.SeedContact(t, "Nobody Archived", nil))
	if _, rows := listArchive(t, h, admin, crmcontracts.ListArchivedActivitiesParams{ContactId: &other}); len(rows) != 0 {
		t.Errorf("a contact with no archived mail listed %d records", len(rows))
	}
}

// TestTheArchiveReviewNamesOnlyThePartiesTheReaderMayRead lists the record
// linked to a contact and a company twice. A reviewer holding contact and
// company read sees both names. One holding only the retention authority sees
// the same ids with the names left empty.
func TestTheArchiveReviewNamesOnlyThePartiesTheReaderMayRead(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	f := seedArchive(t, e)
	h := compose.NewPrivacyHandlers(e.Pool)
	contact := openapi_types.UUID(f.contact)
	heldOf := func(reads ...string) crmcontracts.ArchivedActivity {
		t.Helper()
		as := httptest.NewRequest(http.MethodGet, "/v1/retention/archive", nil).
			WithContext(archiveReviewerCtx(e, principal.ObjectGrant{Read: true}, reads...))
		code, rows := listArchive(t, h, as, crmcontracts.ListArchivedActivitiesParams{ContactId: &contact})
		if code != http.StatusOK {
			t.Fatalf("list with reads %v = %d, want 200", reads, code)
		}
		for _, row := range rows {
			if ids.UUID(row.ActivityId) == f.held {
				return row
			}
		}
		t.Fatalf("the held record is missing from the list with reads %v", reads)
		return crmcontracts.ArchivedActivity{}
	}

	named := heldOf("contact", "company")
	if len(named.Contacts) != 1 || named.Contacts[0].Name != "Archived Subject" {
		t.Errorf("contacts with the contact grant = %+v, want Archived Subject", named.Contacts)
	}
	if len(named.Companies) != 1 || named.Companies[0].Name != "Held Company" {
		t.Errorf("companies with the company grant = %+v, want Held Company", named.Companies)
	}

	bare := heldOf()
	if len(bare.Contacts) != 1 || ids.UUID(bare.Contacts[0].Id) != f.contact || bare.Contacts[0].Name != "" {
		t.Errorf("contacts without the contact grant = %+v, want the id with no name", bare.Contacts)
	}
	if len(bare.Companies) != 1 || bare.Companies[0].Name != "" {
		t.Errorf("companies without the company grant = %+v, want one with no name", bare.Companies)
	}
}

// TestAnAdminDeletesOldArchivedMailAndTheKeptRecordsStay deletes everything
// the review lists. Only the record nothing keeps is erased, with its audit
// certificate and event. A reader without the grant is refused.
func TestAnAdminDeletesOldArchivedMailAndTheKeptRecordsStay(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	f := seedArchive(t, e)
	h := compose.NewPrivacyHandlers(e.Pool)
	body := `{"activity_ids":["` + f.free.String() + `","` + f.held.String() + `","` + f.receipt.String() + `","` + f.live.String() + `"]}`
	deleteAs := func(grant principal.ObjectGrant) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.DeleteArchivedActivities(rec, httptest.NewRequest(http.MethodPost, "/v1/retention/archive/delete",
			strings.NewReader(body)).WithContext(archiveReviewerCtx(e, grant)), crmcontracts.DeleteArchivedActivitiesParams{})
		return rec
	}

	if rec := deleteAs(principal.ObjectGrant{Read: true}); rec.Code != http.StatusForbidden {
		t.Fatalf("a reader without the delete grant = %d, want 403 (%s)", rec.Code, rec.Body)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND body IS NULL`,
		[]ids.UUID{f.free, f.held, f.receipt}); n != 0 {
		t.Fatalf("the refused request erased %d records", n)
	}

	rec := deleteAs(principal.ObjectGrant{Read: true, Delete: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	var out crmcontracts.ArchiveDeletionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Deleted) != 1 || ids.UUID(out.Deleted[0]) != f.free {
		t.Errorf("deleted %v, want only %s", out.Deleted, f.free)
	}
	gotKept := map[ids.UUID]crmcontracts.ArchiveKeptReason{}
	for _, k := range out.Kept {
		gotKept[ids.UUID(k.ActivityId)] = k.Reason
	}
	wantKept := map[ids.UUID]crmcontracts.ArchiveKeptReason{
		f.held:    crmcontracts.ArchiveKeptReasonLegalHold,
		f.receipt: crmcontracts.ArchiveKeptReasonOptOutRecord,
		f.live:    crmcontracts.ArchiveKeptReasonNotArchived,
	}
	for id, reason := range wantKept {
		if gotKept[id] != reason {
			t.Errorf("%s kept as %q, want %q (all kept: %v)", id, gotKept[id], reason, gotKept)
		}
	}

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND subject = 'Erased' AND body IS NULL`, f.free); n != 1 {
		t.Error("the record nothing keeps still holds its content")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND body = 'What was said.'`,
		[]ids.UUID{f.held, f.receipt, f.live}); n != 3 {
		t.Errorf("%d of the three kept records still hold their content, want 3", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'erase' AND entity_type = 'activity'
		AND entity_id = $1 AND evidence->>'cause' = 'archive_review'`, f.free); n != 1 {
		t.Errorf("erase certificates for the deleted record = %d, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'erase' AND entity_id = ANY($1)`,
		[]ids.UUID{f.held, f.receipt, f.live}); n != 0 {
		t.Errorf("kept records carry %d erase certificates, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox e JOIN audit_log a ON a.id = (e.envelope->'trace'->>'audit_log_id')::uuid
		WHERE e.envelope->>'type' = 'retention.applied' AND e.envelope->'entity'->>'id' = $1::text
		  AND e.envelope->'payload'->>'reason' = 'archive review: the controller deleted the record''s content'
		  AND a.action = 'erase'`, f.free); n != 1 {
		t.Errorf("retention.applied events tied to the certificate = %d, want 1", n)
	}

	admin := httptest.NewRequest(http.MethodGet, "/v1/retention/archive", nil).
		WithContext(archiveReviewerCtx(e, principal.ObjectGrant{Read: true}))
	contact := openapi_types.UUID(f.contact)
	_, rows := listArchive(t, h, admin, crmcontracts.ListArchivedActivitiesParams{ContactId: &contact})
	listed := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, ids.UUID(row.ActivityId))
	}
	if slices.Contains(listed, f.free) {
		t.Error("the deleted record is still listed in the archive")
	}
}
