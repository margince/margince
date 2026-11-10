// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What lets the INSTALLATION write to somebody about its own obligations.
//
// The evidence is a live confirm_token, and these cases pin both halves of that:
// a message carrying one is supported, and the claim on its own supports
// nothing. The second half is what keeps the lane from becoming a way to reach
// a contact who has objected — the five subject-serving categories pass a hard
// suppression, so a category that could be claimed without evidence would be a
// route around every refusal the engine makes.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
			"never be sent to the contact who has not consented yet", got.Basis)
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

// plantNarrowObjection writes a contact's marketing_objection scoped to ONE
// consent_purpose, bypassing every writer this package has today.
//
// No writer gives a CONTACT a narrow stop yet — only a LEAD gets one, through
// StopForCredentialTx's named-purpose press (withdrawalpress.go). This is the
// same planted-row pattern lift_integration_test.go's plantSuppression and
// reviewcontext's fixtures already use to reach a state no door produces: the
// row shape communication_suppression.purpose_id defines is real regardless of
// which writer will eventually mint one for a contact, and the reader under
// test — validateOptOutAcknowledgement — must answer it correctly today.
func plantNarrowObjection(t *testing.T, e *resolveEnv, contact ids.ContactID) {
	t.Helper()
	purpose := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (id, key, label, requires_double_opt_in)
		VALUES ($1, 'narrow-newsletter', 'Narrow Newsletter', false)`, purpose); err != nil {
		t.Fatalf("seeding the purpose a narrow stop names: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression
		    (contact_id, purpose_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, $2, $3, 'test', 'human:x', $4)`,
		contact, purpose, commsauthz.ReasonObjection, string(commsauthz.LevelSubject)); err != nil {
		t.Fatalf("planting the narrow objection: %v", err)
	}
}

// TestANarrowObjectionStillOwesTheSubjectLevelAcknowledgement pins the
// EXISTS's deliberate blindness to purpose_id: a marketing_objection scoped to
// one newsletter is still a marketing_objection, and Decree 91 Art. 16 owes an
// acknowledgement of THAT REFUSAL regardless of which newsletter it named. A
// reader that started matching on purpose (rowPurpose equals the send's
// resolved purpose, the rule applySuppression uses to BIND a send) would
// answer unsupported here — refusing the confirmation the subject is owed
// because it asked the wrong question of the wrong kind of row.
func TestANarrowObjectionStillOwesTheSubjectLevelAcknowledgement(t *testing.T) {
	e := setupResolve(t)
	plantNarrowObjection(t, e, e.contact)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryOptoutConfirmation})

	if !got.Supported {
		t.Fatalf("a narrow-purpose objection resolved unsupported (%q) — the acknowledgement is "+
			"subject-level and purpose does not narrow it", got.Reason)
	}
	if got.Basis != commsauthz.BasisLegalObligation {
		t.Errorf("basis %q, want legal_obligation", got.Basis)
	}
}

// TestNoLiveStopOwesNoAcknowledgement is the other direction: a contact who
// never objected is owed no confirmation that a refusal was received, because
// none was. Held beside the narrow-stop case so a reader that answered
// unconditionally true — the failure mode on the OTHER side of the same
// mistake — is caught here instead of by a contact receiving mail about a
// refusal they never made.
func TestNoLiveStopOwesNoAcknowledgement(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryOptoutConfirmation})

	if got.Supported {
		t.Error("a contact with no live stop resolved SUPPORTED for an opt-out acknowledgement — " +
			"there is no refusal on record for this message to confirm")
	}
}

