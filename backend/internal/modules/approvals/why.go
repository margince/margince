// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// whyLimit is the contract's maxLength for WorkGrounds.text, in characters.
const whyLimit = 600

// Grounds is why a proposal exists: who did or said what, about which record,
// and when. Sentence is Margince's own words, shown in each reader's language.
// Text is someone else's words, such as an agent's stated reason, shown as
// written. A proposal sets one of them, or says its headline already does.
type Grounds struct {
	Sentence *systemtext.Component
	Text     string
	// InHeadline marks a proposal whose headline names who, which record and
	// the trigger, so a reason beneath it would only repeat it.
	InHeadline bool
}

// HeadlineSaysWhy is the reason of a proposal whose headline states it.
var HeadlineSaysWhy = Grounds{InHeadline: true}

// storedWhy is the reason as the summary envelope keeps it. It holds the words
// in their stored language, and Margince's sentence when Margince wrote them.
type storedWhy struct {
	Text     string                `json:"text"`
	Sentence *systemtext.Component `json:"sentence,omitempty"`
}

// storedGrounds words the reason for storage. A sentence that will not render
// keeps no reason rather than failing the proposal it explains.
func storedGrounds(ctx context.Context, why Grounds) *storedWhy {
	if why.Sentence != nil {
		text, err := why.Sentence.Render(textlang.English)
		if err != nil {
			slog.ErrorContext(ctx, "approval reason did not render", "err", err)
			return nil
		}
		return &storedWhy{Text: textcut.Runes(text, whyLimit), Sentence: why.Sentence}
	}
	if text := strings.TrimSpace(why.Text); text != "" {
		return &storedWhy{Text: textcut.Runes(SanitizeSummary(text), whyLimit)}
	}
	return nil
}

// readWhy is the stored reason in the reader's language, nil when the
// proposal gave none.
func readWhy(ctx context.Context, stored json.RawMessage) *crmcontracts.WorkGrounds {
	var envelope storedSummary
	if len(stored) == 0 || json.Unmarshal(stored, &envelope) != nil || envelope.Why == nil {
		return nil
	}
	text := envelope.Why.Text
	if envelope.Why.Sentence != nil {
		if localized, err := envelope.Why.Sentence.Render(systemtext.Locale(ctx)); err == nil {
			text = localized
		}
	}
	if text == "" {
		return nil
	}
	return &crmcontracts.WorkGrounds{Text: text}
}
