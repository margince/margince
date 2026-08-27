// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The confirm-your-details flow, end to end: mint a link, open it, answer, and
// find the answer recorded with the evidence that makes it defensible.
//
// The properties under test are the ones the design rests on. The link is
// single-use, so a replay refuses. The grant completes without a confirmation
// mail, and the proof row says what stood in for one. Corrections are proposals
// and never writes. And the whole submit is one transaction, so a refusal leaves
// nothing behind.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedMarketingPurpose plants the double-opt-in marketing lane the confirm page
// asks about, since the shared environment seeds its own purposes by other keys.
func seedMarketingPurpose(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO consent_purpose (key, label, requires_double_opt_in)
		 VALUES ($1, 'Marketing email', true)
		 ON CONFLICT (key) DO NOTHING`, PurposeMarketingEmail); err != nil {
		t.Fatalf("seed the marketing purpose: %v", err)
	}
}

// issueLink mints a confirm link for the environment's person, the way the send
// path will.
func issueLink(t *testing.T, e *channelConsentEnv) IssuedConfirm {
	t.Helper()
	issued, err := e.store.IssueConfirmToken(e.ctx, e.person, "subject@example.test")
	if err != nil {
		t.Fatalf("mint a confirm link: %v", err)
	}
	return issued
}

// marketingStateOf reads the subject's answer as the gate would.
func marketingStateOf(t *testing.T, e *channelConsentEnv) string {
	t.Helper()
	var state string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT coalesce(max(pc.state), '')
		  FROM person_consent pc
		  JOIN consent_purpose cp ON cp.id = pc.purpose_id
		 WHERE pc.person_id = $1 AND cp.key = $2`,
		e.person, PurposeMarketingEmail).Scan(&state); err != nil {
		t.Fatalf("read the marketing state: %v", err)
	}
	return state
}

// The card a person is shown. Every column it names has to exist, which a store
// test that reads a person with no employer never proves: the employer subquery
// only runs when there is an employment row to find, so the shape of the read
// went unexercised until it 500'd against a real record.
func TestTheCardShowsTheSubjectTheirOwnRecord(t *testing.T) {
	e := setupChannelConsent(t)
	orgID := ids.New[ids.OrganizationKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO organization (id, display_name, source, captured_by)
		 VALUES ($1, 'Acme GmbH', 'manual', 'human:x')`, orgID); err != nil {
		t.Fatalf("seed the employer: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO relationship (kind, person_id, organization_id, source, captured_by)
		 VALUES ('employment', $1, $2, 'manual', 'human:x')`, e.person, orgID); err != nil {
		t.Fatalf("seed the employment: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
		 VALUES ('person', $1, 'full_name', 'imprint page', 'connector:siteread')`,
		e.person); err != nil {
		t.Fatalf("seed the provenance: %v", err)
	}

	card, err := e.store.confirmCardFor(e.ctx, e.person)
	if err != nil {
		t.Fatalf("read the card: %v", err)
	}
	if card.FullName == "" {
		t.Error("the card shows no name")
	}
	if card.Company != "Acme GmbH" {
		t.Errorf("company = %q, want the live employer", card.Company)
	}
	if len(card.Provenance) == 0 {
		t.Error("no provenance — the Art. 14 answer is the strongest thing this page has")
	}
}

// A person clicks yes on the page. The grant lands, and it lands CONFIRMED —
// no second mail, because the link they arrived through already proved the
// mailbox.
func TestAnAnswerFromTheConfirmPageCompletesTheGrant(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	if err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		MarketingChoice:  string(StateGranted),
		MarketingWording: "News from time to time, roughly once a month.",
	}); err != nil {
		t.Fatalf("submit the answer: %v", err)
	}

	if got := marketingStateOf(t, e); got != string(StateGranted) {
		t.Fatalf("marketing = %q, want granted", got)
	}
	var confirmed bool
	var trigger, wording string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT ce.double_opt_in_confirmed_at IS NOT NULL,
		       coalesce(ce.issuance_trigger, ''), ce.policy_text
		  FROM consent_event ce
		  JOIN consent_purpose cp ON cp.id = ce.purpose_id
		 WHERE ce.person_id = $1 AND cp.key = $2
		 ORDER BY ce.captured_at DESC, ce.id DESC LIMIT 1`,
		e.person, PurposeMarketingEmail).Scan(&confirmed, &trigger, &wording); err != nil {
		t.Fatalf("read the proof row: %v", err)
	}
	if !confirmed {
		t.Error("the grant is unconfirmed — the click IS the confirmation, so the row must carry one")
	}
	if trigger != string(MailboxProvenByConfirmLink) {
		t.Errorf("issuance_trigger = %q, want %q — the proof row must name what stood in for the round trip",
			trigger, MailboxProvenByConfirmLink)
	}
	if wording == "" {
		t.Error("the proof row carries no wording — Art. 7(1) asks what the subject agreed TO")
	}
}

// The property the whole design rests on: the link is spent, so a replay of the
// same submit refuses. Without this the grant is replayable and proves nothing.
func TestAConfirmLinkAnswersOnce(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	first := ConfirmSubmission{
		MarketingChoice:  string(StateGranted),
		MarketingWording: "News from time to time.",
	}
	if err := e.store.SubmitConfirmation(e.ctx, link.Token, first); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		MarketingChoice:  string(StateWithdrawn),
		MarketingWording: "News from time to time.",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("second submit: %v, want not-found — a spent link must not answer twice", err)
	}
	if got := marketingStateOf(t, e); got != string(StateGranted) {
		t.Errorf("marketing = %q, want granted — the refused replay changed the answer", got)
	}
}

