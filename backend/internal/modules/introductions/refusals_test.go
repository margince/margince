// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func refusedField(t *testing.T, err error) string {
	t.Helper()
	var refusal *values.ParseError
	if !errors.As(err, &refusal) {
		t.Fatalf("got %v, want a refusal that names a field", err)
	}
	return refusal.Field
}

// An ask with no case behind it asks a colleague for a favour with nothing to
// weigh, and a reason of only spaces is the same ask in disguise.
func TestAnAskNeedsAReasonThatSaysSomething(t *testing.T) {
	requester := ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + requester.String(), UserID: requester,
	})
	for _, reason := range []string{"", "     ", "\t\n"} {
		_, err := requesterOf(ctx, NewRequest{IntroducerUser: ids.NewV7(), InternalReason: reason})

		if got := refusedField(t, err); got != "internal_reason" {
			t.Errorf("reason %q refused on %q, want internal_reason", reason, got)
		}
	}
}

func TestSuggestingSomebodyElseWithNobodyNamedIsRefused(t *testing.T) {
	err := (&Store{}).Decide(context.Background(), ids.NewV7(), StatusSuggestOther, "", nil, 1)

	if got := refusedField(t, err); got != "suggested_user_id" {
		t.Errorf("refused on %q, want suggested_user_id", got)
	}
}
