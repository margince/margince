// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// SummaryWords words a summary from its stored parts, false for a kind or parts
// it does not know. Compose supplies it, because the producers own the wording.
type SummaryWords func(lang textlang.Lang, kind string, params json.RawMessage) (string, bool)

// ForReader words the summary in the caller's chosen language when the row kept
// its parts. Otherwise it is the stored text, already in the base language.
func (words SummaryWords) ForReader(ctx context.Context, kind string, params json.RawMessage, stored string) string {
	if words == nil || len(params) == 0 {
		return stored
	}
	actor, ok := principal.Actor(ctx)
	if !ok || !textlang.Known(actor.Locale) {
		return stored
	}
	if worded, ok := words(textlang.Lang(actor.Locale), kind, params); ok && worded != "" {
		return worded
	}
	return stored
}

func (words SummaryWords) reword(ctx context.Context, sig crmcontracts.Signal, params json.RawMessage) crmcontracts.Signal {
	sig.Summary = words.ForReader(ctx, string(sig.Kind), params, sig.Summary)
	return sig
}
