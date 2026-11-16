// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A pack whose data-protection law follows the recipient, against an
// installation established under another one. The test packs stand in for
// the Vietnamese pack (extensions/vn) and an EU one: core holds no country.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	pubjurisdiction "github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// packPair is a recipient-bound pack and an installation pack that is not.
type packPair struct {
	recipient, installation jurisdiction.Code
}

// registerPackPair registers both packs. The recipient's writes its pages in
// Vietnamese, has no legitimate-interest basis and sets the Decree 356/2025
// deadlines; the installation's writes in German and sets none.
// Registered once per process: the registries refuse a second set for a code,
// and the package has a fixed number of test codes to share.
var (
	packPairOnce sync.Once
	sharedPair   packPair
)

func registerPackPair(t *testing.T) packPair {
	t.Helper()
	packPairOnce.Do(func() { sharedPair = registerPackPairOnce(t) })
	return sharedPair
}

func registerPackPairOnce(t *testing.T) packPair {
	t.Helper()
	p := packPair{recipient: testJurisdiction(t), installation: testJurisdiction(t)}
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: p.recipient, Version: 1,
		Disclosures:                    []messaging.Disclosure{{Kind: messaging.ControllerIdentity}},
		DataProtectionFollowsRecipient: true, DisclosureLanguage: string(mailcopy.Vietnamese),
		NoLegitimateInterests: true, OptOutAcknowledgement: true,
	})
	messagingrules.Register(messagingrules.Rules{Jurisdiction: p.installation, Version: 1})
	subjectrightsrules.Register(subjectrightsrules.Rules{
		Jurisdiction: p.recipient, Version: 1,
		Instruments: []messaging.Instrument{{Name: "Test Decree 1/2026"}},
		Deadlines: map[subjectrightsrules.RequestKind]subjectrightsrules.Deadline{
			"rectify":     {Period: pubjurisdiction.Period{Days: 10}},
			"erasure":     {Period: pubjurisdiction.Period{Days: 20}},
			"restriction": {Period: pubjurisdiction.Period{Days: 15}},
		},
		ReplyWithinWorkingDays: 2,
	})
	return p
}

// underInstallation wires the installation's country and its German mail.
func underInstallation(s *Store, p packPair) *Store {
	return s.
		WithInstallationCountry(InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			return p.installation, nil
		})).
		WithMailLanguage(MailLanguageFunc(func(context.Context, pgx.Tx) (string, error) {
			return string(mailcopy.German), nil
		}))
}

// stateTheController gives the installation the particulars a disclosure
// under the recipient-bound pack needs before a controller mail may go.
func stateTheController(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	stateParticulars(t, e, ControllerParticulars{
		LegalName: "Công ty Ví Dụ", PostalAddress: "1 Phố Huế, Hà Nội", PrivacyContact: "privacy@example.test",
	})
}

func placeContact(t *testing.T, owner *pgx.Conn, contact ids.ContactID, code jurisdiction.Code) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`UPDATE contact SET address_country = upper($2) WHERE id = $1`, contact, string(code)); err != nil {
		t.Fatalf("placing the contact: %v", err)
	}
}

