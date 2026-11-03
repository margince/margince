// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A website lead is kept only together with the Art. 14 notice it is owed.
//
// These decide real site_lead approvals through the accept step, the real
// capture Sink, controller mail lane, consent engine and bounce path. Only the
// switch is passed by: the lane here names the version this build sends.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// annasAddress is the address the team page published for Anna.
const annasAddress = "anna@acme.example"

// noticeLaneEnv is one finished site read of acmeTeamSite, the wired notice
// lane, and an approvals service deciding through the accept step.
type noticeLaneEnv struct {
	e        *integration.Env
	ctx      context.Context
	notices  *siteLeadNoticeLane
	vault    *sealingVault
	svc      *approvals.Service
	company  ids.UUID
	siteRead ids.UUID
}

func setupNoticeLane(t *testing.T) *noticeLaneEnv {
	t.Helper()
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	current, _ := consent.ControllerTemplateVersion(consent.TemplatePrivacyNotice)
	vault := &sealingVault{}
	notices := &siteLeadNoticeLane{reviewed: current}
	notices.store.Store(consent.NewStore(InstallationDB(e.Pool)).
		WithConfirmationLane(NewControllerMailQueue(e.Pool, inserter, ""), vault, "https://crm.example.test"))
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	read, _ := runTeamDeepRead(t, e, company)
	svc := approvals.NewService(e.DB())
	svc.WithEffect(siteLeadProposalKind,
		siteLeadAcceptEffect(svc, newCaptureSink(e.Pool, CaptureConfig{}), e.Pool, notices))
	return &noticeLaneEnv{
		e: e, ctx: e.As(e.Rep2, nil, integration.AdminPerms), notices: notices, vault: vault,
		svc: svc, company: company, siteRead: read.ID,
	}
}

// stage proposes one contact the read found.
func (l *noticeLaneEnv) stage(t *testing.T, contact siteContact) ids.ApprovalID {
	t.Helper()
	return stageSiteLeadFrom(t, l.e, l.company, ids.NewV7(), l.siteRead, contact)
}

// accept decides the approval as the inbox does.
func (l *noticeLaneEnv) accept(id ids.ApprovalID) error {
	_, err := l.svc.Decide(l.ctx, id, true, nil)
	return err
}

// noticeRecord is the proof row of one accepted proposal, as state/failure.
func (l *noticeLaneEnv) noticeRecord(t *testing.T, id ids.ApprovalID) string {
	t.Helper()
	return l.e.WsScalar(t, `
		SELECT n.state || '/' || coalesce(n.failure, '') FROM lead_privacy_notice n
		 WHERE n.approval_id = $1`, id)
}

func TestAnAcceptedWebsiteLeadIsToldBeforeAnythingElseIsSent(t *testing.T) {
	l := setupNoticeLane(t)
	id := l.stage(t, annaOnTheTeamPage)
	if err := l.accept(id); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	e := l.e
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE source_system = 'siteread' AND email = $1`, annasAddress); n != 1 {
		t.Fatalf("%d website leads kept, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM comms_outbound o WHERE o.recipients ? $1`, annasAddress); n != 1 {
		t.Fatalf("%d messages to the lead, want exactly the notice", n)
	}
	if got := e.WsScalar(t, `
		SELECT d.verdict || '/' || d.resolved_category FROM communication_decision d
		  JOIN comms_outbound o ON o.id = d.delivery_id
		 WHERE o.template_key = 'privacy_notice'`); got != "allow/privacy_notice" {
		t.Fatalf("staging decision = %q, want allow/privacy_notice", got)
	}
	version, ok := consent.ControllerTemplateVersion(consent.TemplatePrivacyNotice)
	if !ok {
		t.Fatal("privacy notice template is not registered")
	}
	wantProof := fmt.Sprintf("%s@%d/en", consent.TemplatePrivacyNotice, version)
	if got := e.WsScalar(t, `
		SELECT n.template_key || '@' || n.template_version || '/' || n.locale
		  FROM lead_privacy_notice n
		  JOIN lead l ON l.id = n.lead_id
		  JOIN comms_outbound o ON o.id = n.delivery_id
		 WHERE n.approval_id = $1 AND n.state = 'queued'`, id); got != wantProof {
		t.Fatalf("proof row = %q, want queued %q", got, wantProof)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND consumed_at IS NOT NULL`, id); n != 1 {
		t.Fatal("the approval was not redeemed with the notice")
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'lead_privacy_notice.created'`); n != 1 {
		t.Fatalf("%d lead_privacy_notice.created events, want 1 beside the proof row", n)
	}
}

