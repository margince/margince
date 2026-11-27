// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A reply carries its anchor's links with a read check only when it continues
// a mail conversation. Every other reply adds to the records it names, so it
// needs write authority over them, like any new activity.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// colleaguesCompany is a workspace-readable company the rep may not change.
func (e *sendEnv) colleaguesCompany(t *testing.T, anchor ids.ActivityID) ids.UUID {
	t.Helper()
	company := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO company (id, display_name, owner_id, source, captured_by)
		 VALUES ($1, 'Colleague GmbH', $2, 'manual', 'human:x')`, company, e.other); err != nil {
		t.Fatalf("seeding the colleague's company: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`,
		anchor, company); err != nil {
		t.Fatalf("filing the anchor on the company: %v", err)
	}
	return company
}

func (e *sendEnv) replyTo(anchor ids.ActivityID, subject string) error {
	in := sendInput("transactional")
	in.Subject = subject
	_, err := e.store(stubUnsubscribeLinker{}).SendEmail(
		e.as(principal.RowScopeOwn), FromActivity(anchor), in, stubConsentGate{}, &recordingStager{})
	return err
}

func TestAReplyThreadedOntoAMailCarriesItsLinks(t *testing.T) {
	t.Parallel()
	e := setupSend(t)
	anchor := e.seedAnchor(t, "parent@buyer.test", "root@buyer.test")
	company := e.colleaguesCompany(t, anchor)

	if err := e.replyTo(anchor, "Re: pricing"); err != nil {
		t.Fatalf("a reply in the mail's conversation → %v, want it filed where the mail is", err)
	}
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity_link WHERE company_id = $1 AND activity_id <> $2`, company, anchor.UUID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("the reply is filed on the colleague's company %d times, want once", n)
	}
}

func TestAReplyThatStartsAConversationAddsToTheRecordAnew(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		anchor  func(e *sendEnv, t *testing.T) ids.ActivityID
		subject string
	}{
		"to a note": {func(e *sendEnv, t *testing.T) ids.ActivityID {
			id := ids.New[ids.ActivityKind]()
			if _, err := e.owner.Exec(context.Background(),
				`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
				 VALUES ($1, 'note', 'Pricing question', now(), 'manual', 'human:x')`, id); err != nil {
				t.Fatalf("seeding the note: %v", err)
			}
			return id
		}, "Re: pricing"},
		"on a new topic": {func(e *sendEnv, t *testing.T) ids.ActivityID {
			return e.seedAnchor(t, "parent@buyer.test", "root@buyer.test")
		}, "Invoice 4711 overdue"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupSend(t)
			anchor := c.anchor(e, t)
			e.colleaguesCompany(t, anchor)
			if err := e.replyTo(anchor, c.subject); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("a reply that starts a conversation on a colleague's company → %v, want permission denied", err)
			}
		})
	}
}