// TestTheRecipientsPackDecidesTheControllerMailAndThePages: an installation
// under one law writes to a contact whose country's law follows them. The
// record confirmation, the privacy notice and the privacy page are then in
// that pack's language and owe its disclosures. A contact in the
// installation's own country keeps the installation's.
func TestTheRecipientsPackDecidesTheControllerMailAndThePages(t *testing.T) {
	p := registerPackPair(t)
	for _, c := range []struct {
		name     string
		country  func(packPair) jurisdiction.Code
		language string
		owesID   bool
	}{
		{"a contact under the recipient-bound pack", func(p packPair) jurisdiction.Code { return p.recipient }, "vi", true},
		{"a contact under the installation's pack", func(p packPair) jurisdiction.Code { return p.installation }, "de", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setupChannelConsent(t)
			stateTheController(t, e)
			seedSubjectAddress(t, e)
			placeContact(t, e.owner, e.contact, c.country(p))
			stager, vault := &recordingStager{}, &recordingVault{}
			e.store = underInstallation(e.store, p).WithConfirmationLane(stager, vault, "https://crm.example.test/")

			record, err := e.store.IssueConfirmToken(e.ctx, e.contact, "")
			if err != nil {
				t.Fatalf("minting the record confirmation: %v", err)
			}
			if got := stager.seen.Rendered.Language; got != c.language {
				t.Errorf("the record confirmation is written in %q, want %q", got, c.language)
			}
			if got := getConfirmPage(t, e, record.Token)["language"]; got != c.language {
				t.Errorf("the record page opens in %v, want %q", got, c.language)
			}

			notice, err := e.store.IssuePrivacyNotice(e.ctx, e.contact, "")
			if err != nil {
				t.Fatalf("minting the privacy notice: %v", err)
			}
			if got := stager.seen.Rendered.Language; got != c.language {
				t.Errorf("the privacy notice is written in %q, want %q", got, c.language)
			}
			page := getConfirmPage(t, e, notice.Token)
			if got := page["language"]; got != c.language {
				t.Errorf("the privacy page opens in %v, want %q", got, c.language)
			}
			bases, _ := page["purpose_bases"].([]any)
			if len(bases) == 0 {
				t.Errorf("the privacy page names no purpose: %v", page["purpose_bases"])
			}
			for _, basis := range bases {
				if c.owesID && basis == string(commsauthz.BasisLegitimateInterests) {
					t.Errorf("the privacy page names legitimate interests under a pack that has no such basis")
				}
			}
			if c.owesID && (len(bases) != 1 || bases[0] != noBasisRecorded) {
				t.Errorf("with nothing on record the privacy page names %v, want no basis recorded", bases)
			}

			lines, err := e.store.Disclosures(e.ctx, commsauthz.CategoryReplyToInbound,
				[]string{"subject-" + e.contact.String() + "@example.test"}, ids.UUID{})
			if err != nil {
				t.Fatalf("asking what the mail must disclose: %v", err)
			}
			owed := false
			for _, line := range lines {
				owed = owed || line.Kind == string(messaging.ControllerIdentity)
			}
			if owed != c.owesID {
				t.Errorf("the mail owes the controller identity = %v, want %v", owed, c.owesID)
			}
		})
	}
}

// TestTheRecipientsPackSetsTheRequestDeadline: the deadline a record-page
// request is opened with comes from the recipient-bound pack, and the
// installation's month otherwise.
func TestTheRecipientsPackSetsTheRequestDeadline(t *testing.T) {
	p := registerPackPair(t)
	received := time.Date(2026, time.November, 13, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name    string
		country func(packPair) jurisdiction.Code
		want    time.Time
	}{
		{"recipient-bound", func(p packPair) jurisdiction.Code { return p.recipient }, received.AddDate(0, 0, 10)},
		{"installation", func(p packPair) jurisdiction.Code { return p.installation }, received.AddDate(0, 1, 0)},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setupChannelConsent(t)
			placeContact(t, e.owner, e.contact, c.country(p))
			e.store = underInstallation(e.store, p)
			var due time.Time
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				var err error
				due, _, err = e.store.requestDeadlineTx(e.ctx, tx,
					ruleSubject{Kind: entityContact, ID: e.contact.String()}, "rectify", received)
				return err
			}); err != nil {
				t.Fatalf("resolving the deadline: %v", err)
			}
			if !due.Equal(c.want) {
				t.Errorf("a correction is due %s, want %s", due, c.want)
			}
		})
	}
}

// recordingTasks keeps the request tasks a store asked to open.
type recordingTasks struct{ opened []RequestTask }

func (r *recordingTasks) OpenRequestTaskTx(_ context.Context, _ pgx.Tx, task RequestTask) error {
	r.opened = append(r.opened, task)
	return nil
}

