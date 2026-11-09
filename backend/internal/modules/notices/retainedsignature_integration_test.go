// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestARetainedSignatureNoticeKeepsItsLinkAcrossStoredDescriptorVersions(t *testing.T) {
	e := setupNotices(t)
	signature, owner := ids.NewV7(), ids.NewV7()
	input := NewNotice{Recipient: e.recipient, Kind: KindPrivacyReviewRequired,
		Subject:    systemtext.Literal("privacy.review.subject"),
		Body:       fmt.Sprintf(systemtext.Copy("privacy.review.inactive", textlang.English), signature.String(), owner.String()),
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("privacy.review.subject"), Body: systemtext.Sentence("privacy.review.inactive", systemtext.Text(signature.String()), systemtext.Text(owner.String()))},
		DedupeKey:  "retained-signature-review",
	}
	notice, err := e.store.Create(e.engineCtx(), input)
	if err != nil {
		t.Fatal(err)
	}
	unread, err := e.store.UnreadFor(e.asUser(e.recipient), 10)
	if err != nil {
		t.Fatal(err)
	}
	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 || len(page.Items) != 1 {
		t.Fatal("the existing notice did not reach both reader surfaces")
	}
	for _, target := range []Target{unread[0].Target, page.Items[0].Target} {
		if target.Type != "retained_email_signature" || target.ID != signature {
			t.Fatalf("the review opened a different target: %+v", target)
		}
	}
	replay, err := e.store.Create(e.engineCtx(), input)
	if err != nil || replay != notice {
		t.Fatalf("retry replaced the notice: %v", err)
	}
	var untouched bool
	if err := e.owner.QueryRow(context.Background(), `SELECT target_type IS NULL AND target_id IS NULL FROM notice WHERE id=$1`, notice).Scan(&untouched); err != nil {
		t.Fatal(err)
	}
	if !untouched {
		t.Fatal("reading the reference rewrote the delivered notice")
	}
}
