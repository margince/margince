// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
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
	f.rubenCopiedUs = mail(ruben, "inbound", "Credit note for the auditors", rubenCopyBody, 20*time.Hour)
	e.WsExec(t, `UPDATE activity_participant SET role = 'cc' WHERE activity_id = $1 AND user_id IS NOT NULL`,
		f.rubenCopiedUs)
	// A mail from Ruben that copies Jana is hers: she was on it.
	f.janaCopied = mail(ruben, "inbound", "Pilot kickoff venue",
		"The kickoff for the route planning pilot can be held at the Bremen depot.", 12*time.Hour)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'cc')`,
		f.janaCopied, jana)
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
		for _, word := range []string{"9,600", "8,400", "Ruben", "hold the payment", "support took three days", "auditors"} {
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

// recordingDraftLane records each request and answers with an error.
type recordingDraftLane struct{ requests []model.Request }

func (l *recordingDraftLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.requests = append(l.requests, req)
	return model.Response{}, errors.New("recording lane: no answer")
}