// TestARecordPageRequestUnderTheRecipientsPackOpensDatedTasks: a correction
// is due 10 days after it arrives and an erasure 20, each with its first
// answer owed in 2 working days.
func TestARecordPageRequestUnderTheRecipientsPackOpensDatedTasks(t *testing.T) {
	p := registerPackPair(t)
	e := setupChannelConsent(t)
	stateTheController(t, e)
	placeContact(t, e.owner, e.contact, p.recipient)
	tasks := &recordingTasks{}
	e.store = underInstallation(e.store, p).WithRequestTasks(tasks)
	link := issueLink(t, e)

	before := time.Now()
	if _, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections:    map[string]string{ConfirmFieldFullName: "Corrected Name"},
		RequestErasure: true,
	}); err != nil {
		t.Fatalf("submitting the requests: %v", err)
	}

	due := map[string]time.Duration{}
	for _, task := range tasks.opened {
		due[task.Right] = task.DueAt.Sub(before)
		if task.ReplyBy.IsZero() || !task.ReplyBy.Before(task.DueAt) {
			t.Errorf("%s owes its first answer by %s, want a date before %s", task.Right, task.ReplyBy, task.DueAt)
		}
	}
	for right, days := range map[string]int{RequestTaskRectify: 10, RequestTaskErasure: 20} {
		want := time.Duration(days) * 24 * time.Hour
		if got, ok := due[right]; !ok || got < want || got > want+time.Minute {
			t.Errorf("%s task due %s after the request, want %d days (tasks %+v)", right, got, days, tasks.opened)
		}
	}
}

// TestAnObjectionUnderTheRecipientsPackIsReceiptedAndOpensATask: the
// objection gets its receipt at once and a task due 15 days later. A contact
// under the installation's pack gets no such task.
func TestAnObjectionUnderTheRecipientsPackIsReceiptedAndOpensATask(t *testing.T) {
	p := registerPackPair(t)
	for _, c := range []struct {
		name    string
		country func(packPair) jurisdiction.Code
		tasks   int
	}{
		{"recipient-bound", func(p packPair) jurisdiction.Code { return p.recipient }, 1},
		{"installation", func(p packPair) jurisdiction.Code { return p.installation }, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setupChannelConsent(t)
			stateTheController(t, e)
			seedSubjectAddress(t, e)
			placeContact(t, e.owner, e.contact, c.country(p))
			tasks, stager := &recordingTasks{}, &recordingStager{}
			e.store = underInstallation(e.store, p).WithRequestTasks(tasks).
				WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test/")
			before := time.Now()
			if err := e.store.Suppress(e.ctx, SuppressInput{
				ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "Stop using my data.",
			}); err != nil {
				t.Fatalf("recording the objection: %v", err)
			}
			if len(tasks.opened) != c.tasks {
				t.Fatalf("the objection opened %d tasks, want %d", len(tasks.opened), c.tasks)
			}
			if c.tasks == 0 {
				return
			}
			task := tasks.opened[0]
			if task.Right != RequestTaskObjection {
				t.Errorf("the task is about %q, want an objection", task.Right)
			}
			if task.ReplyBy.IsZero() || !task.ReplyBy.Before(task.DueAt) {
				t.Errorf("the task is due %s with the reply by %s, want a reply before it", task.DueAt, task.ReplyBy)
			}
			want := 15 * 24 * time.Hour
			if got := task.DueAt.Sub(before); got < want-time.Minute || got > want+time.Minute {
				t.Errorf("the task is due %s after the objection, want 15 days", got)
			}
			if stager.calls != 1 || stager.seen.Rendered.Key != TemplateOptOutAcknowledgement {
				t.Errorf("the objection staged %d mails under %q, want the receipt at once",
					stager.calls, stager.seen.Rendered.Key)
			}
		})
	}
}

