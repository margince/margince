// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// statesFrequency reports whether text names a mail frequency in any language
// this build writes.
func statesFrequency(text string) bool {
	for _, language := range mailcopy.Languages() {
		words := mailcopy.For(string(language))
		for _, frequency := range []string{words.FrequencyWeekly, words.FrequencyMonthly, words.FrequencyQuarterly} {
			if strings.Contains(text, frequency) {
				return true
			}
		}
	}
	return false
}

// askedQuestion mints a consent link for purpose and answers what its mail
// says and the question its page serves.
func askedQuestion(t *testing.T, e *resolveEnv, store *Store, purpose ids.PurposeID) (string, ServedQuestion, IssuedConfirm) {
	t.Helper()
	ctx := withContactUpdate(e.ctx)
	tx, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := PublishMarketingQuestionTx(e.ctx, tx, clocktest.Now(t)); err != nil {
		t.Fatalf("publishing the questions: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	stager := &recordingStager{}
	store = store.WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test")
	issued, err := store.IssueConsentLink(ctx, e.contact, purpose, "", "")
	if err != nil {
		t.Fatalf("minting the consent link: %v", err)
	}
	ref, err := store.ResolveConfirmToken(ctx, issued.Token, FetchByAHuman)
	if err != nil {
		t.Fatal(err)
	}
	served, err := store.servedQuestionFor(ctx, ref)
	if err != nil || served == nil {
		t.Fatalf("the served question: %v, %v", served, err)
	}
	return stager.seen.Rendered.Body, *served, issued
}

// A consent request for correspondence promises no mail frequency: not in the
// mail, not on the page, and not in the sentence the grant records.
func TestACorrespondenceConsentStatesNoFrequency(t *testing.T) {
	e := setupResolve(t)
	seedDefaultCatalog(t, e)
	var correspondence ids.PurposeID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_purpose WHERE key = $1`, string(ClassBusinessCorrespondence)).Scan(&correspondence); err != nil {
		t.Fatal(err)
	}

	mail, served, issued := askedQuestion(t, e, e.store, correspondence)
	if statesFrequency(mail) {
		t.Errorf("the correspondence consent mail states a frequency:\n%s", mail)
	}
	if statesFrequency(served.Text) {
		t.Errorf("the correspondence consent page states a frequency: %q", served.Text)
	}
	if _, err := e.store.SubmitConfirmation(withContactUpdate(e.ctx), issued.Token, ConfirmSubmission{
		MarketingChoice: string(StateGranted), MarketingWording: served.Text,
	}); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	var recorded string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT policy_text FROM consent_event WHERE contact_id = $1 AND purpose_id = $2 AND new_state = 'granted'`,
		e.contact, correspondence).Scan(&recorded); err != nil {
		t.Fatalf("reading the grant: %v", err)
	}
	if recorded == "" || statesFrequency(recorded) {
		t.Errorf("the grant records %q, want the served sentence with no frequency", recorded)
	}
}

// A consent request for advertising still states how often the mail comes.
func TestAnAdvertisingConsentStatesItsFrequency(t *testing.T) {
	e := setupResolve(t)
	news := seedDoubleOptInPurpose(t, e, "rp_freq_news", "marketing")

	_, served, _ := askedQuestion(t, e, e.store, news)
	if !statesFrequency(served.Text) {
		t.Errorf("the advertising consent page states no frequency: %q", served.Text)
	}
}
