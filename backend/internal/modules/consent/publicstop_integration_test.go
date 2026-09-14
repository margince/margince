// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The subject's own route to stopping more than an unsubscribe stops.
//
// The press used to do both jobs and did them badly: it swept every purpose in
// the catalog, so it ended business correspondence nobody had subscribed to,
// and it recorded the act as a WITHDRAWAL of a consent that was never the basis
// for those messages. Narrowing the press without building this route would
// have left a subject with no way to say "stop entirely".

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// standingStops reads the live suppression kinds for a subject.
func standingStops(t *testing.T, e *channelConsentEnv) map[string]string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT kind, decided_by_level FROM communication_suppression
		 WHERE contact_id = $1 AND revoked_at IS NULL`, e.contact)
	if err != nil {
		t.Fatalf("reading the standing stops: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, level string
		if err := rows.Scan(&kind, &level); err != nil {
			t.Fatalf("scanning a stop: %v", err)
		}
		out[kind] = level
	}
	return out
}

// TestStopAllContactRecordsAnObjectionTheSubjectOwns.
//
// AT THE SUBJECT'S LEVEL, whatever the caller is. The anonymous preference edge
// binds a system principal with no grants, so authorityOf would answer
// LevelUser and any seat could then lift what the subject themselves recorded.
func TestStopAllContactRecordsAnObjectionTheSubjectOwns(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)

	got, err := e.store.PublicStop(e.ctx, e.contact, StopAllContact, "Please stop writing to me.")
	if err != nil {
		t.Fatalf("the subject's stop failed: %v", err)
	}
	if !got.Recorded {
		t.Error("a first press reported nothing recorded")
	}
	if got.ReceiptReference == "" {
		t.Error("the subject was given no receipt to quote")
	}

	stops := standingStops(t, e)
	level, standing := stops["subject_request"]
	if !standing {
		t.Fatalf("no subject_request stop stands: %v", stops)
	}
	if level != string(commsauthz.LevelSubject) {
		t.Errorf("the stop was recorded at level %q, want %q — at anything less a seat "+
			"could lift what the subject themselves asked for",
			level, commsauthz.LevelSubject)
	}
}

// TestStopAllMarketingIsAnObjectionAndNotAWithdrawal.
//
// The two are different legal acts. A withdrawal says the consent is gone; an
// objection says the processing must stop whether or not consent was ever its
// basis. Recording the second as the first is what the old sweep did.
func TestStopAllMarketingIsAnObjectionAndNotAWithdrawal(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)

	if _, err := e.store.PublicStop(e.ctx, e.contact, StopAllMarketing, "No more ads."); err != nil {
		t.Fatalf("the subject's objection failed: %v", err)
	}

	stops := standingStops(t, e)
	if _, standing := stops[commsauthz.ReasonObjection]; !standing {
		t.Errorf("no marketing objection stands: %v", stops)
	}
	if _, standing := stops["subject_request"]; standing {
		t.Error("an objection to marketing recorded a broad subject request — that binds " +
			"invoices, which the subject did not ask to stop")
	}
}

// TestAReplayRecordsNothingNewAndAnswersTheSameReceipt.
//
// A subject pressing the same link twice is one occasion and a stuck browser.
// The seat door beside this one records both deliberately, because a rep
// relaying a second phone call IS a second occasion and the reason may differ.
func TestAReplayRecordsNothingNewAndAnswersTheSameReceipt(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)

	first, err := e.store.PublicStop(e.ctx, e.contact, StopAllContact, "Stop.")
	if err != nil {
		t.Fatalf("the first press failed: %v", err)
	}
	second, err := e.store.PublicStop(e.ctx, e.contact, StopAllContact, "Stop.")
	if err != nil {
		t.Fatalf("the replay failed: %v", err)
	}
	if second.Recorded {
		t.Error("a replay reported a fresh record — the page would show a new confirmation " +
			"for a no-op")
	}
	if second.ReceiptReference != first.ReceiptReference {
		t.Errorf("the replay answered receipt %q and the first press %q — the subject who "+
			"pressed twice holds two references for one request",
			second.ReceiptReference, first.ReceiptReference)
	}
}

// TestAnUnrecognisedActionIsRefusedRatherThanDefaulted.
//
// Defaulting to the narrower kind would under-record a subject who asked for
// everything to stop; defaulting to the broader one would stop invoices nobody
// asked to stop. Neither is a safe direction, so there is no default.
func TestAnUnrecognisedActionIsRefusedRatherThanDefaulted(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)

	if _, err := e.store.PublicStop(e.ctx, e.contact, PublicStopAction("stop_some_of_it"), ""); err == nil {
		t.Fatal("an unrecognised action was accepted")
	}
	if stops := standingStops(t, e); len(stops) != 0 {
		t.Errorf("a refused action still recorded %v", stops)
	}
}

// TestAnUnsubscribeWritesNoStopThatBindsCorrespondence is the exit criterion
// for narrowing the sweep, asked from the other end.
//
// The scope test beside it checks WHICH purposes the press withdrew. This
// checks what the press left behind in the suppression table, because a
// withdrawal that wrote a broad stop as a side effect would pass the first test
// and still end the subject's replies.
//
// Mutation: point the one-click handler back at a sweep over every class and
// this stays green — which is why both tests exist. Write a subject_request
// from the unsubscribe path and this fails.
func TestAnUnsubscribeWritesNoStopThatBindsCorrespondence(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO consent_purpose (key, label, class) VALUES ($1, $1, 'marketing')
		 ON CONFLICT (key) DO UPDATE SET class = EXCLUDED.class`, "newsletter_blast"); err != nil {
		t.Fatalf("seeding the marketing purpose: %v", err)
	}

	if _, err := e.store.PublicStopAllMarketing(e.ctx, e.contact); err != nil {
		t.Fatalf("the unsubscribe failed: %v", err)
	}

	// An unsubscribe is a WITHDRAWAL of consent. It records no suppression at
	// all: the subject did not object, they stopped subscribing, and those are
	// different acts with different reach.
	if stops := standingStops(t, e); len(stops) != 0 {
		t.Errorf("an unsubscribe wrote %v. A subject_request there would bind the replies "+
			"to the subject's own enquiries, which is the defect this slice closes; any "+
			"stop at all overstates what pressing unsubscribe said", stops)
	}
}

