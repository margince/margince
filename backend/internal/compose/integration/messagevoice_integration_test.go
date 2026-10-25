// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// voiceFixture is one captured mail From P To the seat, with T also in To and
// Q in Cc, all four of them contacts. It goes through the real mail parser
// and capture sink.
type voiceFixture struct {
	e          *Env
	p, q, to   ids.UUID
	mail       ids.UUID
	pName      string
	readerSeat ids.UUID
}

func captureVoiceFixture(t *testing.T) voiceFixture {
	t.Helper()
	e := Setup(t)
	f := voiceFixture{e: e, pName: "Pia Sender", readerSeat: e.Rep1}
	f.p = e.SeedContact(t, f.pName, &e.Rep1)
	f.q = e.SeedContact(t, "Quentin Copied", &e.Rep1)
	f.to = e.SeedContact(t, "Tara Addressed", &e.Rep1)
	f.mail = captureCommitmentMessage(t, e, f.p, "voice-thread@buyer.example", "Signed NDA please",
		"Could you send us the signed NDA before Friday?", "inbound",
		time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		commitmentMailRecipient{contact: f.to, role: "to"},
		commitmentMailRecipient{contact: f.q, role: "cc"})
	// The owed-verdict pass reads the mail as a request, so it has a move to owe.
	judge := principal.WithActor(e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:owed_verdict",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	if applied, err := e.Activities.SetOwedVerdict(judge, f.mail, activities.OwedVerdictAsksUs, "voice-test", clocktest.Now(t)); err != nil || !applied {
		t.Fatalf("judging the mail a request: applied=%v err=%v", applied, err)
	}
	return f
}

func (f voiceFixture) row(t *testing.T, contact ids.UUID, reader ids.UUID) (crmcontracts.Contact360, crmcontracts.Activity) {
	t.Helper()
	view, err := contactRoomService(f.e).Assemble(f.e.As(reader, []ids.UUID{f.e.Team1}, AccountRepPerms), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	if view.Activities != nil {
		for _, a := range view.Activities.Data {
			if ids.UUID(a.Id) == f.mail {
				return view, a
			}
		}
	}
	t.Fatalf("the captured mail is not on the contact's page")
	return view, crmcontracts.Activity{}
}

func voiceRole(a crmcontracts.Activity) crmcontracts.ActivityVoiceContactRole {
	if a.Voice == nil || a.Voice.ContactRole == nil {
		return ""
	}
	return *a.Voice.ContactRole
}

func authorContact(a crmcontracts.Activity) *openapi_types.UUID {
	if a.Voice == nil || a.Voice.Author == nil {
		return nil
	}
	return a.Voice.Author.ContactId
}

// On the page of a contact who was only copied, the mail names its real
// author. It marks the contact as in Cc and owes nobody a reply.
func TestACopiedContactsPageNamesTheTrueAuthor(t *testing.T) {
	f := captureVoiceFixture(t)
	view, row := f.row(t, f.q, f.readerSeat)
	if got := authorContact(row); got == nil || ids.UUID(*got) != f.p {
		t.Fatalf("author = %+v, want the contact who wrote it", row.Voice)
	}
	if got := voiceRole(row); got != crmcontracts.ActivityVoiceContactRoleCc {
		t.Errorf("contact_role = %q, want cc", got)
	}
	if listed := listedRow(t, f, f.readerSeat); listed.EmailSummary == nil || listed.EmailSummary.Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("the fixture mail's own move is %+v, want needs_reply so the copied page has one to drop", listed.EmailSummary)
	}
	if row.EmailSummary == nil || row.EmailSummary.Move != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("move = %+v, want none: a copied contact is owed no reply", row.EmailSummary)
	}
	speaker := briefSpeakerOf(t, contactbrief.FromView(view), row.Id)
	if speaker == nil || speaker.Kind != "contact" || speaker.Name != f.pName {
		t.Errorf("brief speaker = %+v, want the contact %s, never them", speaker, f.pName)
	}
}

