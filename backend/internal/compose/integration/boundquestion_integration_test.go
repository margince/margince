// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a subscription consent's proof says the subject agreed to.
//
// consent_event.policy_text is that sentence, and on both confirm-link doors it
// came from the SUBMISSION BODY — whatever the subject's own browser said it
// had shown them. So the evidence a controller produces to show consent was
// freely given quoted a string the client chose, and a client that chose a
// different one would be believed. Meanwhile consent_text_version_id had sat
// unwritten since the migration that added it.
//
// AN EARLIER ATTEMPT BOUND THE WRONG SENTENCE and was reverted: it named the
// invitation mail's body, which says confirming turns a request into a
// permission. The PAGE asks something else — news roughly once a month — and
// that is what a subject answers. A proof naming the first would be published,
// checkable, server-controlled, and about the wrong proposition.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// grantedProof is what one proof row records about the wording.
type grantedProof struct {
	versionID *string
	text      string
	version   *string
}

func readGrantedProof(t *testing.T, c *consentEnv, source string) grantedProof {
	t.Helper()
	var out grantedProof
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT consent_text_version_id::text, policy_text, policy_version
		  FROM consent_event
		 WHERE source = $1
		 ORDER BY captured_at DESC LIMIT 1`, source).
		Scan(&out.versionID, &out.text, &out.version); err != nil {
		t.Fatalf("reading the proof: %v", err)
	}
	return out
}

// mintAConfirmLink has the workspace mail a record-confirmation link.
func mintAConfirmLink(t *testing.T, c *consentEnv) string {
	t.Helper()
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/confirm-request",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("ask the workspace to mail the confirm link → %d", status)
	}
	return confirmLinkToken(t, c.AppEnv)
}

// THE PROOF NAMES THE PUBLISHED QUESTION, NOT THE CLIENT'S SENTENCE.
func TestAGrantNamesThePublishedQuestionItAnswered(t *testing.T) {
	c := setupConsent(t)
	token := mintAConfirmLink(t, c)

	// The client sends something else entirely. Before this change it would
	// have been recorded verbatim as what the subject agreed to.
	var receipt crmcontracts.ConfirmSubmissionReceipt
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+token, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "I agree to absolutely anything whatsoever.",
	}, nil, &receipt); s != http.StatusOK {
		t.Fatalf("the subject spends their own link → %d, want 200", s)
	}

	proof := readGrantedProof(t, c, "confirm_details")
	if proof.versionID == nil {
		t.Fatal("the grant names no published question, so its proof still rests on a sentence " +
			"that arrived in the same request as the answer it is meant to evidence")
	}
	if strings.Contains(proof.text, "absolutely anything whatsoever") {
		t.Error("the proof quotes the CLIENT's sentence — a client that chose a different one " +
			"would be believed")
	}

	// IT NAMES A ROW THAT EXISTS AND SAYS THE SAME THING, so a reader following
	// the id is not given a second, disagreeing answer to what was agreed.
	var body, key string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT body, key FROM consent_text_version WHERE id = $1 AND published_at IS NOT NULL`,
		*proof.versionID).Scan(&body, &key); err != nil {
		t.Fatalf("the proof names a wording that is not published: %v", err)
	}
	if key != "marketing_question" {
		t.Errorf("the proof names the %q wording — a subscription consent is given to the "+
			"question on the page, not to the mail that carried the link", key)
	}
	if proof.text != body {
		t.Errorf("policy_text and the row it names disagree:\n  proof: %q\n  row:   %q",
			proof.text, body)
	}
	// THE ANSWERS ARE IN IT. What somebody agreed to is the question and the
	// option they picked together; a version pinning only the question would
	// let the labels change under it.
	if !strings.Contains(body, "keep me posted") {
		t.Errorf("the published question does not carry the answers the subject chose between: %q",
			body)
	}
}

