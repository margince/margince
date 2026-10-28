// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// SummaryWords words a summary the server built from parts for whoever reads
// it. The producers own the wording, so compose supplies it.
type SummaryWords interface {
	ReaderLanguage(ctx context.Context, tx pgx.Tx) textlang.Lang
	// Word answers false for a kind or parts it does not know.
	Word(lang textlang.Lang, kind string, params json.RawMessage) (string, bool)
}

var summaryWords SummaryWords

// WordSummariesWith sets the words every signal read uses.
func WordSummariesWith(words SummaryWords) { summaryWords = words }

// SummaryIn is the summary in this language when the row kept the parts it
// was worded from, and the stored summary otherwise.
func SummaryIn(lang textlang.Lang, kind string, params json.RawMessage, stored string) string {
	if summaryWords == nil || len(params) == 0 {
		return stored
	}
	if worded, ok := summaryWords.Word(lang, kind, params); ok && worded != "" {
		return worded
	}
	return stored
}

// wordForReader rewords, in the caller's language, each signal whose row kept
// its parts. params runs beside read, one entry per signal.
func wordForReader(ctx context.Context, tx pgx.Tx, read []crmcontracts.Signal, params []json.RawMessage) {
	if summaryWords == nil || !anyParts(params) {
		return
	}
	lang := summaryWords.ReaderLanguage(ctx, tx)
	for i := range read {
		read[i].Summary = SummaryIn(lang, string(read[i].Kind), params[i], read[i].Summary)
	}
}

func anyParts(params []json.RawMessage) bool {
	for _, parts := range params {
		if len(parts) > 0 {
			return true
		}
	}
	return false
}