// The positive case: the sender's own page says they wrote it and keeps the move.
func TestTheSendersOwnPageSaysTheyWroteIt(t *testing.T) {
	f := captureVoiceFixture(t)
	view, row := f.row(t, f.p, f.readerSeat)
	if got := voiceRole(row); got != crmcontracts.ActivityVoiceContactRoleAuthor {
		t.Fatalf("contact_role = %q, want author", got)
	}
	if listed := listedRow(t, f, f.readerSeat); row.EmailSummary == nil || listed.EmailSummary == nil ||
		row.EmailSummary.Move != listed.EmailSummary.Move {
		t.Errorf("move = %+v, want the message's own move, unchanged on its author's page", row.EmailSummary)
	}
	if speaker := briefSpeakerOf(t, contactbrief.FromView(view), row.Id); speaker == nil || speaker.Kind != "them" {
		t.Errorf("brief speaker = %+v, want them", speaker)
	}
}

// A direct addressee is a party the move is owed to, and is not the author.
func TestAToRecipientIsAnAddressee(t *testing.T) {
	f := captureVoiceFixture(t)
	_, sender := f.row(t, f.p, f.readerSeat)
	_, row := f.row(t, f.to, f.readerSeat)
	if got := voiceRole(row); got != crmcontracts.ActivityVoiceContactRoleAddressee {
		t.Fatalf("contact_role = %q, want addressee", got)
	}
	if got := authorContact(row); got == nil || ids.UUID(*got) != f.p {
		t.Errorf("author = %+v, want the sender", row.Voice)
	}
	if row.EmailSummary == nil || sender.EmailSummary == nil || row.EmailSummary.Move != sender.EmailSummary.Move {
		t.Errorf("an addressee's move = %+v, want the sender's %+v", row.EmailSummary, sender.EmailSummary)
	}
}

// Mail the reader sent is "you"; the same mail read by another seat names the
// colleague who sent it.
func TestAColleaguesMailIsTheColleaguesNotTheReaders(t *testing.T) {
	e := Setup(t)
	p := e.SeedContact(t, "Pia Sender", &e.Rep1)
	mail := captureCommitmentMessage(t, e, p, "voice-out@buyer.example", "Your quote",
		"Here is the quote we discussed.", "outbound", time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC))
	f := voiceFixture{e: e, p: p, mail: mail}

	view, row := f.row(t, p, e.Rep1)
	if row.Voice == nil || row.Voice.Author == nil || !row.Voice.Author.IsReader {
		t.Fatalf("author = %+v, want the reader", row.Voice)
	}
	if speaker := briefSpeakerOf(t, contactbrief.FromView(view), row.Id); speaker == nil || speaker.Kind != "you" {
		t.Errorf("brief speaker = %+v, want you", speaker)
	}

	view, row = f.row(t, p, e.Rep2)
	if row.Voice == nil || row.Voice.Author == nil || row.Voice.Author.Kind != crmcontracts.ActivityAuthorKindColleague ||
		row.Voice.Author.IsReader {
		t.Fatalf("author for another seat = %+v, want the colleague who sent it", row.Voice)
	}
	if speaker := briefSpeakerOf(t, contactbrief.FromView(view), row.Id); speaker == nil || speaker.Kind != "colleague" {
		t.Errorf("brief speaker = %+v, want the colleague", speaker)
	}
}

