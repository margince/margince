// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/schema"
)

// Envelope validation binds each event to the supplied conversation.
// Evidence is checked per event before writing, so one refusal cannot hide a valid risk notice.
func validateExtractEnvelope(payload extractPayload, thread settledThread) string {
	if payload.Events == nil {
		return "the reply carries no events key, so it did not answer the question"
	}
	events := payload.events()
	if len(events) > extractMaxEvents {
		return fmt.Sprintf("the conversation yielded %d events, and at most %d may be reported",
			len(events), extractMaxEvents)
	}
	supplied := map[string]bool{}
	for _, message := range thread.Messages {
		supplied[message.ID.String()] = true
	}
	for _, event := range events {
		// Model output may echo untrusted mail. Bound it before logging or retrying.
		if _, ok := extractKinds[event.Kind]; !ok {
			return fmt.Sprintf("event kind %q is not one this site records", clampToken(event.Kind))
		}
		if !supplied[event.MessageID] {
			return fmt.Sprintf("event cites message %q, which was not in the conversation",
				clampToken(event.MessageID))
		}
		if strings.TrimSpace(event.Summary) == "" {
			return "an event carries no summary, so nothing on the card would say what happened"
		}
		if event.Confidence < 0 || event.Confidence > 1 {
			return fmt.Sprintf("confidence %v is outside [0,1]", event.Confidence)
		}
	}
	return ""
}

func validateExtractPayload(payload extractPayload, thread settledThread) string {
	if msg := validateExtractEnvelope(payload, thread); msg != "" {
		return msg
	}
	for _, event := range payload.events() {
		if msg := validateEventEvidence(event, thread); msg != "" {
			return msg
		}
	}
	return ""
}

// extractSchema is the generation-time shape guardrail.
func extractSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"events": schema.Array(schema.Object(
				map[string]schema.Node{
					commitmentMeaningField:  commitmentMeaningSchema(),
					paramKind:               schema.Enum("contract_ended", extractKindOpportunity, "commitment_made"),
					"message_id":            schema.String(),
					"summary":               schema.String(),
					claimQuoteKey:           schema.String(),
					"due_date":              schema.String(),
					"restated_message_id":   schema.String(),
					"restated_quote":        schema.String(),
					"promiser_ref":          schema.String(),
					commitmentRecipientsKey: schema.Array(schema.String()),
					"lifecycle_scope":       schema.Enum("", lifecycleScopeCompany, lifecycleScopeWorkstream, lifecycleScopeUnknown),
					"workstream":            schema.String(),
					"buyer_ref":             schema.String(),
					"commercial_role":       schema.Enum(commercialRoleBuyer, commercialRoleSupplier, commercialRolePartner, commercialRoleProvider, commercialRoleUnknown),
					extractionConfidenceKey: schema.Number(),
				},
				paramKind, "message_id", "summary", claimQuoteKey, "due_date", "promiser_ref", extractionConfidenceKey,
			)),
		},
		"events",
	))
}
