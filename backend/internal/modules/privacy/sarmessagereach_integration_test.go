// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// Which mails the Art. 15 export lists: every mail where the subject is From,
// To, Cc or Bcc, matched by their record or address. A mail filed only on
// their employer is not theirs.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type reachedMail struct {
	activity ids.UUID
	file     string
	subject  string
}

// seedMail writes one workspace-audience email with one attachment and one
// governed outbound row, and returns what the export should name.
func seedMail(t *testing.T, e *sarIdentifierEnv, audience, subject string) reachedMail {
	t.Helper()
	m := reachedMail{activity: ids.NewV7(), file: "file-" + subject + ".pdf", subject: subject}
	mustExec(e.ctx, t, e.owner, `INSERT INTO activity (id, kind, audience, subject, body, source, captured_by, occurred_at)
		VALUES ($1, 'email', $2, $3, 'body of '||$3, 'gmail', 'test:privacy', now())`, m.activity, audience, subject)
	mustExec(e.ctx, t, e.owner, `INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'activity', $2, $3, 'blob/'||$3, 'upload', 'test:privacy')`, ids.NewV7(), m.activity, m.file)
	return m
}

func seedOutbound(t *testing.T, e *sarIdentifierEnv, m reachedMail) {
	t.Helper()
	seat, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the export context carries no signed-in seat")
	}
	mustExec(e.ctx, t, e.owner, `INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id, subject, body, consent_purpose, recipients)
		VALUES ($1, $2, $4, 'gmail', $3, $3, 'outbound body', 'service', '[]'::jsonb)`,
		ids.NewV7(), m.activity, m.subject, seat.UserID)
}

func seedParticipant(t *testing.T, e *sarIdentifierEnv, m reachedMail, role string, contact *ids.ContactID, address *string) {
	t.Helper()
	mustExec(e.ctx, t, e.owner, `INSERT INTO activity_participant (activity_id, contact_id, role, address) VALUES ($1, $2, $3, $4)`,
		m.activity, contact, role, address)
}

func exportedIDs(rows []map[string]any, key string) map[string]bool {
	out := map[string]bool{}
	for _, row := range rows {
		out[strings.ToLower(fmt.Sprint(row[key]))] = true
	}
	return out
}

func has(set map[string]bool, id ids.UUID) bool { return set[strings.ToLower(id.String())] }