// Opening the page reads as absent once the link is spent, so somebody who
// forwards the mail cannot read the record afterwards.
func TestASpentLinkOpensNothing(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	if _, err := e.store.ResolveConfirmToken(e.ctx, link.Token); err != nil {
		t.Fatalf("resolve a live link: %v", err)
	}
	if err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := e.store.ResolveConfirmToken(e.ctx, link.Token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("resolve a spent link: %v, want not-found", err)
	}
}

// A correction is a PROPOSAL. The person record must be untouched, because the
// subject holds a bearer token and no principal — a leaked link that could
// rewrite the CRM is the thing this design exists to prevent.
func TestACorrectionIsStagedAndNeverWritten(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	var before string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(full_name, '') FROM person WHERE id = $1`, e.person).Scan(&before); err != nil {
		t.Fatalf("read the name before: %v", err)
	}

	if err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections: map[string]string{ConfirmFieldFullName: "Corrected Name"},
	}); err != nil {
		t.Fatalf("submit a correction: %v", err)
	}

	var after string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(full_name, '') FROM person WHERE id = $1`, e.person).Scan(&after); err != nil {
		t.Fatalf("read the name after: %v", err)
	}
	if after != before {
		t.Errorf("the person record changed from %q to %q — a bearer-token caller must not write the CRM",
			before, after)
	}

	var kind, field, value string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT kind, coalesce(field, ''), coalesce(proposed_value, '')
		  FROM person_confirm_submission WHERE person_id = $1`,
		e.person).Scan(&kind, &field, &value); err != nil {
		t.Fatalf("read the staged proposal: %v", err)
	}
	if kind != submissionCorrection || field != ConfirmFieldFullName || value != "Corrected Name" {
		t.Errorf("staged %s/%s/%q, want a correction of %s", kind, field, value, ConfirmFieldFullName)
	}
}

// Asking to be removed files a request rather than erasing. Art. 17 allows a
// month, and a link that could destroy a record is a weapon if it leaks.
func TestARemovalRequestIsFiledAndTheRecordSurvives(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	if err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		RequestErasure: true,
	}); err != nil {
		t.Fatalf("submit a removal request: %v", err)
	}

	var alive bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT archived_at IS NULL FROM person WHERE id = $1`, e.person).Scan(&alive); err != nil {
		t.Fatalf("read the person: %v", err)
	}
	if !alive {
		t.Error("the person was archived by a bearer-token caller — removal is a REQUEST a human resolves")
	}
	var open int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM person_confirm_submission
		 WHERE person_id = $1 AND kind = $2 AND resolved_at IS NULL`,
		e.person, submissionErasure).Scan(&open); err != nil {
		t.Fatalf("count the open requests: %v", err)
	}
	if open != 1 {
		t.Errorf("%d open removal request(s), want 1", open)
	}
}

// Answering nothing records nothing. A page view is not consent, and a person
// who corrects their address without answering has not opted in.
func TestOpeningThePageAndCorrectingGrantsNothing(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)

	if err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections: map[string]string{ConfirmFieldTitle: "CFO"},
	}); err != nil {
		t.Fatalf("submit without an answer: %v", err)
	}
	if got := marketingStateOf(t, e); got != "" {
		t.Errorf("marketing = %q, want no record at all — a page view grants nothing", got)
	}
}

// The submit is ONE transaction. A refused consent write must take the staged
// corrections with it, or the page half-worked with nothing to tell the subject
// which half.
func TestARefusedAnswerLeavesNoStagedCorrections(t *testing.T) {
	e := setupChannelConsent(t)
	// No marketing purpose seeded, so resolving it inside the transaction fails
	// AFTER the corrections have been staged — which is the ordering that makes
	// this a real test of the rollback rather than of validation.
	link := issueLink(t, e)

	err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections:      map[string]string{ConfirmFieldFullName: "Corrected Name"},
		MarketingChoice:  string(StateGranted),
		MarketingWording: "News from time to time.",
	})
	if err == nil {
		t.Fatal("submitting against an absent marketing purpose succeeded")
	}

	var staged int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM person_confirm_submission WHERE person_id = $1`, e.person).Scan(&staged); err != nil {
		t.Fatalf("count staged rows: %v", err)
	}
	if staged != 0 {
		t.Errorf("%d correction(s) survived a refused submit — the whole answer is one transaction", staged)
	}
	// And the link is unspent, so the person can answer again rather than
	// having burned their one chance on a server-side fault.
	if _, err := e.store.ResolveConfirmToken(e.ctx, link.Token); err != nil {
		t.Errorf("the link was spent by a refused submit: %v", err)
	}
}

// A field the page does not offer is refused before the link is spent, so a
// malformed client cannot cost somebody their one answer.
func TestAnUnofferedFieldIsRefusedWithoutSpendingTheLink(t *testing.T) {
	e := setupChannelConsent(t)
	link := issueLink(t, e)

	var invalid *ValidationError
	err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections: map[string]string{"owner_id": ids.NewV7().String()},
	})
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want a validation error on the field", err)
	}
	if _, err := e.store.ResolveConfirmToken(e.ctx, link.Token); err != nil {
		t.Errorf("a refused submit spent the link: %v", err)
	}
}

// A fresh issuance supersedes the last, so a person who is emailed twice cannot
// answer through the older link.
func TestAFreshLinkRetiresTheOneBeforeIt(t *testing.T) {
	e := setupChannelConsent(t)
	first := issueLink(t, e)
	second := issueLink(t, e)

	if _, err := e.store.ResolveConfirmToken(e.ctx, first.Token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the superseded link still opens: %v", err)
	}
	if _, err := e.store.ResolveConfirmToken(e.ctx, second.Token); err != nil {
		t.Errorf("the fresh link does not open: %v", err)
	}
}