// A note a colleague logged is the colleague's on the reader's brief, never
// the reader's own words.
func TestAColleaguesNoteSpeaksAsTheColleagueOnTheReadersBrief(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Xenia New", &e.Rep1)
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, AccountRepPerms)
	note, _, err := e.Activities.LogActivity(colleague, activities.LogActivityInput{
		Kind: "note", Subject: new("Met at the trade fair"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the colleague's note: %v", err)
	}
	f := voiceFixture{e: e, mail: ids.UUID(note.Id)}
	view, row := f.row(t, contact, e.Rep1)
	if row.Voice == nil || row.Voice.Author == nil || row.Voice.Author.UserId == nil ||
		ids.UUID(*row.Voice.Author.UserId) != e.Rep2 || row.Voice.Author.IsReader {
		t.Fatalf("author = %+v, want the colleague who logged it", row.Voice)
	}
	if speaker := briefSpeakerOf(t, contactbrief.FromView(view), row.Id); speaker == nil || speaker.Kind != "colleague" {
		t.Errorf("brief speaker = %+v, want the colleague, never you", speaker)
	}
	_, own := f.row(t, contact, e.Rep2)
	if own.Voice == nil || own.Voice.Author == nil || !own.Voice.Author.IsReader {
		t.Errorf("author read by the colleague = %+v, want themselves", own.Voice)
	}
}

// Every surface reads one projection: the activity list (which the company 360
// and the deal page read), the contact 360 and both brief inputs agree.
func TestEverySurfaceNamesTheSameAuthor(t *testing.T) {
	f := captureVoiceFixture(t)
	_, onPage := f.row(t, f.q, f.readerSeat)

	reader := f.e.As(f.readerSeat, []ids.UUID{f.e.Team1}, AccountRepPerms)
	entity, entityID := "contact", f.p
	var listed []crmcontracts.Activity
	if err := database.WithWorkspaceTx(reader, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		listed, _, err = activities.ListActivitiesTx(reader, tx, activities.ListActivitiesInput{EntityType: &entity, EntityID: &entityID})
		return err
	}); err != nil {
		t.Fatalf("listing the activities: %v", err)
	}
	var fromList *crmcontracts.Activity
	for i := range listed {
		if ids.UUID(listed[i].Id) == f.mail {
			fromList = &listed[i]
		}
	}
	if fromList == nil || fromList.Voice == nil || fromList.Voice.Author == nil {
		t.Fatalf("the list row carries no author: %+v", fromList)
	}
	if got, want := fromList.Voice.Author.ContactId, onPage.Voice.Author.ContactId; got == nil || want == nil || *got != *want {
		t.Errorf("the list names author %v, the contact page %v", got, want)
	}
	if fromList.Voice.ContactRole != nil {
		t.Errorf("a list row says contact_role %q, which only a contact page can say", *fromList.Voice.ContactRole)
	}

	company := companybrief.FromView(crmcontracts.Company360{
		Activities: &crmcontracts.ActivityListResponse{Data: listed},
	})
	for _, act := range company.Recent {
		if act.ID != f.mail.String() {
			continue
		}
		if act.Speaker == nil || act.Speaker.Kind != "contact" || act.Speaker.Name != f.pName {
			t.Errorf("company brief speaker = %+v, want the contact %s", act.Speaker, f.pName)
		}
		return
	}
	t.Fatal("the company brief input does not carry the mail")
}

// The newest row is P's mail with Q only in Cc, and Q's own older mail is on
// the page too. The deterministic brief names who wrote the newest message from
// that message, never from Q's older one.
func TestTheBriefFloorNeverCreditsACopiedMailToTheContact(t *testing.T) {
	f := captureVoiceFixture(t)
	captureCommitmentMessage(t, f.e, f.q, "voice-older@buyer.example", "Older note from Q",
		"Thanks for the call last month.", "inbound", time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	view, _ := f.row(t, f.q, f.readerSeat)
	in := contactbrief.FromView(view)
	if len(in.Recent) == 0 || in.Recent[0].ID != f.mail.String() {
		t.Fatalf("the newest row is not P's mail: %+v", in.Recent)
	}
	prose := claims.Prose(contactbrief.Deterministic(f.q.String(), in, "en"))
	if strings.Contains(prose, "They wrote last") || strings.Contains(prose, "You wrote last") {
		t.Errorf("the floor wrote %q; P's mail was told as Q's or the reader's", prose)
	}
}

// A reader who may read the mail but not contacts sees the party the header
// states, and nothing the contact record holds.
func TestAPartyIsNamedOnlyAsFarAsTheReaderMaySee(t *testing.T) {
	f := captureVoiceFixture(t)
	perms := AccountRepPerms
	objects := map[string]principal.ObjectGrant{}
	for name, grant := range perms.Objects {
		if name != objContact {
			objects[name] = grant
		}
	}
	perms.Objects = objects
	reader := f.e.As(f.readerSeat, []ids.UUID{f.e.Team1}, perms)
	var listed []crmcontracts.Activity
	if err := database.WithWorkspaceTx(reader, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		listed, _, err = activities.ListActivitiesTx(reader, tx, activities.ListActivitiesInput{})
		return err
	}); err != nil {
		t.Fatalf("listing the activities: %v", err)
	}
	for _, a := range listed {
		if ids.UUID(a.Id) != f.mail {
			continue
		}
		if a.Voice == nil || a.Voice.Author == nil {
			t.Fatalf("the mail carries no author: %+v", a.Voice)
		}
		author := a.Voice.Author
		if author.ContactId != nil || author.Kind == crmcontracts.ActivityAuthorKindContactRecord {
			t.Errorf("author = %+v, want the stated sender only", author)
		}
		if author.DisplayName != nil && *author.DisplayName == f.pName {
			t.Errorf("author name %q came from the contact record", *author.DisplayName)
		}
		if author.Address == nil || *author.Address == "" {
			t.Errorf("author = %+v, want the stated address", author)
		}
		return
	}
	t.Fatal("the mail is not in the list")
}

