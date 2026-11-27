// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	exitReschedule = "Können wir den Termin auf Donnerstag, 14 Uhr verschieben?"
	exitOffer      = exitReschedule + "\n\nAnbei unser Angebot mit 20 % Rabatt."
)

// refusedSend is the 409 a refused send answers, with the embedded review.
type refusedSend struct {
	Code    string `json:"code"`
	Details struct {
		ReviewID string          `json:"review_id"`
		Review   json.RawMessage `json:"review"`
	} `json:"details"`
}

// replyWith answers a fresh inbound from the subject with body, under the
// correspondence purpose that carries a reply in this fixture.
func (c *consentEnv) replyWith(t *testing.T, body string) (int, refusedSend) {
	t.Helper()
	var refused refusedSend
	status := c.Call(t, "POST", "/v1/activities/"+c.inbound(t)+"/send-email", AnyMap{
		"subject": "Re: Termin", "body": body, "to": []string{"subject@consent.test"},
		"consent_purpose": "business_correspondence",
	}, nil, &refused)
	return status, refused
}

func (c *consentEnv) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := c.Owner.QueryRow(t.Context(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// A reschedule goes out once. The same reply with an offer stops at Send with
// its review in the 409. It goes out once on the sender's own reason, and the
// next offer is decided again.
func TestAnOfferInAReplyStopsAtSendAndGoesOnceOnAReason(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)

	if status, refused := c.replyWith(t, exitReschedule); status != http.StatusAccepted {
		t.Fatalf("a reschedule → %d %q, want 202", status, refused.Code)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Fatalf("%d deliveries after the reschedule, want 1", n)
	}

	status, refused := c.replyWith(t, exitOffer)
	if status != http.StatusConflict || refused.Code != "consent_not_granted" {
		t.Fatalf("an offer → %d %q, want 409 consent_not_granted", status, refused.Code)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Fatalf("%d deliveries after the refused offer, want still 1", n)
	}
	review := c.embeddedReviewMatchesTheGet(t, refused)
	if !review.serves(consent.InlineWarningVersion) || !review.serves(consent.OverrideWarningVersion) {
		t.Fatalf("the sender was served %v, want the modal and the inline warning", review.Warnings)
	}

	body := AnyMap{
		"reason_code": "other", "explanation": "They asked for these terms on the phone today.",
		"warning_version": consent.InlineWarningVersion, "acknowledged": true,
		"assessment_revision": review.AssessmentRevision, "shown_reasons": review.reasons(),
	}
	path := "/v1/communication-reviews/" + review.ID + "/direct-send"
	colleague := c.colleague(t)
	var theirs servedReview
	if status := colleague.Call(t, "GET", "/v1/communication-reviews/"+review.ID, nil, nil, &theirs); status != http.StatusOK {
		t.Fatalf("a colleague holding the exception grant reading the review → %d, want 200", status)
	}
	if theirs.serves(consent.InlineWarningVersion) {
		t.Fatal("a colleague was served the composer's warning they are never shown")
	}
	if status := colleague.Call(t, "POST", path, body, nil, nil); status != http.StatusForbidden {
		t.Fatalf("a colleague sending the author's message → %d, want 403", status)
	}
	if status := c.Call(t, "POST", path, body, nil, nil); status != http.StatusCreated {
		t.Fatalf("sending on the reason → %d, want 201", status)
	}
	var recorded string
	if err := c.Owner.QueryRow(t.Context(), `SELECT warning_version FROM communication_instruction`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != consent.InlineWarningVersion {
		t.Fatalf("the instruction records warning %q, want %q", recorded, consent.InlineWarningVersion)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 2 {
		t.Fatalf("%d deliveries after the directed send, want 2", n)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_instruction WHERE status = 'consumed'`); n != 1 {
		t.Fatalf("%d consumed instructions, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_review WHERE resolved_at IS NULL`); n != 0 {
		t.Fatalf("%d open reviews after the directed send, want 0", n)
	}

	if status, _ := c.replyWith(t, exitOffer); status != http.StatusConflict {
		t.Fatalf("the next offer → %d, want 409: the reason covered one message", status)
	}
	if n := c.count(t, `SELECT count(*) FROM comms_outbound`); n != 2 {
		t.Fatalf("%d deliveries after the next offer, want still 2", n)
	}
	if n := c.count(t, `SELECT count(*) FROM contact_consent`); n != 0 {
		t.Fatalf("%d consent rows after the directed send, want 0", n)
	}
}

// servedReview is the part of a served review the exit test reads.
type servedReview struct {
	ID                 string `json:"id"`
	AssessmentRevision string `json:"assessment_revision"`
	Refusals           []struct {
		ReasonCode string `json:"reason_code"`
	} `json:"refusals"`
	Warnings []servedWarning `json:"warnings"`
}

type servedWarning struct {
	Version string `json:"version"`
}

func (r servedReview) serves(version string) bool {
	return slices.Contains(r.Warnings, servedWarning{Version: version})
}

// colleague is a second admin, who holds the exception grant and did not send.
func (c *consentEnv) colleague(t *testing.T) *apptest.AppEnv {
	t.Helper()
	const email, password = "colleague@consent.test", "colleague-consent-password"
	c.DescribeCompany(t)
	var created struct {
		ID     string `json:"id"`
		Detail string `json:"detail"`
	}
	if status := c.Call(t, "POST", "/v1/users", AnyMap{"email": email, "display_name": "Colleague", "role": "admin"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("adding the colleague → %d %s", status, created.Detail)
	}
	id := created.ID
	apptest.SetPasswordByLink(t, c.AppEnv, id, password)
	return c.AsSecondSeat(t, email, password)
}

func (r servedReview) reasons() []string {
	out := make([]string, 0, len(r.Refusals))
	for _, refusal := range r.Refusals {
		out = append(out, refusal.ReasonCode)
	}
	return out
}

// embeddedReviewMatchesTheGet holds the 409's review to the GET of the same
// review by the same caller, byte for byte once both are decoded.
func (c *consentEnv) embeddedReviewMatchesTheGet(t *testing.T, refused refusedSend) servedReview {
	t.Helper()
	if len(refused.Details.Review) == 0 {
		t.Fatal("the 409 carries no review")
	}
	var fetched json.RawMessage
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+refused.Details.ReviewID, nil, nil, &fetched); status != http.StatusOK {
		t.Fatalf("reading the review → %d", status)
	}
	var embedded, got crmcontracts.CommunicationReview
	if err := json.Unmarshal(refused.Details.Review, &embedded); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fetched, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(embedded, got) {
		t.Fatalf("the embedded review differs from the GET:\n409: %s\nGET: %s", refused.Details.Review, fetched)
	}
	var review servedReview
	if err := json.Unmarshal(fetched, &review); err != nil {
		t.Fatal(err)
	}
	if review.ID != refused.Details.ReviewID || len(review.Refusals) == 0 {
		t.Fatalf("served review %+v does not name the refusal %s", review, refused.Details.ReviewID)
	}
	return review
}

// capturedInbound plants mail the contact sent, as a mail connector captures
// it: their own message in its own thread, filed under them.
func (c *consentEnv) capturedInbound(t *testing.T, subject, body string) string {
	t.Helper()
	id := ids.NewV7()
	if _, err := c.Owner.Exec(t.Context(), `
		INSERT INTO activity (id, kind, direction, subject, body, thread_key, occurred_at, source, captured_by, inbound_sender_authenticated)
		VALUES (@id, 'email', 'inbound', @subject, @body, @thread, @at, 'gmail', 'connector:gmail', true)`,
		pgx.StrictNamedArgs{"id": id, "subject": subject, "body": body, "thread": "thread-" + id.String(), "at": clocktest.Now(t).Add(-time.Hour)}); err != nil {
		t.Fatalf("planting the inbound: %v", err)
	}
	if _, err := c.Owner.Exec(t.Context(), `
		INSERT INTO activity_participant (activity_id, contact_id, address, address_domain_folded, role)
		VALUES (@id, @contact, 'subject@consent.test', 'consent.test', 'from')`,
		pgx.StrictNamedArgs{"id": id, "contact": c.contactID}); err != nil {
		t.Fatalf("planting the inbound's sender: %v", err)
	}
	if _, err := c.Owner.Exec(t.Context(), `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES (@id, 'contact', @contact)`,
		pgx.StrictNamedArgs{"id": id, "contact": c.contactID}); err != nil {
		t.Fatalf("filing the inbound: %v", err)
	}
	return id.String()
}

// sentInto plants the rep's own mail into the thread of a captured inbound.
func (c *consentEnv) sentInto(t *testing.T, inbound, subject string) string {
	t.Helper()
	id := ids.NewV7()
	if _, err := c.Owner.Exec(t.Context(), `
		INSERT INTO activity (id, kind, direction, subject, body, thread_key, occurred_at, source, captured_by)
		SELECT @id, 'email', 'outbound', @subject, 'Anbei.', thread_key, @at, 'gmail', 'connector:gmail'
		  FROM activity WHERE id = @inbound`,
		pgx.StrictNamedArgs{"id": id, "subject": subject, "inbound": inbound, "at": clocktest.Now(t).Add(-time.Minute)}); err != nil {
		t.Fatalf("planting the rep's mail: %v", err)
	}
	if _, err := c.Owner.Exec(t.Context(), `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES (@id, 'contact', @contact)`,
		pgx.StrictNamedArgs{"id": id, "contact": c.contactID}); err != nil {
		t.Fatalf("filing the rep's mail: %v", err)
	}
	return id.String()
}

// loggedBy writes an activity into the record by hand, as a rep logs one.
func (c *consentEnv) loggedBy(t *testing.T, direction, subject string) string {
	t.Helper()
	var logged struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "email", "subject": subject, "direction": direction,
		"links": []AnyMap{{"entity_type": "contact", "entity_id": c.contactID}},
	}, nil, &logged); status != http.StatusCreated {
		t.Fatalf("logging the %s mail → %d", direction, status)
	}
	return logged.ID
}

// answerWith replies to anchor with subject and body.
func (c *consentEnv) answerWith(t *testing.T, anchor, subject, body string) int {
	t.Helper()
	return c.Call(t, "POST", "/v1/activities/"+anchor+"/send-email", AnyMap{
		"subject": subject, "body": body, "to": []string{"subject@consent.test"},
		"consent_purpose": "business_correspondence",
	}, nil, nil)
}

// A pitch the contact wrote stays theirs, in the body and in the carried-over
// subject. A reschedule answering it queues on the first click. A rep who
// quotes the pitch back, or puts an offer in the subject, has written an offer.
func TestAQuotedPitchCountsOnlyWhenTheRepTypesItBack(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	const pitch = "Unser Angebot: 20 % Rabatt auf Ihre Lizenzen."
	answer := func(subject, body string) int {
		return c.answerWith(t, c.capturedInbound(t, "Termin und Angebot", pitch), subject, body)
	}

	if status := answer("Re: Termin und Angebot", exitReschedule); status != http.StatusAccepted {
		t.Fatalf("a reschedule answering a pitch → %d, want 202 on the first click", status)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_review`); n != 0 {
		t.Fatalf("%d reviews after the reschedule, want none", n)
	}
	if status := answer("Re: Termin und Angebot", exitReschedule+"\n\n> "+pitch); status != http.StatusConflict {
		t.Fatalf("a reply quoting the pitch back → %d, want 409", status)
	}
	if status := answer("Re: Termin und Angebot mit 20 % Rabatt", exitReschedule); status != http.StatusConflict {
		t.Fatalf("a reply whose subject the rep turned into an offer → %d, want 409", status)
	}
}

// A subject is quoted only when the contact wrote it. A subject carried over
// from the rep's own pitch, from mail logged by hand or from a sender that
// failed authentication is the rep's.
// Each anchor first answers a plain subject, so the fixture shows the cause.
func TestASubjectTheContactDidNotWriteIsTheReps(t *testing.T) {
	t.Parallel()
	for name, anchor := range map[string]func(c *consentEnv, subject string) string{
		"the rep's own pitch in the contact's thread": func(c *consentEnv, subject string) string {
			return c.sentInto(t, c.capturedInbound(t, "Termin", exitReschedule), subject)
		},
		"a hand-logged inbound": func(c *consentEnv, subject string) string { return c.loggedBy(t, "inbound", subject) },
		"an inbound whose sender failed authentication": func(c *consentEnv, subject string) string {
			anchor := c.capturedInbound(t, subject, exitReschedule)
			if _, err := c.Owner.Exec(t.Context(), `UPDATE activity SET inbound_sender_authenticated = false WHERE id = $1`, anchor); err != nil {
				t.Fatal(err)
			}
			return anchor
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := setupConsent(t)
			c.useReviewableMarketingProfile(t)
			if status := c.answerWith(t, anchor(c, "Termin"), "Re: Termin", exitReschedule); status != http.StatusAccepted {
				t.Fatalf("a plain reply to this anchor → %d, want 202: the fixture proves nothing", status)
			}
			if status := c.answerWith(t, anchor(c, "Termin und Angebot"), "Re: Termin und Angebot", exitReschedule); status != http.StatusConflict {
				t.Fatalf("a reply carrying over a subject the contact did not write → %d, want 409", status)
			}
		})
	}
}
