// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

type storedSummary struct {
	crmcontracts.ApprovalSummaryParts
	SystemText *systemtext.Message `json:"_system_text,omitempty"` //nolint:tagliatelle // The private envelope key cannot collide with a public contract field.
	EffectText *systemtext.Message `json:"_effect_text,omitempty"` //nolint:tagliatelle // The private envelope key cannot collide with a public contract field.
}

func marshalSystemSummary(ctx context.Context, summary string, parts *crmcontracts.ApprovalSummaryParts, message, effect *systemtext.Message) (json.RawMessage, error) {
	if message == nil && effect == nil {
		return marshalSummaryParts(parts)
	}
	if parts == nil {
		parts = &crmcontracts.ApprovalSummaryParts{Sentence: summary, Fields: []crmcontracts.ApprovalSummaryField{}}
	}
	metadata := systemtext.ForWrite(ctx, message, parts.Sentence, "")
	return json.Marshal(storedSummary{ApprovalSummaryParts: *parts, SystemText: metadata, EffectText: systemtext.BoundEffect(effect)})
}

// SummarySystemText reads provenance from the private stored envelope.
func SummarySystemText(raw json.RawMessage) *systemtext.Message {
	var stored storedSummary
	if json.Unmarshal(raw, &stored) != nil {
		return nil
	}
	return stored.SystemText
}

func prepareSystemSummary(ctx context.Context, in StageInput) StageInput {
	if in.SystemText == nil {
		in.Summary = SanitizeSummary(in.Summary)
		in.SummaryParts = SanitizeSummaryParts(in.SummaryParts)
		return in
	}
	sentence := in.Summary
	if in.SummaryParts != nil {
		sentence = in.SummaryParts.Sentence
	}
	metadata, fitted, _, err := systemtext.Fit(in.SystemText, sentence, "", maxSummaryLen, 0)
	if errors.Is(err, systemtext.ErrDisplayLimit) {
		metadata = &systemtext.Message{Subject: systemtext.Sentence("approval.unsummarized")}
		fitted = systemtext.Literal("approval.unsummarized")
		if in.SummaryParts == nil {
			in.SummaryParts = &crmcontracts.ApprovalSummaryParts{Sentence: fitted, Fields: []crmcontracts.ApprovalSummaryField{}}
		}
	} else if err != nil {
		slog.ErrorContext(ctx, "approval display metadata rejected", "err", err)
		metadata, fitted = nil, sentence
	}
	in.SystemText = metadata
	if in.SummaryParts == nil {
		in.Summary = fitted
	} else {
		parts := *in.SummaryParts
		parts.Sentence = fitted
		in.SummaryParts = &parts
	}
	in.Summary = SanitizeSummary(in.Summary)
	in.SummaryParts = SanitizeSummaryParts(in.SummaryParts)
	return in
}

// EffectSystemText preserves staged provenance until the authorized effect redeems it.
func EffectSystemText(raw json.RawMessage) *systemtext.Message {
	var stored storedSummary
	if json.Unmarshal(raw, &stored) != nil {
		return nil
	}
	return stored.EffectText
}
