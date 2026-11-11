// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func approvedHeldWording(t *testing.T) (*consentEnv, string, crmcontracts.WordingCorrectionContext) {
	t.Helper()
	return approvedHeldWordingFor(t, false)
}

func approvedHeldWordingFor(t *testing.T, anotherDirector bool) (*consentEnv, string, crmcontracts.WordingCorrectionContext) {
	t.Helper()
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Requested information", "body": "The requested informaton.",
		"html_body": "<p>The requested informaton.</p>",
		"to":        []string{"subject@consent.test"}, "consent_purpose": "marketing_email",
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("initial send=%d", status)
	}
	review := liveReviewID(t, c)
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress", AnyMap{
		"kind": "marketing_objection", "reason": "A colleague recorded an objection.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording a stop=%d", status)
	}
	director := c.AppEnv
	if anotherDirector {
		c.DescribeCompany(t)
		var invited struct {
			ID string `json:"id"`
		}
		if status := c.Call(t, "POST", "/v1/users", AnyMap{
			"email": "separate-director@consent.test", "display_name": "Separate director", "role": "admin",
		}, nil, &invited); status != http.StatusCreated {
			t.Fatalf("creating director=%d", status)
		}
		const password = "Test-separate-director-password-23!"
		apptest.SetPasswordByLink(t, c.AppEnv, invited.ID, password)
		director = c.AsSecondSeat(t, "separate-director@consent.test", password)
	}
	if status := director.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status < 400 || status >= 500 {
		t.Fatalf("stopped send=%d", status)
	}
	var stop string
	if err := c.Owner.QueryRow(t.Context(), `SELECT id FROM communication_suppression WHERE contact_id=@contact AND revoked_at IS NULL`,
		pgx.StrictNamedArgs{"contact": c.contactID}).Scan(&stop); err != nil {
		t.Fatal(err)
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress/"+stop+"/lift", AnyMap{
		"reason": "The colleague recorded this objection by mistake.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("correcting the mistaken stop=%d", status)
	}
	if anotherDirector {
		c.AppEnv = director
		var hidden crmcontracts.CommunicationReview
		if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &hidden); status != http.StatusOK || hidden.WordingCorrection != nil {
			t.Fatalf("non-owner correction preview=%d %+v", status, hidden.WordingCorrection)
		}
		var observed crmcontracts.WordingCorrectionContext
		if err := c.Owner.QueryRow(t.Context(), `SELECT i.id,s.version,encode(i.acknowledged_wording,'hex'),
			s.payload->>'subject',s.payload->>'body',s.payload->>'html_body'
			FROM communication_instruction i JOIN communication_review r ON r.id=i.review_id
			JOIN scheduled_send s ON s.id=r.delivery_intent_id WHERE r.id=@review`,
			pgx.StrictNamedArgs{"review": review}).Scan(&observed.InstructionId, &observed.Version, &observed.WordingDigest,
			&observed.Subject, &observed.Body, &observed.HtmlBody); err != nil {
			t.Fatal(err)
		}
		return c, review, observed
	}
	var current crmcontracts.CommunicationReview
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &current); status != http.StatusOK || current.WordingCorrection == nil {
		t.Fatalf("approved wording unavailable: status=%d review=%+v", status, current)
	}
	return c, review, *current.WordingCorrection
}

func spellingCorrection(current crmcontracts.WordingCorrectionContext) AnyMap {
	return AnyMap{
		"version": current.Version, "expected_wording": current.WordingDigest,
		"subject": current.Subject, "body": "The requested information.",
		"html_body": "<p>The requested information.</p>", "meaning_unchanged": true,
	}
}

