// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"
)

// A fresh link still supersedes one whose issue was stamped by a replica whose
// clock runs ahead. The earlier link's expiry is held just after its own issue.
// Dating it before that issue would fail the lifetime check and roll back the
// fresh mint with it.
func TestAFreshLinkSupersedesOneStampedByAFasterClock(t *testing.T) {
	e := setupChannelConsent(t)
	issueLink(t, e)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE confirm_token SET issued_at = issued_at + interval '1 minute' WHERE contact_id = $1`,
		e.contact); err != nil {
		t.Fatalf("stamp the earlier link from a faster clock: %v", err)
	}

	second := issueLink(t, e)

	if _, err := e.store.ResolveConfirmToken(e.ctx, second.Token, FetchByAHuman); err != nil {
		t.Errorf("the fresh link does not open: %v", err)
	}
	var issued, expires time.Time
	if err := e.owner.QueryRow(context.Background(),
		`SELECT issued_at, expires_at FROM confirm_token
		  WHERE contact_id = $1 ORDER BY issued_at DESC LIMIT 1`, e.contact).Scan(&issued, &expires); err != nil {
		t.Fatal(err)
	}
	if !expires.After(issued) || expires.Sub(issued) > time.Millisecond {
		t.Errorf("the earlier link expires at %v for an issue at %v; want just after its issue", expires, issued)
	}
}
