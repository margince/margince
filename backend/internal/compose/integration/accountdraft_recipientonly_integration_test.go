// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// An account draft to Jana reads only what Jana took part in. Ruben
// works at the same company and wrote to us about a disputed invoice, once with
// a signature and once without. A third mail of his is linked to Jana by hand
// and carries her as copy evidence the headers do not state. None of the three
// may reach the draft, and neither may a fourth that only copies us. Jana's own
// mail, ours to her, and a mail of his that copies her stay: a copy is a party.

// recipientOnlyFixture is one account with the recipient's own mail beside a
// colleague's.
type recipientOnlyFixture struct {
	company, jana                       ids.UUID
	janaWrote, weWroteJana              string
	rubenSigned, rubenUnsigned, rubenOn string
	rubenCopiedUs, janaCopied           string
	// rubenTask is due first and filed under Ruben; janaTask is filed under Jana.
	rubenTask, janaTask string
}

// The words of Ruben's mail. None of them may reach the model.
const (
	rubenSignedBody = "Hello, your September invoice says 9,600 EUR, but we were promised 8,400 EUR " +
		"for the depot licences. Please correct the invoice.\nRuben Hale, Finance, Harbourline Freight"
	rubenUnsignedBody = "Hello, the September invoice is still wrong and support took three days " +
		"to answer our last ticket. Please send a corrected invoice this week."
	rubenLinkedBody = "Following up on the invoice dispute: we will hold the payment until the amount is fixed."
	rubenCopyBody   = "Our auditors need the September credit note before the quarter closes."
)