// TestNoDecisionUnderAPackWithoutLegitimateInterestsRestsOnThem: under such a
// pack, a message the GDPR model sends on legitimate interests rests on the
// recipient's consent, on an agreement, or on nothing. With nothing, the
// preview marks the recipient for the warning.
func TestNoDecisionUnderAPackWithoutLegitimateInterestsRestsOnThem(t *testing.T) {
	p := registerPackPair(t)
	e := setupResolve(t)
	placeContact(t, e.owner, e.contact, p.recipient)
	e.capturedMail(t, "thread-matter", "cc", true)
	request := commsauthz.Request{ThreadKey: "thread-matter"}

	d := e.decide(t, request)
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("decision = %+v, want the copied recipient allowed", d)
	}
	if d.Basis == commsauthz.BasisLegitimateInterests {
		t.Fatalf("the decision rests on legitimate interests under a pack that has none")
	}
	set, err := e.gate.Preview(e.ctx, commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}}, ThreadKey: "thread-matter",
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if len(set.Decisions) != 1 || !set.Decisions[0].LawfulBasisMissing {
		t.Fatalf("preview %+v, want the recipient marked: no consent and no agreement on record", set.Decisions)
	}

	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (key, label) VALUES ('rp_news', 'News')
		ON CONFLICT (key) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact_consent (contact_id, purpose_id, state, lawful_basis, captured_at, source)
		SELECT $1, id, 'granted', 'consent', now(), 'test' FROM consent_purpose WHERE key = 'rp_news'`,
		e.contact); err != nil {
		t.Fatalf("recording the consent: %v", err)
	}
	if d := e.decide(t, request); d.Basis != commsauthz.BasisConsent {
		t.Errorf("with consent on record the decision rests on %q, want consent", d.Basis)
	}
}

// TestSendingDespiteTheWarningIsAuditedUnderTheConfirmingUser: confirming
// the warning writes one audit row naming the user, and only for a
// recipient the warning applies to.
func TestSendingDespiteTheWarningIsAuditedUnderTheConfirmingUser(t *testing.T) {
	p := registerPackPair(t)
	e := setupResolve(t)
	placeContact(t, e.owner, e.contact, p.recipient)

	confirmed, err := e.store.ConfirmSendWithoutLawfulBasis(e.ctx, []string{e.address})
	if err != nil {
		t.Fatalf("confirming the warning: %v", err)
	}
	if len(confirmed) != 1 {
		t.Fatalf("confirmed %v, want the one address the warning applies to", confirmed)
	}
	var actor string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT actor_id FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'approve'
		   AND after->>'warning' = $2`, e.contact, lawfulBasisWarning).Scan(&actor); err != nil {
		t.Fatalf("reading the confirmation's audit row: %v", err)
	}
	if want := "human:" + e.user.String(); actor != want {
		t.Errorf("the confirmation is audited under %q, want the confirming user %q", actor, want)
	}

	placeContact(t, e.owner, e.contact, p.installation)
	if confirmed, err := e.store.ConfirmSendWithoutLawfulBasis(e.ctx, []string{e.address}); err != nil || len(confirmed) != 0 {
		t.Errorf("confirming outside the pack = %v, %v; want nothing to confirm", confirmed, err)
	}

	agent := principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x"})
	if _, err := e.store.ConfirmSendWithoutLawfulBasis(agent, []string{e.address}); err == nil {
		t.Error("an agent confirmed the warning; only the sender may")
	}
}

