// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A first business email to a contact known only from captured mail: what lets
// it go, and what keeps it out. The Cc rule is scoped to one thread, a recorded
// fact is read by the composer path, a picked deal supports a follow-up, and a
// sender's unsupported claim is a question rather than a dead end.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// capturedMail plants a message a connector captured on a thread, naming the
// subject in the given role and optionally one of our own seats.
func (e *resolveEnv) capturedMail(t *testing.T, threadKey, subjectRole string, withOurSide bool) ids.UUID {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now() - interval '1 hour', 'gmail', 'connector:gmail')`,
		id, threadKey); err != nil {
		t.Fatalf("planting the captured mail: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, contact_id, address, role)
		VALUES ($1, $2, $3, $4)`, id, e.contact, e.address, subjectRole); err != nil {
		t.Fatalf("planting the subject participant: %v", err)
	}
	if withOurSide {
		if _, err := e.owner.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`,
			id, e.user); err != nil {
			t.Fatalf("planting our participant: %v", err)
		}
	}
	return id
}

func TestACcParticipantOfACapturedThreadMayBeWrittenToAboutThatThread(t *testing.T) {
	for _, role := range []string{"to", "cc"} {
		t.Run(role, func(t *testing.T) {
			e := setupResolve(t)
			e.capturedMail(t, "thread-matter", role, true)

			d := e.decide(t, commsauthz.Request{ThreadKey: "thread-matter"})

			if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryReplyToInbound {
				t.Fatalf("decision = %+v, want an allow as reply_to_inbound", d)
			}
			if d.Basis != commsauthz.BasisLegitimateInterests {
				t.Errorf("basis = %q, want legitimate_interests", d.Basis)
			}
		})
	}
}

func TestACcParticipantStillNeedsAnAnswerForAnUnrelatedTopic(t *testing.T) {
	e := setupResolve(t)
	e.capturedMail(t, "thread-matter", "cc", true)

	got := e.resolve(t, commsauthz.Request{ThreadKey: "another-thread"})

	if got.Supported {
		t.Fatal("being copied on one thread supported mail about another topic")
	}
}

func TestTheCcRuleNeedsOurSideOnTheThreadAndAConnectorCapture(t *testing.T) {
	e := setupResolve(t)
	e.capturedMail(t, "thread-no-us", "cc", false)
	if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-no-us"}); got.Supported {
		t.Error("a thread none of our seats took part in supported a first mail")
	}

	e.activity(t, "thread-typed", "inbound", clocktest.Now(t).Add(-time.Hour), e.address, "cc")
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, user_id, role)
		SELECT id, $1, 'from' FROM activity WHERE thread_key = 'thread-typed'`, e.user); err != nil {
		t.Fatal(err)
	}
	if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-typed"}); got.Supported {
		t.Error("a hand-typed activity manufactured the evidence a connector capture provides")
	}
}

func TestAnArchivedCapturedMailSupportsNothing(t *testing.T) {
	e := setupResolve(t)
	id := e.capturedMail(t, "thread-gone", "cc", true)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET archived_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-gone"}); got.Supported {
		t.Error("an archived mail supported a send")
	}
}

func TestTheCcRuleNeverSupportsAdvertising(t *testing.T) {
	e := setupResolve(t)
	e.capturedMail(t, "thread-matter", "cc", true)

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryMarketing, ThreadKey: "thread-matter"})

	if d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonClaimContradictsResolution {
		t.Fatalf("decision = %+v, want a marketing claim denied as claim_contradicts_resolution", d)
	}
}

func TestARecordedRequestSupportsAComposerFollowUp(t *testing.T) {
	e := setupResolve(t)
	claim := commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}
	if got := e.resolve(t, claim); got.Supported {
		t.Fatal("a follow-up with no evidence was supported")
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_qualifying_event (contact_id, kind, occurred_at, note, source, captured_by)
		VALUES ($1, 'requested_by_subject', now() - interval '2 days', 'asked on the phone', 'human', 'human:x')`,
		e.contact); err != nil {
		t.Fatalf("recording the fact: %v", err)
	}

	if got := e.resolve(t, claim); !got.Supported {
		t.Fatalf("a recorded request did not support the follow-up: %+v", got)
	}
}

func TestAPickedOpenDealSupportsADealFollowUp(t *testing.T) {
	cases := map[string]struct {
		status      string
		stakeholder bool
		want        bool
	}{
		"open and a stakeholder": {"open", true, true},
		"closed":                 {"won", true, false},
		"not a stakeholder":      {"open", false, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			deal := e.openDeal(t, c.status, c.stakeholder)

			got := e.resolve(t, commsauthz.Request{
				Context:  commsauthz.CategoryActiveDealFollowup,
				Evidence: commsauthz.Evidence{DealID: deal},
			})

			if got.Supported != c.want {
				t.Errorf("supported = %v, want %v", got.Supported, c.want)
			}
		})
	}
}

func TestAClaimNamingNoDealIsNotSupportedAsADealFollowUp(t *testing.T) {
	e := setupResolve(t)
	if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryActiveDealFollowup}); got.Supported {
		t.Fatal("a deal follow-up naming no deal was supported")
	}
}

func TestTheStrongerEvidenceWinsOverAMistakenClaim(t *testing.T) {
	e := setupResolve(t)
	deal := e.openDeal(t, "open", true)

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, Links: []ids.UUID{deal}})

	if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryActiveDealFollowup {
		t.Fatalf("decision = %+v, want an allow as active_deal_followup", d)
	}
	if d.Requested != commsauthz.CategoryRequestedFollowup {
		t.Errorf("requested = %q, want the claim kept on the row", d.Requested)
	}
}

func TestAnUnsupportedClaimWithNoPurposeIsAQuestionNotAnUnknownPurpose(t *testing.T) {
	for _, claim := range []commsauthz.Category{commsauthz.CategoryCustomerService, commsauthz.CategoryAccountNotice} {
		t.Run(string(claim), func(t *testing.T) {
			e := setupResolve(t)

			d := e.decide(t, commsauthz.Request{Context: claim})

			if d.Verdict != commsauthz.VerdictReview || d.ReasonCode != commsauthz.ReasonNoEvidence {
				t.Fatalf("decision = %+v, want a review as no_compatible_evidence", d)
			}
		})
	}
}

func TestAPurposeNothingDefinesIsStillUnknown(t *testing.T) {
	e := setupResolve(t)

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryCustomerService, LegacyPurposeKey: "invented"})

	if d.ReasonCode != commsauthz.ReasonUnknownPurpose {
		t.Fatalf("reason = %q, want unknown_purpose for an invented purpose key", d.ReasonCode)
	}
}
