// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSignatureWorkKeepsArrivalsDuringARead(t *testing.T) {
	e := setupDedupe(t)
	contact, err := e.store.CreateContact(e.as(), CreateContactInput{FullName: "Pending correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	ctx := principal.SystemActing(e.as(), "agent:enrich")
	queue := func(run *ids.UUID) {
		t.Helper()
		if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error { return QueueSignatureWorkTx(ctx, tx, []ids.ContactID{id}, run) }); err != nil {
			t.Fatal(err)
		}
	}
	pending := func() []SignatureWork {
		t.Helper()
		work, err := e.store.PendingSignatureWork(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		return work
	}
	queue(nil)
	first := pending()
	if got := e.countAs(ctx, t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND attempted_at IS NULL`, pgx.NamedArgs{"contact": id}); got != 1 {
		t.Fatal("a never-read contact received an invented attempt timestamp")
	}
	if len(first) != 1 || first[0].ContactID != id {
		t.Fatalf("first work=%+v", first)
	}
	queue(nil)
	if err := e.store.SettleSignatureWork(ctx, first[0], 0); err != nil {
		t.Fatal(err)
	}
	second := pending()
	if len(second) != 1 || second[0].Generation <= first[0].Generation {
		t.Fatalf("arrival was lost: %+v", second)
	}
	if err := e.store.SettleSignatureWork(ctx, second[0], 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := pending(); len(got) != 0 {
		t.Fatalf("failed work did not back off: %+v", got)
	}
	queue(nil)
	third := pending()
	if len(third) != 1 {
		t.Fatalf("new arrival did not reopen backoff: %+v", third)
	}
	if err := e.store.SettleSignatureWork(ctx, third[0], 0); err != nil {
		t.Fatal(err)
	}
	if got := pending(); len(got) != 0 {
		t.Fatalf("settled work remains due: %+v", got)
	}
}

func TestMergedSignatureWorkStartsBeyondBothPriorGenerationRanges(t *testing.T) {
	for _, test := range []struct {
		name           string
		source, target int
		settleSource   bool
	}{
		{"dirty source behind clean survivor", 2, 9, false},
		{"dirty survivor behind clean source", 9, 2, true},
		{"unread source and clean survivor", 0, 9, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := setupDedupe(t)
			human := e.as()
			source, err := e.store.CreateContact(human, CreateContactInput{FullName: "Merged correspondent", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			target, err := e.store.CreateContact(human, CreateContactInput{FullName: "Surviving correspondent", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sourceID, targetID := ids.From[ids.ContactKind](ids.UUID(source.Id)), ids.From[ids.ContactKind](ids.UUID(target.Id))
			ctx := principal.SystemActing(human, "agent:enrich")
			for id, n := range map[ids.ContactID]int{sourceID: test.source, targetID: test.target} {
				if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
					for range n {
						if err := QueueSignatureWorkTx(ctx, tx, []ids.ContactID{id}, nil); err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			observed, err := e.store.PendingSignatureWork(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			for _, work := range observed {
				clean := work.ContactID == targetID
				if test.settleSource {
					clean = work.ContactID == sourceID
				}
				if clean {
					if err := e.store.SettleSignatureWork(ctx, work, 0); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := e.store.MergeContact(human, sourceID, targetID, nil); err != nil {
				t.Fatal(err)
			}
			for _, work := range observed {
				if err := e.store.SettleSignatureWork(ctx, work, 0); err != nil {
					t.Fatal(err)
				}
			}
			pending, err := e.store.PendingSignatureWork(ctx, 10)
			if err != nil || len(pending) != 1 || pending[0].ContactID != targetID || pending[0].Generation <= int64(max(test.source, test.target)) {
				t.Fatalf("merge lost pending work: %+v %v", pending, err)
			}
		})
	}
}

func TestLiveSignatureRequestsPrecedeAnOlderHistoricalBacklog(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.SystemActing(e.as(), "agent:enrich")
	var historical []ids.ContactID
	for _, name := range []string{"Older correspondent", "Another older correspondent"} {
		contact, err := e.store.CreateContact(e.as(), CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		historical = append(historical, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := QueueSignatureEventTx(ctx, tx, historical, SignatureRequest{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.execAs(ctx, t, `UPDATE contact_signature_enrich_state SET due_at='-infinity' WHERE contact_id=ANY(@contacts)`, pgx.NamedArgs{"contacts": historical})
	contact, err := e.store.CreateContact(e.as(), CreateContactInput{FullName: "Current correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	live := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error { return QueueSignatureWorkTx(ctx, tx, []ids.ContactID{live}, nil) }); err != nil {
		t.Fatal(err)
	}
	work, err := e.store.PendingSignatureWork(ctx, 1)
	if err != nil || len(work) != 1 || work[0].ContactID != live {
		t.Fatalf("live work waited behind history: %+v %v", work, err)
	}
	if err := e.store.SettleSignatureWork(ctx, work[0], 0); err != nil {
		t.Fatal(err)
	}
	work, err = e.store.PendingSignatureWork(ctx, 2)
	if err != nil || len(work) != len(historical) {
		t.Fatalf("historical work was lost: %+v %v", work, err)
	}
}
