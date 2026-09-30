// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The existing-customer exception (UWG §7(3)) is not offered: a pack may grant
// it, and the verdict still refuses, saying why.

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func (e *resolveEnv) marketingVerdict(t *testing.T, marketing MarketingContext) Verdict {
	t.Helper()
	purpose := PurposeRow{ID: ids.NewV7().String(), Key: "newsletter", Label: "Newsletter", Class: ClassMarketing}
	var out Verdict
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = VerdictForContact(e.ctx, tx, e.contact.String(), purpose, time.Now().Add(-defaultReplyWindow), marketing)
		return err
	}); err != nil {
		t.Fatalf("reading the verdict: %v", err)
	}
	return out
}

func TestAGrantedExistingCustomerExceptionStillRefusesAndSaysWhy(t *testing.T) {
	e := setupResolve(t)
	granted := e.marketingVerdict(t, MarketingContext{Exception: &messaging.MarketingException{
		Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true,
	}})
	if granted.State != VerdictUnknown {
		t.Fatalf("verdict with the exception granted = %s, want unknown: the exception is not offered", granted.State)
	}
	if !strings.Contains(granted.Reason, "existing-customer exception is not offered") {
		t.Fatalf("reason = %q, want it to say the exception is not offered", granted.Reason)
	}

	none := e.marketingVerdict(t, MarketingContext{})
	if none.State != VerdictUnknown || none.Reason != "no consent recorded" {
		t.Fatalf("verdict with no exception = %+v, want unknown with the plain reason", none)
	}
}
