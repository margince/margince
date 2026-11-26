// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOnlyTheSystemPagesARemovedDomainsMail(t *testing.T) {
	_, err := DealMailSentFrom(context.Background(), nil, "buyer-corp.test", ids.UUID{}, 10)
	if err == nil {
		t.Error("a caller that is not the system was answered, want a refusal before any query")
	}
}

func TestADomainThatFoldsToNothingNamesNoMail(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "system:test")
	page, err := DealMailSentFrom(ctx, nil, "  ", ids.UUID{}, 10)
	if err != nil || len(page) != 0 {
		t.Errorf("a blank domain answered %v and %v, want no mail and no query", page, err)
	}
}