func TestTheOriginalApprovalKeepsAnAttestedSpellingCorrection(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	path := "/v1/communication-reviews/" + review + "/correct-wording"
	var result crmcontracts.WordingCorrectionResult
	if status := c.Call(t, "POST", path, spellingCorrection(original), nil, &result); status != http.StatusOK || result.Version != original.Version+1 {
		t.Fatalf("correction=%d result=%+v", status, result)
	}
	if status := c.Call(t, "POST", path, spellingCorrection(original), nil, nil); status != http.StatusConflict {
		t.Fatalf("stale correction=%d", status)
	}
	var current crmcontracts.CommunicationReview
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &current); status != http.StatusOK || current.WordingCorrection == nil {
		t.Fatalf("corrected review=%d", status)
	}
	if current.WordingCorrection.InstructionId != original.InstructionId || current.WordingCorrection.Body != "The requested information." || current.WordingCorrection.HtmlBody != "<p>The requested information.</p>" {
		t.Fatalf("the correction replaced the approval or lost the words: %+v", current.WordingCorrection)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("sending the corrected wording=%d", status)
	}
	var count, audits, events int
	var state, body, acknowledged string
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*), min(status), min(encode(acknowledged_wording,'hex'))
		FROM communication_instruction WHERE review_id=@review`,
		pgx.StrictNamedArgs{"review": review}).Scan(&count, &state, &acknowledged); err != nil {
		t.Fatal(err)
	}
	if err := c.Owner.QueryRow(t.Context(), `SELECT o.body FROM comms_outbound o JOIN scheduled_send s ON s.delivery_id=o.id
		JOIN communication_review r ON r.delivery_intent_id=s.id WHERE r.id=@review`,
		pgx.StrictNamedArgs{"review": review}).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if err := c.Owner.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM audit_log WHERE entity_id=@instruction AND action='update'
		 AND evidence->>'correction_kind'='spelling_punctuation'),
		(SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=@instruction::uuid::text
		 AND envelope->>'type'='communication_instruction.updated')`,
		pgx.StrictNamedArgs{"instruction": original.InstructionId}).Scan(&audits, &events); err != nil {
		t.Fatal(err)
	}
	if count != 1 || state != "consumed" || !strings.HasPrefix(body, "The requested information.\n\n") || audits != 1 || events != 1 || acknowledged != original.WordingDigest {
		t.Fatalf("instructions=%d state=%s corrected_delivery=%t audits=%d events=%d", count, state, strings.HasPrefix(body, "The requested information.\n\n"), audits, events)
	}
}

func TestAChangedEnvelopeCannotRetainItsWordingApproval(t *testing.T) {
	for _, field := range []string{"recipients", "cc", "bcc", "sending_entity", "context", "marketing_purpose", "attachment_ids", "future_scope_field"} {
		t.Run(field, func(t *testing.T) {
			c, review, original := approvedHeldWording(t)
			if _, err := c.Owner.Exec(t.Context(), `UPDATE scheduled_send
				SET payload=jsonb_set(payload,ARRAY[@field], '"changed"'::jsonb)
				WHERE id=(SELECT delivery_intent_id FROM communication_review WHERE id=@review)`,
				pgx.StrictNamedArgs{"field": field, "review": review}); err != nil {
				t.Fatal(err)
			}
			if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", spellingCorrection(original), nil, nil); status != http.StatusUnprocessableEntity {
				t.Fatalf("changed %s correction=%d", field, status)
			}
			if field == "future_scope_field" {
				if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusUnprocessableEntity {
					t.Fatalf("changed future scope send=%d", status)
				}
			}
			var corrected bool
			if err := c.Owner.QueryRow(t.Context(), `SELECT corrected_wording IS NOT NULL FROM communication_instruction WHERE review_id=@review`,
				pgx.StrictNamedArgs{"review": review}).Scan(&corrected); err != nil {
				t.Fatal(err)
			}
			if corrected {
				t.Fatal("a changed envelope retained the approval")
			}
		})
	}
}

func TestACorrectionNeedsAnExplicitMeaningUnchangedAttestation(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	request := spellingCorrection(original)
	request["meaning_unchanged"] = false
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", request, nil, nil); status != http.StatusBadRequest {
		t.Fatalf("unattested correction=%d", status)
	}
}