// TestARepsWeakerStopDoesNotSwallowTheSubjectsOwn is Codex's finding, and the
// consequence is that the mail resumes.
//
// A rep relaying a phone call writes a subject_request at LevelUser, which an
// admin may lift. A replay check asking only "does a subject_request stand"
// answers yes to the subject's own press, records nothing, and hands them a
// receipt — after which an admin lifts the rep's row and nothing the subject
// did is on the record.
func TestARepsWeakerStopDoesNotSwallowTheSubjectsOwn(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)

	// The rep's row, as the seat door writes it.
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'test', 'human:rep', $2)`,
		e.contact, string(commsauthz.LevelUser)); err != nil {
		t.Fatalf("planting the rep's stop: %v", err)
	}

	got, err := e.store.PublicStop(e.ctx, e.contact, StopAllContact, "Stop, please.")
	if err != nil {
		t.Fatalf("the subject's stop failed: %v", err)
	}
	if !got.Recorded {
		t.Fatal("the subject's press recorded nothing because a rep had already written a " +
			"weaker row — an admin lifting that row leaves the subject's decision nowhere")
	}
	if level := standingStops(t, e)["subject_request"]; level != string(commsauthz.LevelSubject) {
		t.Errorf("the strongest standing subject_request is at level %q, want %q",
			level, commsauthz.LevelSubject)
	}
}

// pressStop drives the real handler, which is where the credential's scope is
// enforced. The store method below it trusts its caller.
func pressStop(t *testing.T, e *channelConsentEnv, token, action string) int {
	t.Helper()
	h := NewHandlers(database.BindTo(e.store.db.Pool(), ids.From[ids.WorkspaceKind](e.ws)))
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   "system:public_preferences",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/public/preferences/"+token+"/stop",
		strings.NewReader(`{"action":"`+action+`"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.PublicStopContact(rec, req, token)
	return rec.Code
}

// TestANamedSubscriptionLinkCannotStopEverything is Codex's finding, and it was
// a privilege escalation of my own making.
//
// A named_purpose credential is minted to stop ONE subscription. My first
// resolver read only the contact off it and threw the scope away, so a
// forwarded newsletter link could POST stop_all_contact and end that contact's
// invoices and their replies — at the subject's own authority, which no seat
// can lift.
//
// NEITHER action is refused for being large. Both are refused because neither
// is as narrow as the link: the smaller one objects to all direct marketing,
// and the link speaks for one subscription.
func TestANamedSubscriptionLinkCannotStopEverything(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)
	address := "subject-" + e.contact.String() + "@example.test"

	named := mintWithdrawal(t, e, WithdrawalMintInput{
		Address:   address,
		ContactID: e.contact,
		Scope:     WithdrawalScopeNamedPurpose,
		PurposeID: e.newsletter.UUID,
	})
	if named == "" {
		t.Fatal("the mint returned no token")
	}

	for _, action := range []string{"stop_all_contact", "stop_all_marketing"} {
		if code := pressStop(t, e, named, action); code == http.StatusOK {
			t.Errorf("a named-subscription link performed %q — a forwarded newsletter link "+
				"would end this contact's invoices and their replies", action)
		}
	}
	if stops := standingStops(t, e); len(stops) != 0 {
		t.Errorf("a refused press still recorded %v", stops)
	}
}

// TestAnAllMarketingLinkMayStillObject is the positive control: the refusal
// above must be about the link's scope and not about the route being broken.
func TestAnAllMarketingLinkMayStillObject(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)
	address := "subject-" + e.contact.String() + "@example.test"

	broad := mintWithdrawal(t, e, WithdrawalMintInput{
		Address:   address,
		ContactID: e.contact,
		Scope:     WithdrawalScopeAllMarketing,
	})
	if code := pressStop(t, e, broad, "stop_all_marketing"); code != http.StatusOK {
		t.Fatalf("an all-marketing link was refused its own scope: status %d", code)
	}
	if _, standing := standingStops(t, e)[commsauthz.ReasonObjection]; !standing {
		t.Error("the press recorded no marketing objection")
	}
}
