// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An invitation mail's calendar file stays on the mail and off every Documents
// list: the contact's, the company's, the deal's and the deal room's. A PDF on
// the same mail is listed. Each mail is parsed and captured by the production
// writer, because what the lists filter on is what capture stamps.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const invitationICS = "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\n" +
	"UID:demo-1@customer.example\r\nSEQUENCE:1\r\nSUMMARY:Demo\r\nEND:VEVENT\r\nEND:VCALENDAR"

// googleInvitation is Google's shape: the calendar is an unnamed inline part
// of the multipart/alternative body. A PDF rides beside it.
func googleInvitation(from, to string, at time.Time) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to,
		"Subject: Updated invitation: Demo @ Thu 15:00 - 15:30 (CEST)",
		"Date: " + at.Format(time.RFC1123Z),
		"Message-ID: <invite-" + ids.NewV7().String() + "@customer.example>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="mixed"`, "",
		"--mixed",
		`Content-Type: multipart/alternative; boundary="alt"`, "",
		"--alt", "Content-Type: text/plain; charset=UTF-8", "", "You have been invited to Demo.",
		"--alt", "Content-Type: text/html; charset=UTF-8", "", "<p>You have been invited to Demo.</p>",
		"--alt", "Content-Type: text/calendar; charset=UTF-8; method=REQUEST", "", invitationICS,
		"--alt--",
		"--mixed",
		`Content-Type: application/pdf; name="agenda.pdf"`,
		`Content-Disposition: attachment; filename="agenda.pdf"`, "",
		"%PDF-1.4 agenda",
		"--mixed--", "",
	}, "\r\n"))
}

// namedInvitation carries the calendar as a named invite.ics attachment whose
// declared type is not text/calendar, so only its name gives it away.
func namedInvitation(from, to string, at time.Time) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to,
		"Subject: Invitation: Demo",
		"Date: " + at.Format(time.RFC1123Z),
		"Message-ID: <named-" + ids.NewV7().String() + "@customer.example>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="mixed"`, "",
		"--mixed", "Content-Type: text/plain; charset=UTF-8", "", "You have been invited to Demo.",
		"--mixed",
		`Content-Type: application/octet-stream; name="invite.ics"`,
		`Content-Disposition: attachment; filename="invite.ics"`, "",
		invitationICS,
		"--mixed",
		`Content-Type: application/pdf; name="agenda.pdf"`,
		`Content-Disposition: attachment; filename="agenda.pdf"`, "",
		"%PDF-1.4 agenda",
		"--mixed--", "",
	}, "\r\n"))
}

func TestAnInvitationsCalendarFileStaysOffEveryDocumentsList(t *testing.T) {
	for name, invitation := range map[string]func(from, to string, at time.Time) []byte{
		"an unnamed calendar part": googleInvitation,
		"a named invite.ics":       namedInvitation,
	} {
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			e := o.e
			contact := o.contact(t, "Anna Buyer", "anna@customer.example")
			company := e.SeedCompany(t, "Customer", &e.Rep1)
			e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
				VALUES ('employment', $1, $2, 'manual', 'human:test')`, contact, company)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := e.SeedDeal(t, "Customer rollout", pipeline, stage, &e.Rep1)

			mail := o.captureInvitation(t, invitation("anna@customer.example", o.seat, o.now.Add(-time.Hour)))
			e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, mail, deal)
			ctx := e.Admin()
			store := activities.NewStore(e.DB())

			onMail, _, err := store.ListAttachments(ctx, "activity", mail, nil, nil)
			if err != nil {
				t.Fatalf("the mail's files: %v", err)
			}
			files := map[string]ids.UUID{}
			for _, f := range onMail {
				files[f.Filename] = ids.UUID(f.Id)
			}
			calendar, pdf := files["invite.ics"], files["agenda.pdf"]
			if calendar == (ids.UUID{}) || pdf == (ids.UUID{}) {
				t.Fatalf("the mail itself lists %v, want invite.ics and agenda.pdf", files)
			}

			for list, listed := range documentLists(ctx, t, e, deal, contact, company) {
				if !listed[pdf] {
					t.Errorf("the %s Documents list lacks the PDF the mail carried", list)
				}
				if listed[calendar] {
					t.Errorf("the %s Documents list holds the invitation's calendar file", list)
				}
			}

			rooms := dealrooms.NewStore(e.DB())
			ctx = e.As(e.AdminUser, nil, roomSteward)
			room, err := rooms.CreateRoom(ctx, dealrooms.CreateRoomInput{
				DealID: ids.From[ids.DealKind](deal), Title: "Customer room", Source: "manual",
			})
			if err != nil {
				t.Fatalf("create the room: %v", err)
			}
			roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
			if _, err := rooms.AddDocument(ctx, roomID, dealrooms.AddDocumentInput{AttachmentID: pdf, GroupKey: "commercial", Source: "manual"}); err != nil {
				t.Errorf("the room refused the PDF: %v", err)
			}
			if _, err := rooms.AddDocument(ctx, roomID, dealrooms.AddDocumentInput{AttachmentID: calendar, GroupKey: "commercial", Source: "manual"}); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("adding the calendar file to the room answered %v, want not found", err)
			}
		})
	}
}

// roomSteward may open a room on the deal and put the deal's files in it.
var roomSteward = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"deal":      {Read: true, Update: true},
		"deal_room": {Create: true, Read: true, Update: true},
		"activity":  {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

func (o *owedEnv) captureInvitation(t *testing.T, raw []byte) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatalf("parsing the invitation: %v", err)
	}
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{Blob: blobstore.NewMemory()}).Upsert(
		threadConnectorCtx(o.e, o.owner), parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing the invitation: %v", err)
	}
	return ref.ID
}

// documentLists reads the deal's, the contact's and the company's Documents
// lists and returns the attachment ids each holds.
func documentLists(ctx context.Context, t *testing.T, e *integration.Env, deal, contact, company ids.UUID) map[string]map[ids.UUID]bool {
	t.Helper()
	store := activities.NewStore(e.DB())
	lists := map[string]map[ids.UUID]bool{"deal": {}, "contact": {}, "company": {}}
	dealDocs, _, err := store.ListDealDocuments(ctx, deal, activities.DealDocumentFilters{})
	if err != nil {
		t.Fatalf("deal documents: %v", err)
	}
	for _, d := range dealDocs {
		lists["deal"][ids.UUID(d.Attachment.Id)] = true
	}
	contactDocs, _, err := store.ListAttachments(ctx, "contact", contact, nil, nil)
	if err != nil {
		t.Fatalf("contact documents: %v", err)
	}
	for _, d := range contactDocs {
		lists["contact"][ids.UUID(d.Id)] = true
	}
	companyDocs, _, err := store.ListCompanyDocuments(ctx, company, activities.DocumentFilters{})
	if err != nil {
		t.Fatalf("company documents: %v", err)
	}
	for _, d := range companyDocs {
		lists["company"][ids.UUID(d.Id)] = true
	}
	return lists
}
