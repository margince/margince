// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RequestSettlementRule names the rule a settlement verdict was reached under.
// A still_owed verdict recorded under another rule is judged again.
const RequestSettlementRule = "quoted-ask-1"

// minRemainderWords is the shortest quote that names an ask. One word ("Please",
// "Rechnung") can be found in almost any message.
const minRemainderWords = 2

// RequestRemainderGrounded reports whether remaining quotes the customer's own
// words in the request body: at least two words, verbatim, outside quoted
// history and the signature. Our reply never supplies them, so a question we
// asked cannot become work we owe.
func RequestRemainderGrounded(body, remaining string) bool {
	words := strings.FieldsFunc(remaining, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) < minRemainderWords {
		return false
	}
	return values.Quoted(textlang.MessageOpening(body, utf8.RuneCountInString(body)), remaining)
}

// groundedRemainderTx is the remaining text a verdict may record, re-read
// against the request body under the verdict's locks. A text that quotes no
// ask of theirs is dropped, so it never retitles the reminder; the verdict
// itself stands.
func groundedRemainderTx(ctx context.Context, tx pgx.Tx, in RequestSettlementInput) (string, error) {
	if in.Verdict != RequestStillOwed || in.Remaining == "" {
		return in.Remaining, nil
	}
	var body string
	if err := tx.QueryRow(ctx, `SELECT coalesce(body, '') FROM activity WHERE id = $1`,
		in.Request.RequestID).Scan(&body); err != nil {
		return "", fmt.Errorf("activities: re-reading the request to check the remaining quote: %w", err)
	}
	if !RequestRemainderGrounded(body, in.Remaining) {
		return "", nil
	}
	return in.Remaining, nil
}
