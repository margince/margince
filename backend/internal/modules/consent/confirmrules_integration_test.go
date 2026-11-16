// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// confirmRulesCountry is one minted code under both confirm-link rules: a
// record check asks no news, and a consent mail needs the subject's request.
// One code because the package has few to spare, and no test here reads both.
var confirmRulesCountry struct {
	once sync.Once
	code jurisdiction.Code
}

func placeUnderConfirmRules(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	confirmRulesCountry.once.Do(func() {
		confirmRulesCountry.code = testJurisdiction(t)
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: confirmRulesCountry.code, Version: 1,
			RecordConfirmationWithoutNews: true, ConsentConfirmationNeedsRequest: true,
		})
	})
	placeContact(t, e, string(confirmRulesCountry.code))
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

func placeContact(t *testing.T, e *channelConsentEnv, code string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET address_country = upper($2) WHERE id = $1`, e.contact, code); err != nil {
		t.Fatalf("placing the contact in %s: %v", code, err)
	}
}

// TestARecordCheckAsksNoNewsWhereTheRecipientsRulesBarIt. The German pack's
// record confirmation drops the news question. The mail says nothing about
// news and the link pins no question. An answer to one is refused, and the
// link stays unspent.
func TestARecordCheckAsksNoNewsWhereTheRecipientsRulesBarIt(t *testing.T) {
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
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)

	var invalid *ValidationError
	_, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", "")
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Reason, "has not asked to subscribe") {
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
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	placeUnderConfirmRules(t, e)
	request := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_qualifying_event (id, contact_id, kind, note, occurred_at, source, captured_by)
		VALUES ($1, $2, $3, 'Asked on the phone for the newsletter', $4, 'human', 'human:x')`,
		request, e.contact, KindRequestedBySubject, clocktest.Now(t).Add(-time.Hour)); err != nil {
		t.Fatalf("recording the request: %v", err)
	}
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
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO setting (key, value) VALUES ('consent.controller_particulars',
		  '{"LegalName":"Example Trading GmbH","PrivacyPage":"https://example.test/privacy"}')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatalf("stating the particulars: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE consent_purpose SET frequency = 'weekly' WHERE id = $1`, e.doiNews); err != nil {
		t.Fatal(err)
	}
	publishQuestions(t, e)

	issued, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", "en")
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
		e.contact, e.doiNews).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	want := "Confirm that you agree that Example Trading GmbH may use your name and email address to " +
		"send you DOI Newsletter by email, roughly once a week. You can withdraw your consent at any " +
		"time with the unsubscribe link in each email. Information about your rights: " +
		"https://example.test/privacy.\nConfirm subscription"
	if recorded != want {
		t.Errorf("the grant recorded:\n  %q\nwant:\n  %q", recorded, want)
	}
}
