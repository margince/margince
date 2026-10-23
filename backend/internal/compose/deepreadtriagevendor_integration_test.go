// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A site that reads as a mail or hosting vendor, over a real Postgres, the
// production capture sink and a fake site. The verdict waits, and only the seat
// a current staff mail names as its sole direct recipient decides it.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	hostingDomain = "company-a.example"
	hostingStaff  = "contact-a@" + hostingDomain
	hostingReason = "the page sells managed hosting to businesses"
)

// staffMail is one inbound message as the mail mapper hands it to the sink. It
// carries the parsed To and Cc lines, the receiving mailbox and its own date.
type staffMail struct {
	sourceID, from string
	mailbox        ids.UUID
	to, cc         []string
	occurred       time.Time
}

// captureStaffMail lands the message through the production sink, after the
// workspace wrote to the sender, so the tier ladder creates the contact.
func captureStaffMail(t *testing.T, e *integration.Env, mail staffMail) {
	t.Helper()
	thread := "thread-" + mail.sourceID
	seedAttestedOutbound(t, e, "out-"+mail.sourceID+"-"+mail.mailbox.String(), mail.from, thread)
	var participants []connector.MessageParticipant
	for _, address := range mail.to {
		participants = append(participants, connector.MessageParticipant{Email: address, Role: connector.ParticipantRoleTo})
	}
	for _, address := range mail.cc {
		participants = append(participants, connector.MessageParticipant{Email: address, Role: connector.ParticipantRoleCC})
	}
	record := connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: mail.sourceID},
		Counterparty: connector.Counterparty{
			Email: mail.from, DisplayName: "Contact A",
			Domain: mail.from[strings.LastIndex(mail.from, "@")+1:], Direction: connector.DirectionInbound,
		},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "Hosting", Body: "Can we talk?", Direction: connector.DirectionInbound,
			OccurredAt: mail.occurred,
		},
		ThreadKey:    thread,
		Participants: participants,
		Addresses:    append(append([]string{mail.from}, mail.to...), mail.cc...),
		Source:       "gmail:" + mail.sourceID, CapturedBy: "connector:gmail",
	}
	if _, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(mailboxOwnerCtx(e, mail.mailbox), record.WithProviderAttestedParticipants(true)); err != nil {
		t.Fatalf("capturing %s: %v", mail.sourceID, err)
	}
}

// pinClock dates the fixtures and the store's currentness check from one
// instant, which the drift lane may move.
func pinClock(t *testing.T, e *integration.Env) time.Time {
	t.Helper()
	now := clocktest.Now(t)
	e.Contacts.WithClock(func() time.Time { return now })
	return now
}

// runVendorTriage reads a hosting company's front page and has the classifier
// call it a vendor. It returns the extraction fake, which a held verdict must
// never reach.
func runVendorTriage(t *testing.T, e *integration.Env, domain string) *triageBrainFake {
	t.Helper()
	read, _, err := e.Contacts.StartDomainTriageSiteRead(e.As(e.Rep1, nil, integration.AdminPerms), domain, systemDomainTriageActor, nil)
	if err != nil {
		t.Fatalf("starting the triage read: %v", err)
	}
	return runVendorTriageRead(t, e, SiteDeepReadArgs{Workspace: e.WS, SiteReadID: read.ID, RequestedBy: systemDomainTriageActor}, domain)
}

func runVendorTriageRead(t *testing.T, e *integration.Env, args SiteDeepReadArgs, domain string) *triageBrainFake {
	t.Helper()
	site := &fakeSite{pages: map[string]fakeSitePage{
		contacts.TriageSeedURL(domain): {text: readable("Managed hosting for your business.") + " Our team. Contact us."},
	}}
	extract := &triageBrainFake{verdict: `{}`}
	triage := &triageBrainFake{verdict: `{"kind":"provider","confidence":0.9,"reason":"` + hostingReason + `"}`}
	if err := newTriageTestWorker(e, site, extract, triage).run(e.As(e.Rep1, nil, integration.AdminPerms), args); err != nil {
		t.Fatalf("run: %v", err)
	}
	return extract
}