func TestTheExportListsEveryMailTheSubjectSentOrReceived(t *testing.T) {
	e := setupSARIdentifiers(t)
	address := ident(e.contact, liveEmail)
	cc := seedMail(t, e, "workspace", "cc-by-record")
	bcc := seedMail(t, e, "workspace", "bcc-by-record")
	to := seedMail(t, e, "workspace", "to-by-address")
	company := seedMail(t, e, "workspace", "company-only")
	seedParticipant(t, e, cc, "cc", &e.contact, nil)
	seedParticipant(t, e, bcc, "bcc", &e.contact, nil)
	seedParticipant(t, e, to, "to", nil, &address)
	companyID := ids.NewV7()
	mustExec(e.ctx, t, e.owner, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Employer', 'manual', 'test:privacy')`, companyID)
	mustExec(e.ctx, t, e.owner, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, company.activity, companyID)
	for _, m := range []reachedMail{cc, bcc, to, company} {
		seedOutbound(t, e, m)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	activities := exportedIDs(pkg.Activities, "id")
	attachments := exportedIDs(pkg.Attachments, "entity_id")
	outbound := exportedIDs(pkg.SentMessages, "subject")
	for _, m := range []reachedMail{cc, bcc, to} {
		if !has(activities, m.activity) {
			t.Errorf("activities omit %s, a mail the subject received", m.subject)
		}
		if !has(attachments, m.activity) {
			t.Errorf("attachments omit the file of %s", m.subject)
		}
		if !outbound[m.subject] {
			t.Errorf("sent messages omit %s", m.subject)
		}
	}
	if has(activities, company.activity) || has(attachments, company.activity) || outbound[company.subject] {
		t.Errorf("a mail filed only on the subject's employer is in the export")
	}
}

// seedLimitedDerivatives hangs one row of every other mail-reading section off
// a limited mail. Those rows are a governed send, a meeting proposal, an
// invitation, a staged approval quoting it and a reporting contribution.
func seedLimitedDerivatives(t *testing.T, e *sarIdentifierEnv, m reachedMail) {
	t.Helper()
	seedOutbound(t, e, m)
	host, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the export context carries no signed-in seat")
	}
	mustExec(e.ctx, t, e.owner, `INSERT INTO meeting_proposal (activity_id, host_user_id, request, link_ref, token_hash, expires_at)
		VALUES ($1, $2, '{"title":"secret-request"}', 'ref', $3, now() + interval '1 day')`, m.activity, host.UserID, strings.Repeat("a", 64))
	mustExec(e.ctx, t, e.owner, `INSERT INTO meeting_invitation (activity_id, host_user_id, provider, calendar_id, appointment, status, management_hash, management_ref)
		VALUES ($1, $2, 'gcal', 'cal', '{"title":"secret-appointment"}', 'pending', $3, 'ref')`, m.activity, host.UserID, strings.Repeat("b", 64))
	mustExec(e.ctx, t, e.owner, `INSERT INTO approval (kind, status, decided_at, proposed_by, summary, proposed_change, diff_hash, expires_at, evidence)
		VALUES ('update_contact', 'rejected', now(), 'test:privacy', 'staged', '{}', 'h', now() + interval '1 day',
		        jsonb_build_array(jsonb_build_object('source_type', 'activity', 'source_id', $1::text, 'evidence_snippet', 'secret-quote')))`, m.activity)
	execution, report, edition := ids.NewV7(), ids.NewV7(), ids.NewV7()
	mustExec(e.ctx, t, e.owner, `SET session_replication_role = replica`)
	mustExec(e.ctx, t, e.owner, `INSERT INTO report_edition (id, execution_id, report_id, report_revision, publication_audience, owner_id, captured_at, intended_due_at, manifest)
		VALUES ($1, $2, $3, 1, 'owner', $4, now(), now(), '{}')`, edition, execution, report, host.UserID)
	mustExec(e.ctx, t, e.owner, `INSERT INTO report_edition_contribution (edition_id, metric, context_id, contribution_key, source_type, source_id, fact)
		VALUES ($1, 'limited-metric', 'ctx', 'k', 'activity', $2, '{"row":{"value":"secret-value"}}')`, edition, m.activity)
	mustExec(e.ctx, t, e.owner, `SET session_replication_role = DEFAULT`)
}

func TestALimitedMailReachedThroughAParticipantRowIsListedWithoutItsContent(t *testing.T) {
	e := setupSARIdentifiers(t)
	m := seedMail(t, e, "participants", "private-cc")
	seedParticipant(t, e, m, "cc", &e.contact, nil)
	mustExec(e.ctx, t, e.owner, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, m.activity, e.contact)
	seedLimitedDerivatives(t, e, m)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	var found bool
	for _, row := range pkg.Activities {
		if !strings.EqualFold(fmt.Sprint(row["id"]), m.activity.String()) {
			continue
		}
		found = true
		if row["content_disclosed"] != false || row["subject"] != nil || row["body"] != nil {
			t.Errorf("a limited mail leaks its content through the export: %v", row)
		}
	}
	if !found {
		t.Fatal("a limited mail the subject was copied on is not listed")
	}
	for _, row := range pkg.Attachments {
		if strings.EqualFold(fmt.Sprint(row["entity_id"]), m.activity.String()) {
			if row["filename"] != nil {
				t.Errorf("the filename of a limited mail is exported: %v", row)
			}
			break
		}
	}
	assertWithheld(t, "sent message", pkg.SentMessages, "subject", "body", "recipients", "attachments")
	assertWithheld(t, "meeting proposal", pkg.MeetingProposals, "request")
	assertWithheld(t, "meeting invitation", pkg.MeetingInvitations, "appointment", "command")
	var reporting []map[string]any
	for _, row := range pkg.ReportingContributions {
		if row["metric"] == "limited-metric" {
			reporting = append(reporting, row)
		}
	}
	assertWithheld(t, "reporting contribution", reporting, "value")
	for _, row := range pkg.StagedMessages {
		if strings.Contains(fmt.Sprint(row["evidence"]), "secret-quote") {
			t.Errorf("a staged approval quotes a limited mail in the export: %v", row)
		}
	}
	if len(pkg.StagedMessages) == 0 {
		t.Error("the staged approval citing a limited mail is not listed")
	}
}

// assertWithheld wants one row, flagged as content withheld, with every named
// content column empty.
func assertWithheld(t *testing.T, what string, rows []map[string]any, columns ...string) {
	t.Helper()
	if len(rows) != 1 {
		t.Errorf("want one %s row for the limited mail, got %d", what, len(rows))
		return
	}
	if rows[0]["content_disclosed"] != false {
		t.Errorf("the %s row is not flagged as withheld: %v", what, rows[0])
	}
	for _, column := range columns {
		if rows[0][column] != nil {
			t.Errorf("the %s row exports %s of a limited mail: %v", what, column, rows[0][column])
		}
	}
}

// An address or chat account the subject retired, and another live contact now
// holds, is that contact's. Its mail is not in the subject's export. Mail
// resolved to the subject's own record still is.
func TestAMailOnAnIdentifierAnotherContactNowHoldsIsNotTheSubjects(t *testing.T) {
	e := setupSARIdentifiers(t)
	moved := ident(e.contact, retiredEmail)
	movedAccount := ident(e.contact, retiredAccount)
	rival := ids.New[ids.ContactKind]()
	mustExec(e.ctx, t, e.owner, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Rival', 'manual', 'test:privacy')`, rival)
	mustExec(e.ctx, t, e.owner, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, $2, 'manual', 'test:privacy')`, rival, moved)
	mustExec(e.ctx, t, e.owner, `INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
		VALUES ($1, 'telegram', $2, 'rival', 'connector:telegram', 'connector:telegram')`, rival, movedAccount)
	byAddress := seedMail(t, e, "workspace", "rival-address")
	seedParticipant(t, e, byAddress, "from", nil, &moved)
	byAccount := seedMail(t, e, "workspace", "rival-account")
	mustExec(e.ctx, t, e.owner, `UPDATE activity SET kind = 'message', channel_provider = 'telegram' WHERE id = $1`, byAccount.activity)
	mustExec(e.ctx, t, e.owner, `INSERT INTO activity_participant (activity_id, role, channel_user_id, address) VALUES ($1, 'from', $2, NULL)`, byAccount.activity, movedAccount)
	byRecord := seedMail(t, e, "workspace", "own-record")
	seedParticipant(t, e, byRecord, "to", &e.contact, &moved)

	tx, err := e.owner.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(e.ctx); err != nil {
			t.Errorf("rollback: %v", err)
		}
	}()
	emails, _, identities, err := subjectReach(e.ctx, tx, e.contact)
	if err != nil {
		t.Fatal(err)
	}
	all, err := subjectMessages(e.ctx, tx, e.contact, emails, identities)
	if err != nil {
		t.Fatal(err)
	}
	authored, err := subjectAuthoredMessages(e.ctx, tx, e.contact, emails, identities)
	if err != nil {
		t.Fatal(err)
	}
	for name, set := range map[string][]ids.UUID{"the export": all, "the judgements": authored} {
		got := exportedSet(set)
		if has(got, byAddress.activity) {
			t.Errorf("%s carries a mail on an address another contact now holds", name)
		}
		if has(got, byAccount.activity) {
			t.Errorf("%s carries a mail on a chat account another contact now holds", name)
		}
	}
	if !has(exportedSet(all), byRecord.activity) {
		t.Error("the export dropped a mail resolved to the subject's own record")
	}
}

func exportedSet(list []ids.UUID) map[string]bool {
	out := map[string]bool{}
	for _, id := range list {
		out[strings.ToLower(id.String())] = true
	}
	return out
}