// listedRow is the captured mail as the plain activity list returns it.
func listedRow(t *testing.T, f voiceFixture, seat ids.UUID) crmcontracts.Activity {
	t.Helper()
	reader := f.e.As(seat, []ids.UUID{f.e.Team1}, AccountRepPerms)
	var listed []crmcontracts.Activity
	if err := database.WithWorkspaceTx(reader, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		listed, _, err = activities.ListActivitiesTx(reader, tx, activities.ListActivitiesInput{})
		return err
	}); err != nil {
		t.Fatalf("listing the activities: %v", err)
	}
	for _, a := range listed {
		if ids.UUID(a.Id) == f.mail {
			return a
		}
	}
	t.Fatal("the mail is not in the list")
	return crmcontracts.Activity{}
}

func briefSpeakerOf(t *testing.T, in contactbrief.Input, id openapi_types.UUID) *contactbrief.Speaker {
	t.Helper()
	for _, act := range in.Recent {
		if act.ID == id.String() {
			return act.Speaker
		}
	}
	t.Fatalf("the brief input does not carry activity %s", id)
	return nil
}

// A From row that names a contact the reader may not see stays an address. The
// address it states now belongs to a contact the reader does see, and that
// namesake is never shown as the author.
func TestAHiddenSenderIsNeverNamedAsAVisibleNamesake(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	hidden := e.SeedContact(t, "Ada Hidden", &e.Rep1)
	filedUnder := e.SeedContact(t, "Cleo Visible", &e.Rep1)
	address := commitmentMailAddress(t, e, hidden)
	logger := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	logged, _, err := e.Activities.LogActivity(logger, activities.LogActivityInput{
		Kind: "email", Subject: new("Contract question"), Body: new("Can we talk?"), Direction: new("inbound"),
		EmailFrom: address,
		Links: []activities.ActivityLinkInput{
			{EntityType: "contact", EntityID: hidden}, {EntityType: "contact", EntityID: filedUnder},
		},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	private := "owner"
	if _, err := e.Contacts.UpdateContact(admin, ids.From[ids.ContactKind](hidden), contacts.UpdateContactInput{
		Visibility: &private,
		Emails:     []contacts.ContactEmailInput{{Email: "ada.new@buyer.example", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("hiding the sender and moving their address: %v", err)
	}
	namesake := e.SeedContact(t, "Bea Namesake", &e.Rep2)
	if _, err := e.Contacts.UpdateContact(admin, ids.From[ids.ContactKind](namesake), contacts.UpdateContactInput{
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("giving the namesake the address: %v", err)
	}

	stranger := listedAuthorOn(t, e, e.Rep2, filedUnder, ids.UUID(logged.Id))
	if stranger != nil && (stranger.ContactId != nil || stranger.DisplayName != nil && *stranger.DisplayName == "Bea Namesake") {
		t.Errorf("a reader who cannot see the sender is shown author %+v, want the stated address only", stranger)
	}
	owner := listedAuthorOn(t, e, e.Rep1, filedUnder, ids.UUID(logged.Id))
	if owner == nil || owner.ContactId == nil || ids.UUID(*owner.ContactId) != hidden {
		t.Errorf("the sender's owner is shown author %+v, want the sender %v", owner, hidden)
	}
}

// listedAuthorOn is the author one row carries on a contact's activity list.
func listedAuthorOn(t *testing.T, e *Env, seat, contact, activityID ids.UUID) *crmcontracts.ActivityAuthor {
	t.Helper()
	reader := e.As(seat, []ids.UUID{e.Team1}, AccountRepPerms)
	entity := "contact"
	var listed []crmcontracts.Activity
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var err error
		listed, _, err = activities.ListActivitiesTx(reader, tx, activities.ListActivitiesInput{EntityType: &entity, EntityID: &contact})
		return err
	}); err != nil {
		t.Fatalf("listing the activities: %v", err)
	}
	for _, a := range listed {
		if ids.UUID(a.Id) == activityID {
			if a.Voice == nil {
				return nil
			}
			return a.Voice.Author
		}
	}
	t.Fatalf("seat %v does not see the mail on the contact's list", seat)
	return nil
}
