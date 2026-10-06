// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cached relationship brief is generated prose about the subject, held per
// reader. Both privacy acts leave the contact row standing, so the ON DELETE
// CASCADE on contact_id never fires and nothing but a named purge reaches it.
//
// Both acts are asserted because they are separate statements in separate
// files, and an operator told a contact was ANONYMIZED has been told the same
// thing about findability as one told they were erased.

import (
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestErasureDestroysTheCachedBriefEveryReaderHeld(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)
	cacheABrief(t, e, subject)

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 0 {
		t.Errorf("%d cached brief(s) survive the erasure, each holding what the model was told "+
			"about this subject", n)
	}
}

func TestAnonymisingAContactDestroysTheCachedBriefToo(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)
	cacheABrief(t, e, subject)

	service := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := service.AnonymiseContacts(e.Admin(), []ids.UUID{subject}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("AnonymiseContacts → %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 0 {
		t.Errorf("%d cached brief(s) survive the anonymise, still answering with the name it cleared", n)
	}
}

// cacheABrief writes one reader's cached brief for the subject.
//
// The row goes in directly rather than through contactbrief.Service, which
// would need a stubbed model lane and an assembled contact page to produce a
// payload this assertion never reads: the purge is keyed on contact_id alone.
// The shape is the catalog's — payload NOT NULL, generated_by one of two
// values — and cachedBriefs is read once BEFORE each act, so a column this
// fixture stops matching fails the insert rather than quietly testing nothing.
func cacheABrief(t *testing.T, e *integration.Env, subject ids.UUID) {
	t.Helper()
	if _, err := e.Pool.Exec(e.Admin(), `
		INSERT INTO contact_brief (user_id, contact_id, fingerprint, generated_by, payload)
		VALUES ($1, $2, 'fp-1', 'deterministic', $3::jsonb)`,
		e.AdminUser, subject,
		`{"headline":"Briefed Subject asked about the retrofit timeline"}`); err != nil {
		t.Fatalf("caching a brief for the subject: %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 1 {
		t.Fatalf("the fixture cached %d briefs, want 1 — the assertions below would prove nothing", n)
	}
}

func cachedBriefs(t *testing.T, e *integration.Env, subject ids.UUID) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(e.Admin(),
		`SELECT count(*) FROM contact_brief WHERE contact_id = $1`, subject).Scan(&n); err != nil {
		t.Fatalf("counting the subject's cached briefs: %v", err)
	}
	return n
}