type vendorRow struct {
	status, source, reason string
	cursorCleared          bool
}

func vendorRowOf(t *testing.T, e *integration.Env, domain string) vendorRow {
	t.Helper()
	var row vendorRow
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status, COALESCE(source, ''), COALESCE(pending_reason, ''), next_attempt_at IS NULL
			FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&row.status, &row.source, &row.reason, &row.cursorCleared)
	}); err != nil {
		t.Fatal(err)
	}
	return row
}

// asksAbout reports whether the seat is asked. The membership check, the
// worklist read and the paged review list must agree.
func asksAbout(ctx context.Context, t *testing.T, e *integration.Env, domain string) bool {
	t.Helper()
	owns, err := e.Contacts.OwnsDomainQuestion(ctx, domain)
	if err != nil {
		t.Fatal(err)
	}
	open, err := e.Contacts.OpenDomainQuestionsForOwner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paged, _, err := e.Contacts.DomainQuestionsForOwner(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	listed, reviewed := false, false
	for _, q := range open {
		listed = listed || q.Domain == domain
	}
	for _, q := range paged {
		reviewed = reviewed || q.Domain == domain
	}
	if owns != listed || owns != reviewed {
		t.Fatalf("membership %v, worklist %v and review %v disagree about %s", owns, listed, reviewed, domain)
	}
	return owns
}

func onRefusedList(t *testing.T, e *integration.Env, domain string) bool {
	t.Helper()
	rows, _, err := e.Contacts.ListDomainAdmissions(e.As(e.Rep1, nil, integration.AdminPerms), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Domain == domain && row.Admission == contacts.DomainUndecided {
			return true
		}
	}
	return false
}

func answerDomain(ctx context.Context, e *integration.Env, verb, domain string) int {
	request := httptest.NewRequest(http.MethodPost, "/capture/domain-questions/"+domain+"/"+verb, nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handlers := domainQuestionHandlers{contacts: e.Contacts}
	if verb == "keep-vendor" {
		handlers.KeepDomainAsVendor(response, request, domain)
	} else {
		handlers.KeepDomainQuestion(response, request, domain)
	}
	return response.Code
}

func employmentEdges(t *testing.T, e *integration.Env, email, domain string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM relationship r JOIN contact_email m ON m.contact_id = r.contact_id
		JOIN company_domain d ON d.company_id = r.company_id WHERE m.email = $1 AND d.domain = $2 AND r.kind = 'employment'`, email, domain)
}

func TestOnlyTheSoleDirectRecipientOfCurrentStaffMailDecidesAVendorQuestion(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	rep1, rep2 := e.As(e.Rep1, nil, integration.AdminPerms), e.As(e.Rep2, nil, integration.AdminPerms)
	// Addressed to Rep1, Rep2 copied, and delivered to both mailboxes.
	mail := staffMail{
		sourceID: "vendor-1", from: hostingStaff, mailbox: e.Rep1,
		to: []string{seatEmail(t, e.Rep1)}, cc: []string{seatEmail(t, e.Rep2)}, occurred: now.Add(-time.Hour),
	}
	captureStaffMail(t, e, mail)
	mail.mailbox = e.Rep2
	captureStaffMail(t, e, mail)
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = 'vendor-1' AND direction = 'inbound'`); n != 1 {
		t.Fatalf("%d interactions for the reply, want 1", n)
	}
	extract := runVendorTriage(t, e, hostingDomain)

	if row := vendorRowOf(t, e, hostingDomain); row.status != contacts.DomainPending || row.reason != contacts.PendingVendorVerdict || !row.cursorCleared {
		t.Fatalf("disposition = %+v, want an open vendor question no sweep re-reads", row)
	}
	if _, _, companies := triageState(t, e, hostingDomain); companies != 0 || extract.calls != 0 {
		t.Fatalf("companies=%d extractions=%d, want the read held at the landing page", companies, extract.calls)
	}
	if !asksAbout(rep1, t, e, hostingDomain) {
		t.Fatal("the sole direct recipient was not asked")
	}
	questions, err := e.Contacts.OpenDomainQuestionsForOwner(rep1)
	if err != nil || len(questions) != 1 || !strings.Contains(questions[0].Note, hostingReason) {
		t.Fatalf("questions = %+v (%v), want the classifier's reason on the card", questions, err)
	}
	if asksAbout(rep2, t, e, hostingDomain) {
		t.Fatal("a copied seat whose mailbox also received the mail was asked")
	}
	for _, verb := range []string{"keep", "keep-vendor"} {
		if code := answerDomain(rep2, e, verb, hostingDomain); code != http.StatusNotFound {
			t.Errorf("copied seat %s = %d, want 404", verb, code)
		}
	}

	if code := answerDomain(rep1, e, "keep", hostingDomain); code != http.StatusOK {
		t.Fatalf("create company = %d, want 200", code)
	}
	if n := employmentEdges(t, e, hostingStaff, hostingDomain); n != 1 {
		t.Errorf("%d employment edges, want the staff member linked to the new company", n)
	}
	if row := vendorRowOf(t, e, hostingDomain); row.status != contacts.DomainCompany || row.reason != "" || asksAbout(rep1, t, e, hostingDomain) {
		t.Errorf("after create company the disposition is %+v, want company with no open question", row)
	}
}

func TestAVendorQuestionWithNoSoleOrCurrentRecipientStaysOnTheRecord(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	rep1, rep2 := e.As(e.Rep1, nil, integration.AdminPerms), e.As(e.Rep2, nil, integration.AdminPerms)
	for _, tc := range []struct {
		name, domain string
		to           []ids.UUID
		age          time.Duration
		rep1Asked    bool
	}{
		{"two addressed seats", "two-seats.example", []ids.UUID{e.Rep1, e.Rep2}, time.Hour, false},
		{"mail older than the horizon", "old-mail.example", []ids.UUID{e.Rep1}, 31 * 24 * time.Hour, false},
		{"mail inside the horizon", "recent-mail.example", []ids.UUID{e.Rep1}, 29 * 24 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var to []string
			for _, seat := range tc.to {
				to = append(to, seatEmail(t, seat))
			}
			captureStaffMail(t, e, staffMail{sourceID: tc.domain, from: "contact@" + tc.domain, mailbox: e.Rep1, to: to, occurred: now.Add(-tc.age)})
			runVendorTriage(t, e, tc.domain)
			if row := vendorRowOf(t, e, tc.domain); row.reason != contacts.PendingVendorVerdict {
				t.Fatalf("disposition = %+v, want the vendor fact kept", row)
			}
			if got := asksAbout(rep1, t, e, tc.domain); got != tc.rep1Asked {
				t.Errorf("Rep1 asked = %v, want %v", got, tc.rep1Asked)
			}
			if asksAbout(rep2, t, e, tc.domain) {
				t.Error("Rep2 was asked")
			}
			if !onRefusedList(t, e, tc.domain) {
				t.Error("the vendor fact is not reachable on the refused domains list")
			}
		})
	}
}

