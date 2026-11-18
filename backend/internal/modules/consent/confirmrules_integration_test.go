// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

func placeUnderConfirmRules(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	code := testMessagingRules(t, messagingrules.Rules{
		Version: 1, RecordConfirmationWithoutNews: true, ConsentConfirmationNeedsRequest: true,
	})
	placeContact(t, e.owner, e.contact, code)
}

// publishQuestions publishes the questions a link pins, as the boot does.
func publishQuestions(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := PublishMarketingQuestionTx(e.ctx, tx, clocktest.Now(t)); err != nil {
		t.Fatalf("publishing the questions: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestARecordCheckAsksNoNewsWhereTheRecipientsRulesBarIt. The German pack's
// record confirmation drops the news question. The mail says nothing about
// news and the link pins no question. An answer to one is refused, and the
// link stays unspent.
func TestARecordCheckAsksNoNewsWhereTheRecipientsRulesBarIt(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	stager := &recordingStager{}
	e.store = e.store.WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test")
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)

	issued, err := e.store.IssueConfirmToken(e.ctx, e.contact, "")
	if err != nil {
		t.Fatalf("minting the record check: %v", err)
	}
	if got := stager.seen.Rendered.Key; got != TemplateRecordCheck {
		t.Errorf("the mail went out as %q, want %q", got, TemplateRecordCheck)
	}
	if containsFold(stager.seen.Rendered.Body, "news") {
		t.Errorf("the record check mentions news:\n%s", stager.seen.Rendered.Body)
	}
	ref, err := e.store.ResolveConfirmToken(e.ctx, issued.Token, FetchByAHuman)
	if err != nil {
		t.Fatal(err)
	}
	if ref.AsksNews || ref.QuestionKey != "" {
		t.Errorf("the link asks news %v and pins %q, want no question", ref.AsksNews, ref.QuestionKey)
	}
	var invalid *ValidationError
	if _, err := e.store.SubmitConfirmation(e.ctx, issued.Token, ConfirmSubmission{
		MarketingChoice: "granted", MarketingWording: "anything",
	}); !errors.As(err, &invalid) {
		t.Fatalf("a news answer on a link that asked none: %v, want a validation error", err)
	}
	if _, err := e.store.SubmitConfirmation(e.ctx, issued.Token, ConfirmSubmission{}); err != nil {
		t.Errorf("the refused answer spent the link: %v", err)
	}
}

// TestARecordConfirmationStillAsksNewsElsewhere is the other side: a recipient
// with no such rule gets the news question and the wording that names it.
func TestARecordConfirmationStillAsksNewsElsewhere(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	stager := &recordingStager{}
	e.store = e.store.WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test")
	seedSubjectAddress(t, e)

	issued, err := e.store.IssueConfirmToken(e.ctx, e.contact, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := stager.seen.Rendered.Key; got != TemplateRecordConfirmation {
		t.Errorf("the mail went out as %q, want %q", got, TemplateRecordConfirmation)
	}
	ref, err := e.store.ResolveConfirmToken(e.ctx, issued.Token, FetchByAHuman)
	if err != nil {
		t.Fatal(err)
	}
	if !ref.AsksNews || ref.QuestionKey != TemplateMarketingQuestion {
		t.Errorf("the link asks news %v and pins %q, want the marketing question", ref.AsksNews, ref.QuestionKey)
	}
}

// TestAConsentMailWithoutTheSubjectsRequestIsRefused. Under the Vietnamese
// pack a consent-confirmation mail answers the subject's own request; staff
// cannot send one to somebody who never asked.
func TestAConsentMailWithoutTheSubjectsRequestIsRefused(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)

	requireNoSubjectRequestRefusal(t, e)
}

// TestAPackNamingNoClassNeedsTheRequestForEveryPurpose. A pack that asks a
// request for consent mail and names no purpose class asks it for all of
// them, correspondence included.
func TestAPackNamingNoClassNeedsTheRequestForEveryPurpose(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedDefaultCatalog(t, e.owner)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)
	var correspondence ids.PurposeID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_purpose WHERE key = $1`, string(ClassBusinessCorrespondence)).Scan(&correspondence); err != nil {
		t.Fatalf("reading the seeded correspondence purpose: %v", err)
	}

	var missing *NoSubjectRequestError
	if _, err := e.store.IssueConsentLink(e.ctx, e.contact, correspondence, "", ""); !errors.As(err, &missing) {
		t.Fatalf("a correspondence consent mail with no request on record: %v, want the refusal", err)
	}
}

// requireNoSubjectRequestRefusal asks for a consent link as a user and wants
// the request rule's refusal, with no link minted.
func requireNoSubjectRequestRefusal(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	var missing *NoSubjectRequestError
	if _, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", ""); !errors.As(err, &missing) {
		t.Fatalf("a consent mail with no request on record: %v, want the refusal", err)
	}
	var minted int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM confirm_token WHERE contact_id = $1`, e.contact).Scan(&minted); err != nil {
		t.Fatal(err)
	}
	if minted != 0 {
		t.Errorf("the refusal minted %d links", minted)
	}
}

