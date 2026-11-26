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
	var companyID ids.UUID = ids.NewV7()
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

func TestALimitedMailReachedThroughAParticipantRowIsListedWithoutItsContent(t *testing.T) {
	e := setupSARIdentifiers(t)
	m := seedMail(t, e, "participants", "private-cc")
	seedParticipant(t, e, m, "cc", &e.contact, nil)

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
			return
		}
	}
	t.Error("the attachment of a limited mail is not listed")
}

func TestASharedAddressPutsTheMailInTheDataButNotInTheJudgementSelector(t *testing.T) {
	e := setupSARIdentifiers(t)
	shared := ident(e.contact, retiredEmail)
	rival := ids.New[ids.ContactKind]()
	mustExec(e.ctx, t, e.owner, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Rival', 'manual', 'test:privacy')`, rival)
	mustExec(e.ctx, t, e.owner, `INSERT INTO contact_email (contact_id, email, source, captured_by) VALUES ($1, $2, 'manual', 'test:privacy')`, rival, shared)
	m := seedMail(t, e, "workspace", "shared-address")
	seedParticipant(t, e, m, "from", nil, &shared)

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
	if !has(exportedSet(all), m.activity) {
		t.Error("the data sections omit a mail on an address a second contact also holds")
	}
	if has(exportedSet(authored), m.activity) {
		t.Error("the judgement selector attributes a mail on a shared address to one subject")
	}
}

func exportedSet(list []ids.UUID) map[string]bool {
	out := map[string]bool{}
	for _, id := range list {
		out[strings.ToLower(id.String())] = true
	}
	return out
}