func TestKeepAsVendorSettlesTheVerdictAndLaterMailAsksNothing(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	rep1 := e.As(e.Rep1, nil, integration.AdminPerms)
	rep1Email := seatEmail(t, e.Rep1)
	captureStaffMail(t, e, staffMail{sourceID: "vendor-keep", from: hostingStaff, mailbox: e.Rep1, to: []string{rep1Email}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	audits := func() int {
		return e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN company_domain_disposition d ON d.id = a.entity_id WHERE d.domain = $1`, hostingDomain)
	}
	before := audits()

	if code := answerDomain(rep1, e, "keep-vendor", hostingDomain); code != http.StatusNoContent {
		t.Fatalf("keep as vendor = %d, want 204", code)
	}
	if row := vendorRowOf(t, e, hostingDomain); row.status != contacts.DomainProvider || row.source != contacts.DomainSourceHuman || row.reason != "" {
		t.Fatalf("disposition = %+v, want provider settled by a human", row)
	}
	if _, _, companies := triageState(t, e, hostingDomain); companies != 0 {
		t.Fatalf("%d companies after keep as vendor, want 0", companies)
	}
	if audits() != before+1 {
		t.Errorf("audit rows %d → %d, want one for the answer", before, audits())
	}
	captureStaffMail(t, e, staffMail{sourceID: "vendor-keep-2", from: "contact-b@" + hostingDomain, mailbox: e.Rep1, to: []string{rep1Email}, occurred: now})
	if asksAbout(rep1, t, e, hostingDomain) || stillDue(t, e, hostingDomain) {
		t.Fatal("new mail from a domain kept as a vendor asked again")
	}
	if code := answerDomain(rep1, e, "keep-vendor", hostingDomain); code != http.StatusNotFound {
		t.Errorf("a second answer = %d, want 404: nothing is open to answer", code)
	}
}

func TestOnlyTheDeciderWithTheCompanyGrantMayKeepAVendorVerdict(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	rep1 := e.As(e.Rep1, nil, integration.AdminPerms)
	captureStaffMail(t, e, staffMail{sourceID: "vendor-guard", from: hostingStaff, mailbox: e.Rep1, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	const unevidenced = "unreadable.example"
	openTriageQuestion(t, e, unevidenced, "hello@"+unevidenced, "Sender")
	if _, err := e.Contacts.ResolveUnreadableDomainTriage(rep1, contacts.ResolveDomainTriageInput{Domain: unevidenced}); err != nil {
		t.Fatal(err)
	}

	agent := principal.WithActor(rep1, principal.Principal{Type: principal.PrincipalAgent, ID: "agent:test"})
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		domain string
		want   int
	}{
		{"agent bearer", agent, hostingDomain, http.StatusForbidden},
		{"seat without company update, open domain", e.As(e.Rep1, nil, integration.ReadOnlyPerms), hostingDomain, http.StatusForbidden},
		{"seat without company update, unknown domain", e.As(e.Rep1, nil, integration.ReadOnlyPerms), "never-seen.example", http.StatusForbidden},
		{"colleague never asked", e.As(e.Rep2, nil, integration.AdminPerms), hostingDomain, http.StatusNotFound},
		{"question that is not a vendor question", rep1, unevidenced, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := answerDomain(tc.ctx, e, "keep-vendor", tc.domain); code != tc.want {
				t.Fatalf("keep as vendor = %d, want %d", code, tc.want)
			}
		})
	}
	if row := vendorRowOf(t, e, hostingDomain); row.reason != contacts.PendingVendorVerdict {
		t.Fatalf("a refused answer changed the question: %+v", row)
	}
}

func TestNewMailOnAnOpenVendorQuestionReadsNoSiteAgain(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	captureStaffMail(t, e, staffMail{sourceID: "vendor-new-1", from: hostingStaff, mailbox: e.Rep1, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	if !asksAbout(e.As(e.Rep1, nil, integration.AdminPerms), t, e, hostingDomain) {
		t.Fatal("precondition: the sole direct recipient is asked")
	}

	// Newer staff mail to a second seat. The site is not read again. The newer
	// mail controls, so the second seat decides and the first no longer does.
	captureStaffMail(t, e, staffMail{sourceID: "vendor-new-2", from: "contact-b@" + hostingDomain, mailbox: e.Rep2, to: []string{seatEmail(t, e.Rep2)}, occurred: now})
	if row := vendorRowOf(t, e, hostingDomain); row.reason != contacts.PendingVendorVerdict || !row.cursorCleared {
		t.Fatalf("new mail re-armed the vendor question: %+v", row)
	}
	if asksAbout(e.As(e.Rep1, nil, integration.AdminPerms), t, e, hostingDomain) {
		t.Error("the seat an older mail named is still asked")
	}
	if !asksAbout(e.As(e.Rep2, nil, integration.AdminPerms), t, e, hostingDomain) {
		t.Error("the sole direct recipient of the newest mail is not asked")
	}
}

// A colleague creates the company by hand while the vendor question is open.
// The next staff mail settles the question onto that company.
func TestAHandMadeCompanySettlesAnOpenVendorQuestionOnTheNextMail(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	rep1 := e.As(e.Rep1, nil, integration.AdminPerms)
	rep1Email := seatEmail(t, e.Rep1)
	captureStaffMail(t, e, staffMail{sourceID: "vendor-hand-1", from: hostingStaff, mailbox: e.Rep1, to: []string{rep1Email}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	if _, err := e.Contacts.CreateCompany(rep1, contacts.CreateCompanyInput{
		DisplayName: "Company A", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: hostingDomain, IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}

	const second = "contact-b@" + hostingDomain
	captureStaffMail(t, e, staffMail{sourceID: "vendor-hand-2", from: second, mailbox: e.Rep1, to: []string{rep1Email}, occurred: now})
	if row := vendorRowOf(t, e, hostingDomain); row.status != contacts.DomainCompany || row.reason != "" {
		t.Fatalf("disposition = %+v, want company with no open question", row)
	}
	if asksAbout(rep1, t, e, hostingDomain) {
		t.Error("the settled domain is still asked about")
	}
	if n := employmentEdges(t, e, second, hostingDomain); n != 1 {
		t.Errorf("%d employment edges for the new staff contact, want 1", n)
	}
}

func TestAnAdmittedDomainIsReadOnAndASuppressedOneAsksNobody(t *testing.T) {
	e := integration.Setup(t)
	rep1 := e.As(e.Rep1, nil, integration.AdminPerms)
	const admitted, suppressed = "admitted-host.example", "suppressed-host.example"

	args := openTriageQuestion(t, e, admitted, "contact@"+admitted, "Contact")
	if _, err := e.Contacts.SetDomainAdmission(rep1, admitted, contacts.DomainAdmitted, "a client of ours"); err != nil {
		t.Fatal(err)
	}
	if extract := runVendorTriageRead(t, e, args, admitted); extract.calls == 0 {
		t.Error("an admitted domain stopped at the vendor verdict; a colleague's yes outranks it")
	}
	if row := vendorRowOf(t, e, admitted); row.reason == contacts.PendingVendorVerdict {
		t.Errorf("an admitted domain was asked about again: %+v", row)
	}

	args = openTriageQuestion(t, e, suppressed, "contact@"+suppressed, "Contact")
	if _, err := e.Contacts.SetDomainAdmission(rep1, suppressed, contacts.DomainSuppressed, "a vendor we use"); err != nil {
		t.Fatal(err)
	}
	runVendorTriageRead(t, e, args, suppressed)
	if row := vendorRowOf(t, e, suppressed); row.reason == contacts.PendingVendorVerdict {
		t.Error("a suppressed domain raised a vendor question")
	}
	if _, _, companies := triageState(t, e, suppressed); companies != 0 {
		t.Errorf("%d companies on a suppressed domain, want 0", companies)
	}
}

// A notification address and a true mail vendor's customer raise no company
// question, through the same production sink.
func TestANotificationOrAConsumerMailboxRaisesNoVendorQuestion(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	to := []string{seatEmail(t, e.Rep1)}
	captureStaffMail(t, e, staffMail{sourceID: "vendor-notify", from: "noreply@notify.company-b.example", mailbox: e.Rep1, to: to, occurred: now})
	captureStaffMail(t, e, staffMail{sourceID: "vendor-consumer", from: "someone@live.fr", mailbox: e.Rep1, to: to, occurred: now})
	for _, domain := range []string{"company-b.example", "live.fr"} {
		if n := e.WsCount(t, `SELECT count(*) FROM company_domain_disposition WHERE domain = $1`, domain); n != 0 {
			t.Errorf("%s opened %d domain questions, want 0", domain, n)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_domain WHERE domain = 'live.fr'`); n != 0 {
		t.Errorf("%d companies on a consumer mail domain, want 0", n)
	}
	if _, found := contactVisibility(t, e, "someone@live.fr"); !found {
		t.Error("the consumer mailbox's sender got no contact; only the company is withheld")
	}
}

func TestStaffMailToTwoDifferentSeatsMakesNoDecider(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	const domain = "split-seats.example"
	captureStaffMail(t, e, staffMail{sourceID: "split-1", from: "a@" + domain, mailbox: e.Rep1, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	captureStaffMail(t, e, staffMail{sourceID: "split-2", from: "b@" + domain, mailbox: e.Rep2, to: []string{seatEmail(t, e.Rep2)}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, domain)
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		if asksAbout(e.As(seat, nil, integration.AdminPerms), t, e, domain) {
			t.Errorf("seat %s was asked although two seats are named", seat)
		}
	}
	if !onRefusedList(t, e, domain) {
		t.Error("the vendor fact is not reachable on the refused domains list")
	}
}

func TestTheProvenRecipientIsAskedWhenOnlyAColleaguesMailboxHoldsTheMail(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	captureStaffMail(t, e, staffMail{sourceID: "copy-only", from: hostingStaff, mailbox: e.Rep2, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	rep1 := e.As(e.Rep1, nil, integration.AdminPerms)
	if !asksAbout(rep1, t, e, hostingDomain) {
		t.Fatal("the sole direct recipient was not asked because another mailbox held the copy")
	}
	if asksAbout(e.As(e.Rep2, nil, integration.AdminPerms), t, e, hostingDomain) {
		t.Fatal("the mailbox that only held the copy was asked")
	}
	if code := answerDomain(rep1, e, "keep-vendor", hostingDomain); code != http.StatusNoContent {
		t.Fatalf("keep as vendor = %d, want 204", code)
	}
}

// The decider must be able to open the mail. A sole direct recipient always
// can, at any row scope, until a statutory hold withholds the mail.
func TestAMailTheDeciderMayNotOpenMakesNoQuestion(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	// Rep2's mailbox holds the mail, so capture files it under Rep2's private
	// contact. Rep1 is the sole direct recipient.
	captureStaffMail(t, e, staffMail{sourceID: "withheld", from: hostingStaff, mailbox: e.Rep2, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	ownScope := e.As(e.Rep1, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "company": {Read: true, Update: true}, "activity": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})
	if !asksAbout(ownScope, t, e, hostingDomain) {
		t.Fatal("the sole direct recipient at own row scope was not asked about mail addressed to them")
	}
	var mail ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_id = 'withheld'`).Scan(&mail)
	}); err != nil {
		t.Fatal(err)
	}
	integration.HoldUnderStatutoryFloor(t, e, mail)
	if asksAbout(ownScope, t, e, hostingDomain) {
		t.Fatal("a mail the reader may not open still made them the decider")
	}
	if code := answerDomain(ownScope, e, "keep-vendor", hostingDomain); code != http.StatusNotFound {
		t.Fatalf("keep as vendor on withheld evidence = %d, want 404", code)
	}
}

// vendorFactUnchanged asserts a refused answer wrote nothing.
func vendorFactUnchanged(t *testing.T, e *integration.Env, domain string, audits int) {
	t.Helper()
	if row := vendorRowOf(t, e, domain); row.status != contacts.DomainPending || row.reason != contacts.PendingVendorVerdict {
		t.Errorf("the vendor fact changed: %+v", row)
	}
	if _, _, companies := triageState(t, e, domain); companies != 0 {
		t.Errorf("%d companies, want 0", companies)
	}
	if got := domainAudits(t, e, domain); got != audits {
		t.Errorf("audit rows %d → %d, want none written", audits, got)
	}
}

func domainAudits(t *testing.T, e *integration.Env, domain string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN company_domain_disposition d ON d.id = a.entity_id WHERE d.domain = $1`, domain)
}

func TestASeatWithoutActivityReadGetsNoVendorQuestion(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	captureStaffMail(t, e, staffMail{sourceID: "no-activity-read", from: hostingStaff, mailbox: e.Rep1, to: []string{seatEmail(t, e.Rep1)}, occurred: now.Add(-time.Hour)})
	runVendorTriage(t, e, hostingDomain)
	grants := map[string]principal.ObjectGrant{
		"contact": {Read: true}, "company": {Create: true, Read: true, Update: true},
	}
	noActivity := e.As(e.Rep1, nil, principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll})
	var mail ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_id = 'no-activity-read'`).Scan(&mail)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.GetActivity(noActivity, ids.From[ids.ActivityKind](mail), storekit.LiveOnly); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("GetActivity without activity read = %v, want permission denied", err)
	}
	audits := domainAudits(t, e, hostingDomain)
	if asksAbout(noActivity, t, e, hostingDomain) {
		t.Fatal("a seat that may not open the mail was asked about it")
	}
	for _, verb := range []string{"keep", "keep-vendor"} {
		if code := answerDomain(noActivity, e, verb, hostingDomain); code != http.StatusNotFound {
			t.Errorf("%s without activity read = %d, want 404", verb, code)
		}
	}
	vendorFactUnchanged(t, e, hostingDomain, audits)

	grants["activity"] = principal.ObjectGrant{Read: true}
	if !asksAbout(e.As(e.Rep1, nil, principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll}), t, e, hostingDomain) {
		t.Fatal("with activity read restored the sole direct recipient is not asked")
	}
}

// The newest staff mail controls for every reader. When it names no single
// seat, or the reader may not open it, an older mail to one seat does not
// take its place.
func TestANewerUnclearMailIsNotOverruledByAnOlderOne(t *testing.T) {
	e := integration.Setup(t)
	now := pinClock(t, e)
	rep1, rep2 := seatEmail(t, e.Rep1), seatEmail(t, e.Rep2)
	for _, tc := range []struct {
		name, domain string
		to           []string
		hold         bool
	}{
		{"newer mail to two seats", "newer-two.example", []string{rep1, rep2}, false},
		{"newer mail to no seat", "newer-none.example", []string{"buyer@outside.example"}, false},
		{"newer mail withheld", "newer-held.example", []string{rep1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captureStaffMail(t, e, staffMail{sourceID: tc.domain + "-old", from: "old@" + tc.domain, mailbox: e.Rep1, to: []string{rep1}, occurred: now.Add(-48 * time.Hour)})
			runVendorTriage(t, e, tc.domain)
			asker := e.As(e.Rep1, nil, integration.AdminPerms)
			if !asksAbout(asker, t, e, tc.domain) {
				t.Fatal("precondition: the older mail's sole recipient is asked")
			}
			captureStaffMail(t, e, staffMail{sourceID: tc.domain + "-new", from: "new@" + tc.domain, mailbox: e.Rep1, to: tc.to, occurred: now.Add(-time.Hour)})
			if tc.hold {
				var mail ids.UUID
				if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
					return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_id = $1`, tc.domain+"-new").Scan(&mail)
				}); err != nil {
					t.Fatal(err)
				}
				integration.HoldUnderStatutoryFloor(t, e, mail)
			}
			audits := domainAudits(t, e, tc.domain)
			for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
				if asksAbout(e.As(seat, nil, integration.AdminPerms), t, e, tc.domain) {
					t.Errorf("seat %s is asked although the newest mail names no single readable seat", seat)
				}
			}
			for _, verb := range []string{"keep", "keep-vendor"} {
				if code := answerDomain(asker, e, verb, tc.domain); code != http.StatusNotFound {
					t.Errorf("%s by the older mail's recipient = %d, want 404", verb, code)
				}
			}
			vendorFactUnchanged(t, e, tc.domain, audits)
			if !onRefusedList(t, e, tc.domain) {
				t.Error("the vendor fact is not reachable on the refused domains list")
			}
		})
	}
}