func TestADecisionRedrivenAfterTheReviewChangedSendsNothing(t *testing.T) {
	l := setupNoticeLane(t)
	id := l.stage(t, annaOnTheTeamPage)
	l.notices.reviewed++
	if err := l.accept(id); !errors.Is(err, errSiteLeadNoticeUnreviewed) {
		t.Fatalf("accepting under a review of other wording = %v, want unreviewed", err)
	}
	// Deciding again is the re-drive, and it reaches the accept step alone.
	if err := l.accept(id); !errors.Is(err, errSiteLeadNoticeUnreviewed) {
		t.Fatalf("re-driving under a review of other wording = %v, want unreviewed", err)
	}
	e := l.e
	if n := e.WsCount(t, `SELECT count(*) FROM comms_outbound`); n != 0 {
		t.Fatalf("%d messages sent with unreviewed wording, want 0", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND consumed_at IS NULL`, id); n != 1 {
		t.Fatal("the refused decision spent the approval; it could never be carried out once reviewed")
	}
}

func TestTheNoticeLinkOpensTheDisclosureForTheLead(t *testing.T) {
	l := setupNoticeLane(t)
	if err := l.accept(l.stage(t, annaOnTheTeamPage)); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	token := l.vault.sealed[strings.LastIndex(l.vault.sealed, "/")+1:]
	store := consent.NewStore(InstallationDB(l.e.Pool))
	ref, err := store.ResolveConfirmToken(l.e.Admin(), token, consent.FetchByAHuman)
	if err != nil {
		t.Fatalf("resolving the lead's notice link: %v", err)
	}
	if ref.LeadID.IsZero() || !ref.ContactID.IsZero() {
		t.Fatalf("the link resolved to lead %v and contact %v, want the lead alone", ref.LeadID, ref.ContactID)
	}
	if mailed := l.e.WsScalar(t, `SELECT locale FROM lead_privacy_notice`); ref.QuestionLocale == "" || ref.QuestionLocale != mailed {
		t.Errorf("the lead's page names language %q and its mail was written in %q: the page must follow the mail",
			ref.QuestionLocale, mailed)
	}
	info, err := store.PrivacyInformationFor(l.e.Admin(), ref)
	if err != nil {
		t.Fatalf("assembling the lead's disclosure: %v", err)
	}
	if info.AcquiredAs != "public_or_business_source" || info.AcquiredAt == nil {
		t.Fatalf("the disclosure names source %q at %v, want the public source and when", info.AcquiredAs, info.AcquiredAt)
	}
}

func TestABouncedNoticeDeletesTheLeadAndLeavesARecordNamingNobody(t *testing.T) {
	l := setupNoticeLane(t)
	id := l.stage(t, annaOnTheTeamPage)
	if err := l.accept(id); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	e := l.e
	if n := e.WsCount(t, `SELECT count(*) FROM site_read WHERE contacts::text ILIKE '%anna%'`); n != 1 {
		t.Fatal("the site read does not hold Anna, so this test cannot show her leaving it")
	}
	messageID := e.WsScalar(t, `SELECT message_id FROM comms_outbound WHERE template_key = 'privacy_notice'`)
	if err := newBounceSink(e.Pool).RecordBounce(e.Admin(), connector.BounceReport{
		MessageID: messageID, Recipient: annasAddress, Kind: connector.BounceHard,
	}); err != nil {
		t.Fatalf("recording the bounce: %v", err)
	}
	for what, query := range map[string]string{
		"lead":              `SELECT count(*) FROM lead WHERE source_system = 'siteread'`,
		"notice link":       `SELECT count(*) FROM confirm_token WHERE lead_id IS NOT NULL`,
		"notice mail":       `SELECT count(*) FROM comms_outbound WHERE template_key = 'privacy_notice'`,
		"captured original": `SELECT count(*) FROM raw_capture WHERE source_system = 'siteread'`,
		"address stop":      `SELECT count(*) FROM communication_suppression WHERE lower(address) = 'anna@acme.example'`,
		"proposal details": `SELECT count(*) FROM approval
			WHERE (proposed_change::text || summary || coalesce(evidence::text, '')) ILIKE '%anna%'`,
		"site read entry":            `SELECT count(*) FROM site_read WHERE contacts::text ILIKE '%anna%'`,
		"notice row naming the lead": `SELECT count(*) FROM lead_privacy_notice n WHERE row_to_json(n)::text ~* 'anna|muster|acme'`,
	} {
		if n := e.WsCount(t, query); n != 0 {
			t.Errorf("%d %s rows left after the notice bounced, want 0", n, what)
		}
	}
	if got := e.WsScalar(t, `
		SELECT n.state || '/' || n.failure || '/' || (n.lead_id IS NULL) || '/' || (n.delivery_id IS NULL)
		  FROM lead_privacy_notice n WHERE n.approval_id = $1`, id); got != "failed/bounced/true/true" {
		t.Fatalf("the record of the failed notice = %q, want failed/bounced naming no lead or delivery", got)
	}
}

func TestAWebsiteLeadWithoutAPublishedAddressIsNeverKept(t *testing.T) {
	l := setupNoticeLane(t)
	unaddressed := annaOnTheTeamPage
	unaddressed.PublishedEmail = ""
	id := l.stage(t, unaddressed)
	if err := l.accept(id); !errors.Is(err, errSiteLeadHasNoAddress) {
		t.Fatalf("accepting a website lead with no published address = %v, want the no-address refusal", err)
	}
	e := l.e
	if n := e.WsCount(t, `SELECT count(*) FROM lead`); n != 0 {
		t.Fatalf("%d leads kept for somebody who cannot be told, want 0", n)
	}
	if got := l.noticeRecord(t, id); got != "failed/no_address" {
		t.Fatalf("the record of the failed notice = %q, want failed/no_address", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND proposed_change = '{}'::jsonb`, id); n != 1 {
		t.Fatal("the proposal still carries the contact's details after they could not be told")
	}
}

func TestANoticeTheEngineRefusesKeepsNoLead(t *testing.T) {
	l := setupNoticeLane(t)
	id := l.stage(t, annaOnTheTeamPage)
	// A dead mailbox: the engine refuses the notice, so the capture rolls back.
	l.e.WsExec(t, `
		INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'hard_bounce', 'test', 'system:test', 'machine')`, annasAddress)
	if err := l.accept(id); err == nil {
		t.Fatal("a notice the engine refused was reported as sent")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM lead`); n != 0 {
		t.Fatalf("%d leads kept although their notice was refused, want 0", n)
	}
	if got := l.noticeRecord(t, id); got != "failed/refused" {
		t.Fatalf("the record of the failed notice = %q, want failed/refused", got)
	}
}

func TestAnAddressAnotherLeadHoldsKeepsNothingFromTheWebsite(t *testing.T) {
	l := setupNoticeLane(t)
	e := l.e
	name, address := "Anna M.", annasAddress
	incumbent, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &address})
	if err != nil {
		t.Fatalf("creating the lead another source holds: %v", err)
	}
	before := e.WsScalar(t, `SELECT row_to_json(l)::text FROM lead l WHERE id = $1`, incumbent.Id)
	id := l.stage(t, annaOnTheTeamPage)
	if err := l.accept(id); !errors.Is(err, errSiteLeadCollision) {
		t.Fatalf("accepting a website lead whose address another lead holds = %v, want the collision refusal", err)
	}
	if after := e.WsScalar(t, `SELECT row_to_json(l)::text FROM lead l WHERE id = $1`, incumbent.Id); after != before {
		t.Errorf("the existing lead changed:\nbefore %s\nafter  %s", before, after)
	}
	for what, query := range map[string]string{
		"website lead":      `SELECT count(*) FROM lead WHERE source_system = 'siteread'`,
		"captured original": `SELECT count(*) FROM raw_capture WHERE source_system = 'siteread'`,
		"merge proposal":    `SELECT count(*) FROM approval WHERE kind = 'merge_records'`,
		"notice mail":       `SELECT count(*) FROM comms_outbound`,
	} {
		if n := e.WsCount(t, query); n != 0 {
			t.Errorf("%d %s rows after a collision, want 0", n, what)
		}
	}
	if got := l.noticeRecord(t, id); got != "failed/collision" {
		t.Fatalf("the record of the failed notice = %q, want failed/collision", got)
	}
}

// The address became a contact's while the proposal waited. The accept asks again:
// no lead, no notice, and the decider is told they are already a contact.
func TestAWebsiteLeadForAnExistingContactKeepsNothing(t *testing.T) {
	l := setupNoticeLane(t)
	e := l.e
	id := l.stage(t, annaOnTheTeamPage)
	if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Anna M.", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: annasAddress, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	if err := l.accept(id); !errors.Is(err, errSiteLeadIsAContact) {
		t.Fatalf("accepting a website lead for an existing contact = %v, want the already-a-contact refusal", err)
	}
	for what, query := range map[string]string{
		"lead":        `SELECT count(*) FROM lead`,
		"notice mail": `SELECT count(*) FROM comms_outbound`,
		"task":        `SELECT count(*) FROM activity WHERE kind = 'task'`,
	} {
		if n := e.WsCount(t, query); n != 0 {
			t.Errorf("%d %s rows after accepting a website lead for a contact, want 0", n, what)
		}
	}
	if got := l.noticeRecord(t, id); got != "failed/collision" {
		t.Fatalf("the record of the untold notice = %q, want failed/collision", got)
	}
}

func TestAFailedRecordOfAnUntoldLeadIsRetriedByDecidingAgain(t *testing.T) {
	l := setupNoticeLane(t)
	unaddressed := annaOnTheTeamPage
	unaddressed.PublishedEmail = ""
	id := l.stage(t, unaddressed)
	disarm := failLeadNoticeWrites(t, integration.OwnerConn(t))
	if err := l.accept(id); err == nil || errors.Is(err, errSiteLeadHasNoAddress) {
		t.Fatalf("a decision whose failure record could not be written = %v, want the write's fault", err)
	}
	e := l.e
	if n := e.WsCount(t, `
		SELECT count(*) FROM approval
		 WHERE id = $1 AND consumed_at IS NULL AND proposed_change <> '{}'::jsonb`, id); n != 1 {
		t.Fatal("a failed cleanup spent or emptied the approval, so nothing can retry it")
	}
	disarm()
	if err := l.accept(id); !errors.Is(err, errSiteLeadHasNoAddress) {
		t.Fatalf("deciding again = %v, want the no-address refusal recorded this time", err)
	}
	if got := l.noticeRecord(t, id); got != "failed/no_address" {
		t.Fatalf("the record after the retry = %q, want failed/no_address", got)
	}
}

func TestAWebsiteLeadIsNotHandedOnWhileItsNoticeIsPending(t *testing.T) {
	l := setupNoticeLane(t)
	if err := l.accept(l.stage(t, annaOnTheTeamPage)); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	e := l.e
	leadID := l.websiteLead(t)
	other := l.otherLead(t)
	assertEveryHandoverRefused(t, l, leadID, other)
	if n := e.WsCount(t, `SELECT count(*) FROM contact`); n != 0 {
		t.Fatalf("%d contacts after the refused promotion, want 0", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND owner_id IS NULL AND merged_into_id IS NULL`, leadID); n != 1 {
		t.Fatal("a refused handover still gave the website lead an owner or merged it")
	}
}

// A website lead no notice row names, a notice that bounced softly, or one
// still inside the bounce window: each holds the lead. Only a notice sent,
// unbounced and out for the window lets it go.
func TestTheNoticeHoldFailsClosed(t *testing.T) {
	l := setupNoticeLane(t)
	e := l.e
	rep := e.As(e.Rep1, nil, integration.AdminPerms)
	name, address, source, key := "Rowless", "rowless@acme.example", provenance.WebsiteLeadSource, "rowless-key"
	rowless, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, Email: &address, SourceSystem: &source, SourceID: &key, Source: source,
	})
	if err != nil {
		t.Fatalf("creating a website lead with no notice row: %v", err)
	}
	if _, err := e.Contacts.ClaimRecord(rep, "lead", ids.UUID(rowless.Id), nil); !errors.Is(err, contacts.ErrWebsiteLeadNotYetTold) {
		t.Fatalf("claiming a website lead no notice row names = %v, want the pending refusal", err)
	}
	if err := l.accept(l.stage(t, annaOnTheTeamPage)); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	leadID := l.websiteLeadNamed(t, annasAddress)
	e.WsExec(t, `UPDATE comms_outbound SET status = 'sent', sent_at = now() - interval '4 days',
		bounced_at = now(), bounce_kind = 'soft' WHERE template_key = 'privacy_notice'`)
	if _, err := e.Contacts.ClaimRecord(rep, "lead", leadID.UUID, nil); !errors.Is(err, contacts.ErrWebsiteLeadNotYetTold) {
		t.Fatalf("claiming a website lead whose notice bounced = %v, want the pending refusal", err)
	}
	e.WsExec(t, `UPDATE comms_outbound SET bounced_at = NULL, bounce_kind = NULL,
		sent_at = now() - interval '1 hour' WHERE template_key = 'privacy_notice'`)
	if _, err := e.Contacts.ClaimRecord(rep, "lead", leadID.UUID, nil); !errors.Is(err, contacts.ErrWebsiteLeadNotYetTold) {
		t.Fatalf("claiming inside the bounce window = %v, want the pending refusal", err)
	}
	e.WsExec(t, `UPDATE comms_outbound SET sent_at = now() - interval '4 days' WHERE template_key = 'privacy_notice'`)
	if _, err := e.Contacts.ClaimRecord(rep, "lead", leadID.UUID, nil); err != nil {
		t.Fatalf("claiming a website lead told four days ago without a bounce = %v, want it claimed", err)
	}
}