func seedRecipientOnlyAccount(t *testing.T, e *Env) recipientOnlyFixture {
	t.Helper()
	admin := e.Admin()
	company := e.SeedCompany(t, "Harbourline Freight", &e.Rep1)
	jana := e.SeedContact(t, "Jana Novak", &e.Rep1)
	ruben := e.SeedContact(t, "Ruben Hale", &e.Rep1)
	companyID := companyIDOf(company)
	for _, employee := range []ids.UUID{jana, ruben} {
		contactID := ContactIDOf(employee)
		if _, err := e.Contacts.CreateRelationship(admin, contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
		}); err != nil {
			t.Fatalf("employing a contact at the account: %v", err)
		}
	}
	mail := func(from ids.UUID, direction, subject, body string, ago time.Duration) string {
		at := roomAgo(ago)
		logged, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
			Kind: "email", Direction: &direction, Subject: &subject, Body: &body, OccurredAt: &at,
			Links: []activities.ActivityLinkInput{
				{EntityType: "contact", EntityID: from},
				{EntityType: "company", EntityID: company},
			},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		return ids.UUID(logged.Id).String()
	}
	f := recipientOnlyFixture{company: company, jana: jana}
	f.weWroteJana = mail(jana, "outbound", "Pilot proposal for route planning",
		"Hi Jana, as promised, here is the pilot proposal for route planning at two of your depots.", 8*24*time.Hour)
	f.janaWrote = mail(jana, "inbound", "Re: Pilot proposal for route planning",
		"Thanks Anna, I will take the proposal to our managing director next week.", 6*24*time.Hour)
	f.rubenSigned = mail(ruben, "inbound", "Invoice September - wrong amount", rubenSignedBody, 3*24*time.Hour)
	f.rubenUnsigned = mail(ruben, "inbound", "Invoice September", rubenUnsignedBody, 2*24*time.Hour)
	f.rubenOn = mail(ruben, "inbound", "Invoice dispute", rubenLinkedBody, 24*time.Hour)

	// The third mail is filed under Jana too. It also carries her as copy
	// evidence, but its stated headers name only Ruben's address. A link and
	// an unstated copy row say the mail concerns her, not that she was on it.
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
		f.rubenOn, jana)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'ruben.hale@harbourline.example', 'from')`,
		f.rubenOn)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'cc')`,
		f.rubenOn, jana)

	// Ruben writes to someone else and copies us. Our side is on it, Jana is not.
	f.rubenCopiedUs = mail(ruben, "inbound", "Credit note for the auditors", rubenCopyBody, time.Hour)
	e.WsExec(t, `UPDATE activity_participant SET role = 'cc' WHERE activity_id = $1 AND user_id IS NOT NULL`,
		f.rubenCopiedUs)
	// A mail from Ruben that copies Jana is hers: she was on it.
	f.janaCopied = mail(ruben, "inbound", "Pilot kickoff venue",
		"The kickoff for the route planning pilot can be held at the Bremen depot.", 12*time.Hour)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'cc')`,
		f.janaCopied, jana)
	// Our newest mail on the account went to Ruben, not to Jana.
	mail(ruben, "outbound", "Your credit note", "Here is the credit note your auditors asked for.", 2*time.Hour)

	task := func(on ids.UUID, subject string, due time.Duration) string {
		at, dueAt := roomAgo(30*24*time.Hour), roomFixedNow.Add(due)
		logged, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
			Kind: "task", Subject: &subject, OccurredAt: &at, DueAt: &dueAt,
			Links: []activities.ActivityLinkInput{
				{EntityType: "contact", EntityID: on},
				{EntityType: "company", EntityID: company},
			},
		})
		if err != nil {
			t.Fatalf("logging task %q: %v", subject, err)
		}
		return ids.UUID(logged.Id).String()
	}
	f.rubenTask = task(ruben, "Send Ruben the corrected invoice", 24*time.Hour)
	f.janaTask = task(jana, "Send Jana the depot plan", 3*24*time.Hour)
	return f
}

func TestAnAccountDraftReadsOnlyTheRecipientsOwnConversation(t *testing.T) {
	e := Setup(t)
	f := seedRecipientOnlyAccount(t, e)
	svc := companySurfaceService(e)
	req := accountdraft.Request{ContactID: f.jana.String()}

	opts, err := accountdraft.ViewOptions(req)
	if err != nil {
		t.Fatalf("the draft's read options: %v", err)
	}
	view, err := svc.AssembleScoped(e.Admin(), companyIDOf(f.company), opts)
	if err != nil {
		t.Fatalf("assembling the draft's view: %v", err)
	}
	in, err := accountdraft.FromView(view, req)
	if err != nil {
		t.Fatalf("folding the draft's input: %v", err)
	}
	recent := recentAccountIDs(in.Recent)
	for name, id := range map[string]string{
		"Ruben's signed mail":              f.rubenSigned,
		"Ruben's unsigned mail":            f.rubenUnsigned,
		"Ruben's mail linked to Jana too":  f.rubenOn,
		"Ruben's mail that only copies us": f.rubenCopiedUs,
	} {
		if recent[id] {
			t.Errorf("%s reached the draft to Jana", name)
		}
	}
	if !recent[f.janaWrote] || !recent[f.weWroteJana] || !recent[f.janaCopied] {
		t.Errorf("Jana's own conversation is missing from her draft: %v", recent)
	}
	if in.Commitment == nil || in.Commitment.ID != f.janaTask {
		t.Errorf("the draft's commitment is %+v, want Jana's own task", in.Commitment)
	}
	if !sameInstant(view.LastInboundAt, new(roomAgo(12*time.Hour))) {
		t.Errorf("the draft's last inbound is %v, want the mail Jana was copied on", view.LastInboundAt)
	}
	if !sameInstant(view.LastOutboundAt, new(roomAgo(8*24*time.Hour))) {
		t.Errorf("the draft's last outbound is %v, want our proposal to Jana", view.LastOutboundAt)
	}
	payload, err := json.Marshal(in.Fenced())
	if err != nil {
		t.Fatalf("encoding the input: %v", err)
	}
	for _, word := range []string{"9,600", "8,400", "invoice", "Ruben", "auditors"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(word)) {
			t.Errorf("the draft's input carries %q from a colleague's mail:\n%s", word, payload)
		}
	}

	// The same account read without the recipient narrowing carries all four,
	// so their absence above is the narrowing's doing.
	wide, err := svc.AssembleScoped(e.Admin(), companyIDOf(f.company), company360.AssembleOptions{WrittenByHumanOnly: true})
	if err != nil {
		t.Fatalf("assembling the unnarrowed view: %v", err)
	}
	unnarrowed, err := accountdraft.FromView(wide, req)
	if err != nil {
		t.Fatalf("folding the unnarrowed view: %v", err)
	}
	all := recentAccountIDs(unnarrowed.Recent)
	if !all[f.rubenSigned] || !all[f.rubenUnsigned] || !all[f.rubenOn] || !all[f.rubenCopiedUs] {
		t.Fatalf("the unnarrowed account lost Ruben's mail, so its absence proves nothing: %v", all)
	}
	if unnarrowed.Commitment == nil || unnarrowed.Commitment.ID != f.rubenTask {
		t.Errorf("the unnarrowed commitment is %+v, want Ruben's task, so its absence proves nothing", unnarrowed.Commitment)
	}
	if !sameInstant(wide.LastInboundAt, new(roomAgo(time.Hour))) || !sameInstant(wide.LastOutboundAt, new(roomAgo(2*time.Hour))) {
		t.Errorf("the unnarrowed account's last touches are %v/%v, want Ruben's newer exchange",
			wide.LastInboundAt, wide.LastOutboundAt)
	}
}

// The service sends the model the narrowed view. A recording lane sees every
// request the draft makes, and none carries Ruben's words.
func TestTheModelNeverSeesAColleaguesMail(t *testing.T) {
	e := Setup(t)
	f := seedRecipientOnlyAccount(t, e)
	lane := &recordingDraftLane{}
	draft := accountdraft.NewService(companySurfaceService(e), lane)
	// The lane answers nothing usable. Whatever the draft does next, the
	// requests it sent are what this test reads.
	_, _, _ = draft.Draft(e.Admin(), companyIDOf(f.company), accountdraft.Request{ContactID: f.jana.String()})
	if len(lane.requests) == 0 {
		t.Fatal("the draft sent the model nothing, so this test proves nothing")
	}
	sawJana := false
	for _, req := range lane.requests {
		sent, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("encoding a model request: %v", err)
		}
		text := string(sent)
		for _, word := range colleagueWords {
			if strings.Contains(text, word) {
				t.Errorf("the model was sent %q from a colleague's mail", word)
			}
		}
		sawJana = sawJana || strings.Contains(text, "managing director")
	}
	if !sawJana {
		t.Error("the model was not sent Jana's own reply")
	}
}

// colleagueWords are phrases only Ruben's mail and task carry.
var colleagueWords = []string{"9,600", "8,400", "Ruben", "hold the payment", "support took three days", "auditors"}

// The contact page's drafter reads Jana's own timeline. A link files Ruben's
// mail there too, so the draft's read keeps only what she was a party to.
func TestAContactDraftLeavesOutAColleaguesMailLinkedToTheRecipient(t *testing.T) {
	e := Setup(t)
	f := seedRecipientOnlyAccount(t, e)
	svc := contactRoomService(e)
	jana := ContactIDOf(f.jana)

	page, err := svc.AssembleScoped(e.Admin(), jana, contact360.AssembleOptions{WrittenByHumanOnly: true})
	if err != nil {
		t.Fatalf("assembling Jana's page: %v", err)
	}
	if !contactActivityIDs(page)[f.rubenOn] {
		t.Fatal("Jana's page does not file Ruben's linked mail, so its absence below proves nothing")
	}
	narrowed, err := svc.AssembleScoped(e.Admin(), jana, contact360.AssembleOptions{WrittenByHumanOnly: true, PartyOnly: true})
	if err != nil {
		t.Fatalf("assembling Jana's draft view: %v", err)
	}
	kept := contactActivityIDs(narrowed)
	if kept[f.rubenOn] {
		t.Error("Ruben's mail linked to Jana reached her draft view")
	}
	if !kept[f.janaWrote] || !kept[f.janaCopied] {
		t.Errorf("Jana's own conversation is missing from her draft view: %v", kept)
	}

	lane := &recordingDraftLane{}
	_, _, _ = contactdraft.NewService(svc, lane).Draft(e.Admin(), jana, contactdraft.Request{})
	if len(lane.requests) == 0 {
		t.Fatal("the contact draft sent the model nothing, so this test proves nothing")
	}
	for _, req := range lane.requests {
		sent, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("encoding a model request: %v", err)
		}
		for _, word := range colleagueWords {
			if strings.Contains(string(sent), word) {
				t.Errorf("the contact draft sent the model %q from a colleague's mail", word)
			}
		}
	}
}

func contactActivityIDs(page crmcontracts.Contact360) map[string]bool {
	out := map[string]bool{}
	if page.Activities == nil {
		return out
	}
	for _, act := range page.Activities.Data {
		out[ids.UUID(act.Id).String()] = true
	}
	return out
}

// The narrowing runs before the cap. Six newer mails from a colleague would
// fill the draft's six slots if it ran after, and leave Jana's six out.
func TestTheRecipientNarrowingRunsBeforeTheCap(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	company := e.SeedCompany(t, "Harbourline Freight", &e.Rep1)
	jana := e.SeedContact(t, "Jana Novak", &e.Rep1)
	ruben := e.SeedContact(t, "Ruben Hale", &e.Rep1)
	janaID, companyID := ContactIDOf(jana), companyIDOf(company)
	if _, err := e.Contacts.CreateRelationship(admin, contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &janaID, CompanyID: &companyID,
	}); err != nil {
		t.Fatalf("employing Jana at the account: %v", err)
	}
	mail := func(from ids.UUID, subject string, ago time.Duration) string {
		at, direction, body := roomAgo(ago), "inbound", subject+" in full."
		logged, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
			Kind: "email", Direction: &direction, Subject: &subject, Body: &body, OccurredAt: &at,
			Links: []activities.ActivityLinkInput{
				{EntityType: "contact", EntityID: from},
				{EntityType: "company", EntityID: company},
			},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		return ids.UUID(logged.Id).String()
	}
	want := map[string]bool{}
	for i := range 6 {
		want[mail(jana, fmt.Sprintf("Pilot question %d", i), time.Duration(10+i)*24*time.Hour)] = true
		mail(ruben, fmt.Sprintf("Invoice question %d", i), time.Duration(1+i)*time.Hour)
	}
	req := accountdraft.Request{ContactID: jana.String()}
	opts, err := accountdraft.ViewOptions(req)
	if err != nil {
		t.Fatalf("the draft's read options: %v", err)
	}
	view, err := companySurfaceService(e).AssembleScoped(admin, companyIDOf(company), opts)
	if err != nil {
		t.Fatalf("assembling the draft's view: %v", err)
	}
	in, err := accountdraft.FromView(view, req)
	if err != nil {
		t.Fatalf("folding the draft's input: %v", err)
	}
	got := recentAccountIDs(in.Recent)
	if len(got) != len(want) {
		t.Fatalf("the draft read %d items, want Jana's %d", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Errorf("Jana's mail %s fell off the draft behind newer colleague mail", id)
		}
	}
}

// recordingDraftLane records each request and answers with an error.
type recordingDraftLane struct{ requests []model.Request }

func (l *recordingDraftLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.requests = append(l.requests, req)
	return model.Response{}, errors.New("recording lane: no answer")
}