// TestTheGrantKeepsTheRequestItAnswers. With the request on record the mail
// goes, and the proof row of the grant names that request.
func TestTheGrantKeepsTheRequestItAnswers(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)
	asked, err := e.store.RecordQualifyingEvent(e.ctx, e.contact, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "Asked on the phone for the newsletter",
		OccurredAt: clocktest.Now(t).Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("recording the request: %v", err)
	}
	request := recordedID(t, asked)
	publishQuestions(t, e)

	issued, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", "")
	if err != nil {
		t.Fatalf("a consent mail answering a recorded request: %v", err)
	}
	ref, err := e.store.ResolveConfirmToken(e.ctx, issued.Token, FetchByAHuman)
	if err != nil {
		t.Fatal(err)
	}
	served, err := e.store.servedQuestionFor(e.ctx, ref)
	if err != nil || served == nil {
		t.Fatalf("the served question: %v, %v", served, err)
	}
	if _, err := e.store.SubmitConfirmation(e.ctx, issued.Token, ConfirmSubmission{
		MarketingChoice: "granted", MarketingWording: served.Text,
	}); err != nil {
		t.Fatalf("granting: %v", err)
	}
	var kept ids.UUID
	var recorded string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT subject_request_id, policy_text FROM consent_event
		 WHERE contact_id = $1 AND purpose_id = $2 AND new_state = 'granted'`,
		e.contact, e.doiNews).Scan(&kept, &recorded); err != nil {
		t.Fatal(err)
	}
	if kept != request {
		t.Errorf("the grant names request %v, want %v", kept, request)
	}
	if recorded != served.Text {
		t.Errorf("the grant recorded %q, want the served text %q", recorded, served.Text)
	}
}

// TestTheStoredConsentTextIsTheFilledSentence. What a grant records is the
// served question with the controller, the purpose, its frequency and the
// privacy page in it, and no placeholder left.
func TestTheStoredConsentTextIsTheFilledSentence(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stateParticulars(t, e, ControllerParticulars{
		LegalName: "Example Trading GmbH", PrivacyPage: "https://example.test/privacy",
	})
	configurer := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"consent_config": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	weekly, err := e.store.CreatePurpose(configurer, "weekly_news", "Weekly news", true, "weekly")
	if err != nil {
		t.Fatalf("creating a weekly purpose: %v", err)
	}
	publishQuestions(t, e)

	issued, err := e.store.IssueConsentLink(e.ctx, e.contact, weekly.ID, "", "en")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := e.store.ResolveConfirmToken(e.ctx, issued.Token, FetchByAHuman)
	if err != nil {
		t.Fatal(err)
	}
	served, err := e.store.servedQuestionFor(e.ctx, ref)
	if err != nil || served == nil {
		t.Fatalf("the served question: %v, %v", served, err)
	}
	if _, err := e.store.SubmitConfirmation(e.ctx, issued.Token, ConfirmSubmission{
		MarketingChoice: "granted", MarketingWording: served.Text,
	}); err != nil {
		t.Fatal(err)
	}
	var recorded string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT policy_text FROM consent_event WHERE contact_id = $1 AND purpose_id = $2`,
		e.contact, weekly.ID).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	want := "Confirm that you agree that Example Trading GmbH may use your name and email address to " +
		"send you Weekly news by email, roughly once a week. You can withdraw your consent at any " +
		"time with the unsubscribe link in each email. Information about your rights: " +
		"https://example.test/privacy.\nConfirm subscription"
	if recorded != want {
		t.Errorf("the grant recorded:\n  %q\nwant:\n  %q", recorded, want)
	}
}

// TestAStoredMailStampDoesNotStandInForTheMail. A stamped inbound_message is
// read again from the timeline, so a stamp whose mail is gone is no request.
func TestAStoredMailStampDoesNotStandInForTheMail(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)
	mail := plantActivity(t, e, "inbound", e.contact)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return RecordDerivedQualifyingEvent(e.ctx, tx, e.contact.String(), QualifyingEvent{
			Kind: "inbound_message", OccurredAt: clocktest.Now(t).Add(-time.Hour),
			SourceEntityType: entityActivity, SourceEntityID: mail.String(),
		}, "human:x")
	}); err != nil {
		t.Fatalf("stamping the mail: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET archived_at = now() WHERE id = $1`, mail); err != nil {
		t.Fatalf("archiving the mail: %v", err)
	}
	requireNoSubjectRequestRefusal(t, e)
}

// TestARequestRecordedForAnotherEntityIsNotThisMailsRequest. The mail is the
// installation's, sent for the default entity; a request recorded for a second
// entity does not answer for it.
func TestARequestRecordedForAnotherEntityIsNotThisMailsRequest(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)
	branch := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by)
		VALUES ($1, 'Branch', 'Branch Co Ltd', 'zk', 'human:x')`, branch); err != nil {
		t.Fatalf("adding a second entity: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO legal_entity_member (user_id, legal_entity_id, assigned_by)
		VALUES ($1, $2, 'human:x')`, e.user, branch); err != nil {
		t.Fatalf("placing the user in the branch: %v", err)
	}
	if _, err := e.store.RecordQualifyingEvent(e.ctx, e.contact, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "Asked the branch for the newsletter",
		OccurredAt: clocktest.Now(t).Add(-time.Hour),
	}); err != nil {
		t.Fatalf("recording the branch's request: %v", err)
	}
	requireNoSubjectRequestRefusal(t, e)
}