func TestAnotherApproverCannotCorrectSomebodyElsesDecision(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	c.DescribeCompany(t)
	var invited struct {
		ID                  string `json:"id"`
		Title, Detail, Code string
	}
	if status := c.Call(t, "POST", "/v1/users", AnyMap{
		"email": "other-approver@consent.test", "display_name": "Another approver", "role": "admin",
	}, nil, &invited); status != http.StatusCreated {
		t.Fatalf("inviting another approver=%d %+v", status, invited)
	}
	const password = "Test-approver-password-23!"
	apptest.SetPasswordByLink(t, c.AppEnv, invited.ID, password)
	other := c.AsSecondSeat(t, "other-approver@consent.test", password)
	var read crmcontracts.CommunicationReview
	if status := other.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &read); status != http.StatusOK || read.WordingCorrection != nil {
		t.Fatalf("another approver's correction preview=%d %+v", status, read.WordingCorrection)
	}
	if status := other.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", spellingCorrection(original), nil, nil); status != http.StatusNotFound {
		t.Fatalf("another approver's correction=%d", status)
	}
}

func TestAReadSeatCanReadItsReviewButCannotRetainACorrection(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	c.SetWorkspaceSeat(t, "read")
	t.Cleanup(func() { c.SetWorkspaceSeat(t, "full") })
	var read crmcontracts.CommunicationReview
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &read); status != http.StatusOK || read.WordingCorrection != nil {
		t.Fatalf("read-seat review=%d correction=%+v", status, read.WordingCorrection)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", spellingCorrection(original), nil, nil); status != http.StatusForbidden {
		t.Fatalf("read-seat correction=%d", status)
	}
}

func TestUnapprovedWordingChangesStillNeedANewDecision(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	if _, err := c.Owner.Exec(t.Context(), `UPDATE scheduled_send
		SET payload=jsonb_set(payload,'{body}','"The fee has increased."'::jsonb)
		WHERE id=(SELECT delivery_intent_id FROM communication_review WHERE id=@review)`,
		pgx.StrictNamedArgs{"review": review}); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"correct-wording", "direct-send"} {
		request := spellingCorrection(original)
		if operation == "direct-send" {
			request = directed()
		}
		if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/"+operation, request, nil, nil); status < 400 || status >= 500 {
			t.Fatalf("unapproved material edit %s=%d", operation, status)
		}
	}
	var state string
	var deliveries int
	if err := c.Owner.QueryRow(t.Context(), `SELECT status,(SELECT count(*) FROM comms_outbound)
		FROM communication_instruction WHERE review_id=@review`, pgx.StrictNamedArgs{"review": review}).Scan(&state, &deliveries); err != nil {
		t.Fatal(err)
	}
	if state != "directed" || deliveries != 0 {
		t.Fatalf("material edit instruction=%s deliveries=%d", state, deliveries)
	}
}

func TestACorrectedApprovalCannotSendPastANewStop(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", spellingCorrection(original), nil, nil); status != http.StatusOK {
		t.Fatalf("correcting wording=%d", status)
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress", AnyMap{
		"kind": "subject_request", "reason": "Asked to stop before sending.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("recording stop=%d", status)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status < 400 || status >= 500 {
		t.Fatalf("sending past stop=%d", status)
	}
	var deliveries int
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*) FROM comms_outbound`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("a stopped corrected message staged %d deliveries", deliveries)
	}
}

func TestAnApproverCannotRewriteAnotherSendersHeldMessage(t *testing.T) {
	c, review, observed := approvedHeldWordingFor(t, true)
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", spellingCorrection(observed), nil, nil); status != http.StatusNotFound {
		t.Fatalf("non-owner correction=%d", status)
	}
	var corrected bool
	if err := c.Owner.QueryRow(t.Context(), `SELECT corrected_wording IS NOT NULL FROM communication_instruction WHERE review_id=@review`, pgx.StrictNamedArgs{"review": review}).Scan(&corrected); err != nil {
		t.Fatal(err)
	}
	if corrected {
		t.Fatal("a reviewer rewrote another sender's message")
	}
}

func TestAPayloadSchemaChangeInvalidatesAnExistingWordingApproval(t *testing.T) {
	c, review, observed := approvedHeldWording(t)
	if _, err := c.Owner.Exec(t.Context(), `UPDATE scheduled_send SET payload_version=payload_version+1 WHERE id=(SELECT delivery_intent_id FROM communication_review WHERE id=@review)`, pgx.StrictNamedArgs{"review": review}); err != nil {
		t.Fatal(err)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", spellingCorrection(observed), nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("changed payload schema correction=%d", status)
	}
	var deliveries int
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*) FROM comms_outbound`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("changed payload schema staged %d deliveries", deliveries)
	}
}