// TestTheOptOutReceiptIsWrittenInTheRecipientsPackLanguage: a contact whose
// last link mail was in English gets the receipt in the language of the pack
// that follows them.
func TestTheOptOutReceiptIsWrittenInTheRecipientsPackLanguage(t *testing.T) {
	p := registerPackPair(t)
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stateTheController(t, e)
	stager := &recordingStager{}
	e.store = underInstallation(e.store, p).
		WithMailLanguage(MailLanguageFunc(func(context.Context, pgx.Tx) (string, error) {
			return string(mailcopy.English), nil
		})).
		WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test/")
	if _, err := e.store.IssueConfirmToken(e.ctx, e.contact, ""); err != nil {
		t.Fatalf("minting the English link mail: %v", err)
	}
	if got := stager.seen.Rendered.Language; got != string(mailcopy.English) {
		t.Fatalf("the last link mail is in %q, want English for this test to mean anything", got)
	}
	placeContact(t, e.owner, e.contact, p.recipient)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "No more ads.",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}
	receipt := stager.seen.Rendered
	words := mailcopy.For(string(mailcopy.Vietnamese))
	if receipt.Key != TemplateOptOutAcknowledgement || receipt.Language != string(mailcopy.Vietnamese) {
		t.Fatalf("the receipt is %q in %q, want the opt-out receipt in Vietnamese", receipt.Key, receipt.Language)
	}
	if receipt.Subject != words.OptOutAckSubject {
		t.Errorf("the receipt subject is %q, want %q", receipt.Subject, words.OptOutAckSubject)
	}
	if !strings.Contains(receipt.Body, words.OptOutAckIgnore) {
		t.Errorf("the receipt body is not the Vietnamese wording:\n%s", receipt.Body)
	}
}

// TestARequestTheOptOutReceiptDoesNotCoverIsAnsweredAtOnce: under a pack that
// follows the recipient, a restriction and a non-marketing withdrawal get the
// request receipt at once. Each also gets a task due 15 days later.
func TestARequestTheOptOutReceiptDoesNotCoverIsAnsweredAtOnce(t *testing.T) {
	p := registerPackPair(t)
	for _, c := range []struct {
		name    string
		request func(t *testing.T, e *channelConsentEnv) error
		right   string
	}{
		{"a restriction", func(t *testing.T, e *channelConsentEnv) error {
			return e.store.Suppress(e.ctx, SuppressInput{ContactID: e.contact, Kind: kindProcessingRestriction, Reason: "Disputed data."})
		}, RequestTaskRestriction},
		{"a withdrawal of a service purpose", func(t *testing.T, e *channelConsentEnv) error {
			var purpose ids.PurposeID
			if err := e.owner.QueryRow(context.Background(), `
				INSERT INTO consent_purpose (key, label, class) VALUES ('rp_service_'||$1, 'Service updates', 'transactional')
				RETURNING id`, e.contact.String()[:8]).Scan(&purpose); err != nil {
				t.Fatal(err)
			}
			if _, err := e.store.Record(e.ctx, RecordInput{
				ContactID: e.contact, PurposeID: purpose, NewState: "granted", PolicyText: &grantWording,
			}); err != nil {
				return err
			}
			_, err := e.store.Record(e.ctx, RecordInput{ContactID: e.contact, PurposeID: purpose, NewState: "withdrawn"})
			return err
		}, RequestTaskWithdrawal},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setupChannelConsent(t)
			seedSubjectAddress(t, e)
			stateTheController(t, e)
			placeContact(t, e.owner, e.contact, p.recipient)
			tasks, stager := &recordingTasks{}, &recordingStager{}
			e.store = underInstallation(e.store, p).WithRequestTasks(tasks).
				WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test/")

			if err := c.request(t, e); err != nil {
				t.Fatalf("recording the request: %v", err)
			}
			receipt := stager.seen.Rendered
			if stager.calls != 1 || receipt.Key != TemplateRequestReceipt || receipt.Language != string(mailcopy.Vietnamese) {
				t.Fatalf("staged %d mails, the last %q in %q; want the request receipt in Vietnamese at once",
					stager.calls, receipt.Key, receipt.Language)
			}
			if len(tasks.opened) != 1 || tasks.opened[0].Right != c.right {
				t.Fatalf("opened tasks %+v, want one %s task", tasks.opened, c.right)
			}
			// Due 15 days after the request, and the reply 2 to 4 calendar days
			// after it, so the two lie 11 to 13 days apart whatever the weekday.
			task := tasks.opened[0]
			if gap := task.DueAt.Sub(task.ReplyBy); gap < 11*24*time.Hour || gap > 13*24*time.Hour {
				t.Errorf("the task is due %s and the reply by %s, want 15 days and 2 working days after the request",
					task.DueAt, task.ReplyBy)
			}
		})
	}
}