// withdrawThroughThePress seeds a purpose of the given class and withdraws it
// the way an unsubscribe press does, which writes no suppression.
func withdrawThroughThePress(t *testing.T, e *resolveEnv, key string, class Class) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (key, label, class) VALUES ($1, $1, $2)`, key, string(class)); err != nil {
		t.Fatalf("seeding the purpose: %v", err)
	}
	writer := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	withdrawn, err := e.store.PublicWithdrawAll(writer, e.contact, []string{key})
	if err != nil || len(withdrawn) != 1 {
		t.Fatalf("withdrawing %q answered %v, %v", key, withdrawn, err)
	}
}

// TestAWithdrawnMarketingPurposeOwesAnAcknowledgement. An unsubscribe press
// withdraws a marketing purpose and records no suppression, and Decree 91
// Art. 16 owes it an acknowledgement all the same.
func TestAWithdrawnMarketingPurposeOwesAnAcknowledgement(t *testing.T) {
	e := setupResolve(t)
	withdrawThroughThePress(t, e, "press-newsletter", ClassMarketing)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryOptoutConfirmation})

	if !got.Supported {
		t.Fatalf("a withdrawn marketing purpose resolved unsupported (%q)", got.Reason)
	}
}

// TestAWithdrawnCorrespondencePurposeOwesNoAcknowledgement. Replies to the
// contact's own enquiries are not advertising, so withdrawing them is no
// refusal the acknowledgement could confirm.
func TestAWithdrawnCorrespondencePurposeOwesNoAcknowledgement(t *testing.T) {
	e := setupResolve(t)
	withdrawThroughThePress(t, e, "press-replies", ClassBusinessCorrespondence)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryOptoutConfirmation})

	if got.Supported {
		t.Error("a withdrawn correspondence purpose resolved SUPPORTED for an opt-out acknowledgement")
	}
}

// resolveForLead asks the same question about a lead holding the address.
func (e *resolveEnv) resolveForLead(t *testing.T, lead ids.UUID) resolution {
	t.Helper()
	var out resolution
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = e.gate.resolveCategory(e.ctx, tx,
			commsauthz.Request{Context: commsauthz.CategoryOptoutConfirmation},
			subjectRef{Kind: entityLead, ID: lead.String(), Address: "lead@buyer.test"})
		return err
	}); err != nil {
		t.Fatalf("resolving for the lead: %v", err)
	}
	return out
}

// plantLeadStop seeds a lead and a suppression on it of the given kind.
func plantLeadStop(t *testing.T, e *resolveEnv, kind string, revoked bool) ids.UUID {
	t.Helper()
	lead := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO lead (id, full_name, email, source, captured_by)
		VALUES ($1, 'Lead Refusal', 'lead@buyer.test', 'manual', 'human:x')`, lead); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	if kind == "" {
		return lead
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (lead_id, kind, source, captured_by, decided_by_level, revoked_at)
		VALUES ($1, $2, 'test', 'human:x', $3, CASE WHEN $4 THEN now() END)`,
		lead, kind, string(commsauthz.LevelSubject), revoked); err != nil {
		t.Fatalf("planting the lead's stop: %v", err)
	}
	return lead
}

// A lead's live objection is the evidence its receipt rests on.
func TestALeadsLiveObjectionOwesAnAcknowledgement(t *testing.T) {
	e := setupResolve(t)
	lead := plantLeadStop(t, e, commsauthz.ReasonObjection, false)

	got := e.resolveForLead(t, lead)

	if !got.Supported || got.Basis != commsauthz.BasisLegalObligation {
		t.Fatalf("a lead's live objection resolved %+v, want supported on a legal obligation", got)
	}
}

// A lead with no stop, a lifted one, or one that refused nothing is owed none.
func TestALeadWithNoStandingRefusalOwesNoAcknowledgement(t *testing.T) {
	for name, plant := range map[string]struct {
		kind    string
		revoked bool
	}{
		"no stop":     {kind: ""},
		"lifted stop": {kind: commsauthz.ReasonObjection, revoked: true},
		"hard bounce": {kind: commsauthz.ReasonHardBounce},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			lead := plantLeadStop(t, e, plant.kind, plant.revoked)
			if got := e.resolveForLead(t, lead); got.Supported {
				t.Errorf("%s resolved supported for a lead's opt-out acknowledgement", name)
			}
		})
	}
}