func TestACorrectionSanitizesItsHTMLAndRecordsBothOwnedWrites(t *testing.T) {
	c, review, observed := approvedHeldWording(t)
	request := spellingCorrection(observed)
	request["html_body"] = "<p>The requested information.</p><script>alert(1)</script>"
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording", request, nil, nil); status != http.StatusOK {
		t.Fatalf("HTML correction=%d", status)
	}
	var markup string
	var audits, events int
	if err := c.Owner.QueryRow(t.Context(), `SELECT s.payload->>'html_body',
  (SELECT count(*) FROM audit_log WHERE entity_id=s.id AND action='update' AND evidence->>'instruction_id'=@instruction::uuid::text),
  (SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=s.id::text AND envelope->>'type'='scheduled_send.updated')
  FROM scheduled_send s JOIN communication_review r ON r.delivery_intent_id=s.id WHERE r.id=@review`, pgx.StrictNamedArgs{"review": review, "instruction": observed.InstructionId}).Scan(&markup, &audits, &events); err != nil {
		t.Fatal(err)
	}
	if markup != "<p>The requested information.</p>" || audits != 1 || events != 1 {
		t.Fatalf("sanitized=%t audits=%d events=%d", markup == "<p>The requested information.</p>", audits, events)
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("sending sanitized correction=%d", status)
	}
	var delivered string
	if err := c.Owner.QueryRow(t.Context(), `SELECT html_body FROM comms_outbound`).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(delivered, markup) || strings.Contains(delivered, "<script>") {
		t.Fatal("the corrected, sanitized HTML did not reach delivery")
	}
}

func TestEachCorrectionKeepsTheOriginalDecisionAndNeedsTheLatestSnapshot(t *testing.T) {
	c, review, original := approvedHeldWording(t)
	path := "/v1/communication-reviews/" + review + "/correct-wording"
	if status := c.Call(t, "POST", path, spellingCorrection(original), nil, nil); status != http.StatusOK {
		t.Fatalf("first correction=%d", status)
	}
	var current crmcontracts.CommunicationReview
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &current); status != http.StatusOK || current.WordingCorrection == nil {
		t.Fatalf("reading first correction=%d", status)
	}
	request := spellingCorrection(*current.WordingCorrection)
	request["subject"] = original.Subject + "."
	if status := c.Call(t, "POST", path, request, nil, nil); status != http.StatusOK {
		t.Fatalf("second correction=%d", status)
	}
	var count int
	var acknowledged string
	if err := c.Owner.QueryRow(t.Context(), `SELECT count(*),min(encode(acknowledged_wording,'hex')) FROM communication_instruction WHERE review_id=@review`, pgx.StrictNamedArgs{"review": review}).Scan(&count, &acknowledged); err != nil {
		t.Fatal(err)
	}
	if count != 1 || acknowledged != original.WordingDigest {
		t.Fatal("a subsequent correction replaced the original decision")
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", directed(), nil, nil); status != http.StatusCreated {
		t.Fatalf("sending second correction=%d", status)
	}
	if _, err := c.Owner.Exec(t.Context(), `UPDATE communication_instruction SET corrected_wording=decode(repeat('0',64),'hex') WHERE review_id=@review`, pgx.StrictNamedArgs{"review": review}); err == nil {
		t.Fatal("the database accepted a correction to a spent decision")
	}
}
