// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What lets the INSTALLATION write to somebody about its own obligations.
//
// The evidence is a live confirm_token, and these cases pin both halves of that:
// a message carrying one is supported, and the claim on its own supports
// nothing. The second half is what keeps the lane from becoming a way to reach
// a person who has objected — the five subject-serving categories pass a hard
// suppression, so a category that could be claimed without evidence would be a
// route around every refusal the engine makes.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// TestAConfirmationRestsOnALiveLink is the lane's admit case.
func TestAConfirmationRestsOnALiveLink(t *testing.T) {
	e := setupResolve(t)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(72*time.Hour), nil)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRecordConfirmation})

	if !got.Supported {
		t.Fatalf("a record confirmation with a live link resolved unsupported (%q) — the "+
			"installation cannot ask somebody to check what is held about them", got.Reason)
	}
	if got.Category != commsauthz.CategoryRecordConfirmation {
		t.Errorf("resolved %q, want record_confirmation", got.Category)
	}
	// A legal obligation rather than consent, and the ordering is the point:
	// asking somebody to check what is held is Art. 14 work the installation
	// owes them, so it cannot rest on a permission they have not given.
	if got.Basis != commsauthz.BasisLegalObligation {
		t.Errorf("basis %q, want legal_obligation: a confirmation that rested on consent could "+
			"never be sent to the person who has not consented yet", got.Basis)
	}
}

// TestAClaimedConfirmationWithNoLinkSupportsNothing is the refusal case, and it
// is the one that matters: without it the category is a bypass.
func TestAClaimedConfirmationWithNoLinkSupportsNothing(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRecordConfirmation})

	if got.Supported {
		t.Fatal("a record confirmation with no link resolved SUPPORTED. The five subject-serving " +
			"categories pass a hard suppression, so a claim that needs no evidence is a route to " +
			"somebody who has objected")
	}
}

// TestASpentLinkNoLongerSupportsAConfirmation — a link already followed is not
// evidence for a second mail.
func TestASpentLinkNoLongerSupportsAConfirmation(t *testing.T) {
	e := setupResolve(t)
	consumed := time.Now().Add(-time.Hour)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(72*time.Hour), &consumed)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRecordConfirmation})

	if got.Supported {
		t.Error("a consumed link still supports a confirmation, so the installation may keep " +
			"mailing somebody who already answered")
	}
}

// TestAnExpiredLinkNoLongerSupportsAConfirmation — a dead link makes the mail a
// dead end for whoever receives it.
func TestAnExpiredLinkNoLongerSupportsAConfirmation(t *testing.T) {
	e := setupResolve(t)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(-time.Hour), nil)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRecordConfirmation})

	if got.Supported {
		t.Error("an expired link still supports a confirmation, so the mail goes out carrying a " +
			"link that cannot be followed")
	}
}

// TestAConfirmationLinkDoesNotSupportTheOtherKind holds the two apart. They ask
// different questions and arrive in different mails, so one must not evidence
// the other.
func TestAConfirmationLinkDoesNotSupportTheOtherKind(t *testing.T) {
	e := setupResolve(t)
	e.issueLinkRow(t, LinkRecordConfirmation, time.Now().Add(72*time.Hour), nil)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryConsentConfirmation})

	if got.Supported {
		t.Error("a record-confirmation link supports a CONSENT confirmation, so an installation " +
			"that asked somebody to check their details may also mail them for an opt-in they " +
			"never asked about")
	}
}