// THE DEDICATED CONSENT LINK BINDS TOO.
//
// Two doors write a grant from a confirm link, and this is the one most
// double-opt-in consents actually arrive through. Fixing only the other would
// have left the defect standing where it matters most.
func TestADedicatedConsentLinkBindsTheQuestionToo(t *testing.T) {
	c := setupConsent(t)

	// A double-opt-in link is minted for one purpose and asks only about that.
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/double-opt-in",
		AnyMap{"purpose_id": c.purposes["marketing_email"]}, nil, nil); status >= 400 {
		t.Fatalf("minting the double-opt-in link → %d", status)
	}
	token := confirmLinkToken(t, c.AppEnv)

	var receipt crmcontracts.ConfirmSubmissionReceipt
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+token, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "whatever the client says",
	}, nil, &receipt); s != http.StatusOK {
		t.Fatalf("spending the consent link → %d, want 200", s)
	}

	proof := readGrantedProof(t, c, "consent_link")
	if proof.versionID == nil {
		t.Fatal("a dedicated consent link's grant names no published question, so the door most " +
			"double-opt-in consents come through still records the client's sentence")
	}

	// AND IT NAMES THE RIGHT ONE, which asserting a non-null id does not show.
	// This page asks a different question from the record-confirmation page: it
	// names the purpose the link was minted for rather than describing a
	// frequency. Binding the other door's sentence would be published,
	// checkable, server-controlled — and about a proposition nobody was asked,
	// which is the defect an earlier attempt shipped and had to revert.
	var key, body string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT key, body FROM consent_text_version WHERE id = $1`, *proof.versionID).
		Scan(&key, &body); err != nil {
		t.Fatalf("reading the bound question: %v", err)
	}
	if key != "subscription_question" {
		t.Errorf("the subscription page's grant names the %q wording — the subject was asked to "+
			"confirm a named purpose, not the generic monthly-news question", key)
	}
	if !strings.Contains(body, "{purpose}") {
		t.Errorf("the published question does not carry the placeholder the page fills: %q", body)
	}

	// AND THE PROOF NAMES THE PURPOSE, not the placeholder. The published row
	// keeps {purpose} — one row serves every purpose — but what the subject
	// read had their subscription's own name in it, and the export hands them
	// policy_text with the purpose's KEY beside it rather than its label. A
	// hole in the sentence is a hole nobody can fill.
	if strings.Contains(proof.text, "{purpose}") {
		t.Errorf("the proof records a sentence with a hole in it: %q", proof.text)
	}
	var label string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT label FROM consent_purpose WHERE key = 'marketing_email'`).Scan(&label); err != nil {
		t.Fatalf("reading the purpose label: %v", err)
	}
	if !strings.Contains(proof.text, label) {
		t.Errorf("the proof does not name what the subject subscribed to:\n  proof: %q\n  want it to contain: %q",
			proof.text, label)
	}
}

// A LINK THAT PINNED NOTHING KEEPS THE EVIDENCE IT HAD.
//
// Links minted before the question was pinned name no version and no locale.
// Refusing those grants would be punishing subjects for our own missing
// bookkeeping, so they record the client's sentence exactly as before — and
// name no published row, which says plainly that nothing was checked.
func TestALinkThatPinnedNoQuestionStillRecordsTheGrant(t *testing.T) {
	c := setupConsent(t)
	token := mintAConfirmLink(t, c)

	// Unpin it, as a link minted before this change would be.
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE confirm_token SET question_locale = NULL, question_version = NULL`); err != nil {
		t.Fatalf("unpinning the link: %v", err)
	}

	var receipt crmcontracts.ConfirmSubmissionReceipt
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+token, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "Yes, send me occasional product news.",
	}, nil, &receipt); s != http.StatusOK {
		t.Fatalf("the subject's consent was refused over our own bookkeeping → %d", s)
	}

	proof := readGrantedProof(t, c, "confirm_details")
	if proof.versionID != nil {
		t.Errorf("an unpinned link named published wording %q — we cannot say which of three "+
			"rows the subject read, and guessing is what this whole change exists to stop",
			*proof.versionID)
	}
	if proof.text != "Yes, send me occasional product news." {
		t.Errorf("the fallback did not record what the client sent: %q", proof.text)
	}
}
