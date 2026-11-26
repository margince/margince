// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

func TestASiteRolePastTheTitleLimitFillsTheTitleCut(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	contactID, companyID := e.seedEmployedContact(ctx, t,
		"Ilse Brandt", "ilse@sitetitle.test", "Brandt GmbH", "sitetitle.test")

	limit := textbounds.LimitOf("public.contact.title")
	role := strings.Repeat("Head of Partnerships ", limit/10)
	matched, err := e.store.ApplySiteContactFields(ctx, companyID, SiteContactFields{
		Name:            "Ilse Brandt",
		Role:            role,
		EvidenceSnippet: "Ilse Brandt — team page",
		SourceURL:       "https://sitetitle.test/team",
	})
	if err != nil {
		t.Fatalf("a site role longer than the title limit failed the fill: %v", err)
	}
	if !matched {
		t.Fatal("the site did not match the contact, so this test proves nothing about the title")
	}

	var title string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT title FROM contact WHERE id = $1`, contactID).Scan(&title)
	}); err != nil {
		t.Fatalf("read back the title: %v", err)
	}
	if n := utf8.RuneCountInString(title); n == 0 || n > limit {
		t.Fatalf("title holds %d characters, want 1..%d", n, limit)
	}
	if !strings.HasPrefix(role, title) {
		t.Fatalf("title %q is not the start of the site's role", title)
	}
}