// plantActivity writes a mail the contact took part in; activities owns the
// writer, so the row is planted as the other tests here plant one.
func plantActivity(t *testing.T, e *channelConsentEnv, direction string, contact ids.ContactID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, now() - interval '1 hour', 'gmail', 'human:x')`, id, direction); err != nil {
		t.Fatalf("planting the mail: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, contact_id, address, role)
		VALUES ($1, $2, $3, 'from')`, id, contact, "subject-"+contact.String()+"@example.test"); err != nil {
		t.Fatalf("planting the participant: %v", err)
	}
	return id
}

// TestAMergeKeepsTheRequestAGrantAnswered. Both records hold the inquiry one
// booking made, and the merge drops the source's duplicate copies. The copy a
// grant answered stays, so the grant still names a request.
func TestAMergeKeepsTheRequestAGrantAnswered(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	survivor := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Survivor', 'manual', 'human:x')`,
		survivor); err != nil {
		t.Fatal(err)
	}
	booking := plantActivity(t, e, "inbound", e.contact)
	for _, contact := range []ids.ContactID{e.contact, survivor} {
		if err := e.store.RecordInquiry(e.ctx, contact, booking); err != nil {
			t.Fatalf("recording the inquiry: %v", err)
		}
	}
	publishQuestions(t, e)
	issued, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := e.store.ResolveConfirmToken(e.ctx, issued.Token, FetchByAHuman)
	if err != nil {
		t.Fatal(err)
	}
	served, err := e.store.servedQuestionFor(e.ctx, ref)
	if err != nil || served == nil {
		t.Fatalf("the served question: %v, %v", served, err)
	}
	if _, err := e.store.SubmitConfirmation(e.ctx, issued.Token, ConfirmSubmission{
		MarketingChoice: "granted", MarketingWording: served.Text,
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.CarrySatellitesTx(e.ctx, tx,
			commsauthz.ContactStopSubject(e.contact), commsauthz.ContactStopSubject(survivor))
	}); err != nil {
		t.Fatalf("carrying: %v", err)
	}
	var named *ids.UUID
	var cites *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT ce.subject_request_id, q.source_entity_id
		  FROM consent_event ce LEFT JOIN consent_qualifying_event q ON q.id = ce.subject_request_id
		 WHERE ce.purpose_id = $1 AND ce.new_state = 'granted'`, e.doiNews).Scan(&named, &cites); err != nil {
		t.Fatal(err)
	}
	if named == nil || cites == nil || *cites != booking {
		t.Errorf("after the merge the grant names request %v citing %v, want the booking's inquiry", named, cites)
	}
}

// TestTheMailedExpiryIsOnTheRecipientsClock. A contact whose country has one
// zone reads the expiry in that zone, named. A contact whose country has
// several reads it on the installation's clock, UTC here.
func TestTheMailedExpiryIsOnTheRecipientsClock(t *testing.T) {
	t.Parallel()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		country jurisdiction.Code
		expiry  func(words mailcopy.Copy, at time.Time) string
	}{
		{"jp", func(words mailcopy.Copy, at time.Time) string { return words.CountryInstant(at.In(tokyo), "jp") }},
		{"us", func(words mailcopy.Copy, at time.Time) string { return words.Instant(at.UTC()) }},
	} {
		e := setupChannelConsent(t)
		stager := &recordingStager{}
		e.store = e.store.WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test")
		seedSubjectAddress(t, e)
		placeContact(t, e.owner, e.contact, tc.country)
		issued, err := e.store.IssueConfirmToken(e.ctx, e.contact, "en")
		if err != nil {
			t.Fatalf("%s: minting the link: %v", tc.country, err)
		}
		want := tc.expiry(mailcopy.For(stager.seen.Rendered.Language), issued.ExpiresAt)
		if !strings.Contains(stager.seen.Rendered.Body, want) {
			t.Errorf("%s: the mail does not say %q:\n%s", tc.country, want, stager.seen.Rendered.Body)
		}
	}
}

// TestAContactWhoCameInThroughAFormHasAsked. The acquisition a form wrote is
// the contact's own request, read by the same owner as every other request.
func TestAContactWhoCameInThroughAFormHasAsked(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)
	// contacts owns the acquisition writer, which this module may not import.
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		VALUES ($1, $2, $3, 'human:x')`, e.contact, acquisitionEventOrForm, clocktest.Now(t).Add(-time.Hour)); err != nil {
		t.Fatalf("recording the form: %v", err)
	}
	if _, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", ""); err != nil {
		t.Errorf("a consent mail to a contact who came in through a form: %v, want it sent", err)
	}
}