// The writers of the notice and of its erasure act on what the accept itself
// recorded, never on a column a request can set.
func TestTheNoticeWritersRefuseAnyOtherRecord(t *testing.T) {
	l := setupNoticeLane(t)
	e := l.e
	other := l.otherLead(t)
	// A lead under the website source that no accept captured: what a forged
	// source_system would look like if a door ever let one through.
	name, address, source, key := "Forged", "forged@acme.example", provenance.WebsiteLeadSource, "forged-key"
	forged, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, Email: &address, SourceSystem: &source, SourceID: &key, Source: source,
	})
	if err != nil {
		t.Fatalf("creating a lead under the website source by hand: %v", err)
	}
	forgedID := ids.From[ids.LeadKind](ids.UUID(forged.Id))
	// A site_lead proposal decided and redeemed by an effect that keeps nobody.
	redeemed := l.stage(t, annaOnTheTeamPage)
	redeemOnly := approvals.NewService(e.DB())
	redeemOnly.WithEffect(siteLeadProposalKind, func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, hash string) error {
		_, _, err := redeemOnly.Redeem(ctx, id, siteLeadProposalKind, hash)
		return err
	})
	if _, err := redeemOnly.Decide(l.ctx, redeemed, true, nil); err != nil {
		t.Fatalf("redeeming a site_lead proposal: %v", err)
	}
	teller, _ := l.notices.teller()
	for what, lead := range map[string]ids.LeadID{"another source's lead": other, "a forged website lead": forgedID} {
		if _, err := teller.IssueLeadPrivacyNotice(l.ctx, consent.LeadNotice{LeadID: lead, ApprovalID: redeemed}); err == nil {
			t.Errorf("%s was sent the notice under a proposal that did not capture it", what)
		}
		err := databaseTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
			return privacy.EraseUntoldLeadTx(ctx, tx, privacy.UntoldLead{LeadID: lead, ApprovalID: redeemed})
		})
		if err == nil {
			t.Errorf("the untold-lead eraser accepted %s", what)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1`, lead); n != 1 {
			t.Errorf("%s is gone", what)
		}
	}
	pending := l.stage(t, annaOnTheTeamPage)
	err = databaseTx(t, e, func(ctx context.Context, tx pgx.Tx) error {
		return consent.RecordLeadNoticeFailedTx(ctx, tx, pending, consent.LeadNoticeRefused)
	})
	if err == nil {
		t.Fatal("a failure was recorded against a proposal nobody redeemed")
	}
}

// A request cannot move a website lead out of the hold, and cannot create a
// lead under the website source in the first place.
func TestARequestCannotDodgeTheNoticeHold(t *testing.T) {
	l := setupNoticeLane(t)
	if err := l.accept(l.stage(t, annaOnTheTeamPage)); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	e := l.e
	leadID := l.websiteLead(t)
	admin := e.Admin()
	relabel := "manual"
	if _, err := e.Contacts.UpdateLead(admin, leadID, contacts.UpdateLeadInput{Source: &relabel}); err == nil {
		t.Fatal("a request relabelled the website lead's source")
	}
	if _, err := e.Contacts.ClaimRecord(e.As(e.Rep1, nil, integration.AdminPerms), "lead", leadID.UUID, nil); !errors.Is(err, contacts.ErrWebsiteLeadNotYetTold) {
		t.Fatalf("claiming after relabelling the source = %v, want the pending refusal", err)
	}
	_, err := contacts.NewProvider(e.DB()).Create(admin, datasource.CreateInput{
		EntityType: datasource.EntityLead, Source: "manual",
		Fields: map[string]any{
			"full_name": "From a request", "email": "request@acme.example",
			"source_system": provenance.WebsiteLeadSource, "source_id": "from-a-request",
		},
	})
	if err == nil {
		t.Fatal("a request created a lead under the website source")
	}
}

func TestErasingTheContactAWebsiteLeadBecameRemovesTheNoticeLink(t *testing.T) {
	l := setupNoticeLane(t)
	if err := l.accept(l.stage(t, annaOnTheTeamPage)); err != nil {
		t.Fatalf("accepting the website lead: %v", err)
	}
	e := l.e
	contact := e.SeedContact(t, "Anna Muster", &e.Rep1)
	e.WsExec(t, `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:test')`, contact, annasAddress)
	eraser := privacy.NewEraser(e.DB()).WithPayloadVault(l.vault)
	if err := eraser.EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM confirm_token WHERE delivered_to = $1`, annasAddress); n != 0 {
		t.Fatalf("%d notice links still carry the erased address, want 0", n)
	}
}

// failLeadNoticeWrites makes every write of a proof row fail until the
// returned disarm runs. Disarmed in cleanup too: the lane keeps the schema
// between tests.
func failLeadNoticeWrites(t *testing.T, owner *pgx.Conn) func() {
	t.Helper()
	ctx := context.Background()
	for _, statement := range []string{
		`CREATE OR REPLACE FUNCTION lead_notice_write_fault() RETURNS trigger
		 LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'lead notice write fault injection'; END $$`,
		`CREATE TRIGGER lead_notice_write_fault_trigger BEFORE INSERT ON lead_privacy_notice
		 FOR EACH ROW EXECUTE FUNCTION lead_notice_write_fault()`,
	} {
		if _, err := owner.Exec(ctx, statement); err != nil {
			t.Fatalf("arming the fault injection: %v", err)
		}
	}
	disarm := func() {
		for _, statement := range []string{
			`DROP TRIGGER IF EXISTS lead_notice_write_fault_trigger ON lead_privacy_notice`,
			`DROP FUNCTION IF EXISTS lead_notice_write_fault()`,
		} {
			if _, err := owner.Exec(context.Background(), statement); err != nil {
				t.Errorf("disarming the fault injection: %v", err)
			}
		}
	}
	t.Cleanup(disarm)
	return disarm
}

// websiteLead is the one website lead the accepted proposal kept.
func (l *noticeLaneEnv) websiteLead(t *testing.T) ids.LeadID {
	t.Helper()
	return l.websiteLeadNamed(t, annasAddress)
}

// websiteLeadNamed is the website lead holding this address.
func (l *noticeLaneEnv) websiteLeadNamed(t *testing.T, address string) ids.LeadID {
	t.Helper()
	return ids.From[ids.LeadKind](ids.MustParse(l.e.WsScalar(t,
		`SELECT id::text FROM lead WHERE source_system = 'siteread' AND email = $1`, address)))
}

// otherLead is a lead a human created, which owes no website notice.
func (l *noticeLaneEnv) otherLead(t *testing.T) ids.LeadID {
	t.Helper()
	name, address := "Bea Typed", "bea@elsewhere.example"
	lead, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &address})
	if err != nil {
		t.Fatalf("creating a lead by hand: %v", err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

// assertEveryHandoverRefused drives each path that passes a lead to
// somebody, and expects the notice hold from each.
func assertEveryHandoverRefused(t *testing.T, l *noticeLaneEnv, leadID, other ids.LeadID) {
	t.Helper()
	e := l.e
	rep := e.As(e.Rep1, nil, integration.AdminPerms)
	owner := ids.From[ids.UserKind](e.Rep1)
	for path, err := range map[string]error{
		"promote": func() error {
			_, _, err := e.Contacts.PromoteLead(rep, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"})
			return err
		}(),
		"claim": func() error { _, err := e.Contacts.ClaimRecord(rep, "lead", leadID.UUID, nil); return err }(),
		"assign by update": func() error {
			_, err := e.Contacts.UpdateLead(rep, leadID, contacts.UpdateLeadInput{OwnerID: &owner})
			return err
		}(),
		"merge into another lead": func() error { _, err := e.Contacts.MergeLead(rep, leadID, other); return err }(),
		"merge another lead in":   func() error { _, err := e.Contacts.MergeLead(rep, other, leadID); return err }(),
	} {
		if !errors.Is(err, contacts.ErrWebsiteLeadNotYetTold) {
			t.Errorf("%s of a website lead whose notice is pending = %v, want the pending refusal", path, err)
		}
	}
	decision, err := e.Contacts.RouteLead(rep, leadID, contacts.RoutingConfig{Owners: []ids.UserID{owner}})
	if err != nil || decision.Assigned || decision.Reason != "notice_pending" {
		t.Errorf("routing a website lead whose notice is pending = %+v, %v, want left unrouted", decision, err)
	}
}

// databaseTx runs fn in one workspace transaction as the admin.
func databaseTx(t *testing.T, e *integration.Env, fn func(context.Context, pgx.Tx) error) error {
	t.Helper()
	ctx := e.Admin()
	return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { return fn(ctx, tx) })
}
