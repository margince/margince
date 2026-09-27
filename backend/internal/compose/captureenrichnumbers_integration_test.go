// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A signature listing several numbers, through the real pass: the model's reply
// is scripted, and everything after it — the evidence gate, the apply, the
// number list and the evidence rows — is production's.
//
// The rule these hold: every number is recorded, a newer signature replaces a
// number only by stating a different number of the same country, and a number
// it leaves out stays.

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	germany    = "+49 175 5550101"
	germanyNew = "+49 171 5550109"
	vietnam    = "+84 35 555 0102"
	singapore  = "+65 9555 0103"
	thailand   = "+66 97 555 0104"
)

// fourNumberSignature is Bob's signature block with the German number given.
func fourNumberSignature(german string) string {
	return "Hi,\n\nsee you next week.\n\nBob Contact\nRegional Director\n" +
		"Germany: " + german + "\nVietnam: " + vietnam + "\nSingapore: " + singapore + "\nThailand: " + thailand
}

// phoneReply is the model's answer: one phone entry per number, each quoting
// itself.
func phoneReply(numbers ...string) []map[string]any {
	out := make([]map[string]any, 0, len(numbers))
	for _, n := range numbers {
		out = append(out, map[string]any{"field": "phone", "value": n, "evidence_snippet": n, "confidence": 0.9})
	}
	return out
}

func TestEveryNumberASignatureListsIsRecorded(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bob@numbers.example", fourNumberSignature(germany))
	brain := &signatureScriptBrain{fields: phoneReply(germany, vietnam, singapore, thailand)}
	runEnrichPass(t, e, brain)

	want := []string{"+491755550101", "+6595550103", "+66975550104", "+84355550102"}
	if got := liveNumbers(t, e, contact); !slices.Equal(got, want) {
		t.Fatalf("live numbers = %v, want all four the signature lists", got)
	}
	evidence := numberEvidence(t, e, contact)
	if got := slices.Sorted(maps.Keys(evidence)); !slices.Equal(got, want) {
		t.Fatalf("evidence rows = %v, want one per number", got)
	}
	for number, row := range evidence {
		if row.replaced != "" {
			t.Errorf("%s records it replaced %s — four numbers stated together replace nothing", number, row.replaced)
		}
	}
	if got := evidence["+6595550103"].snippet; got != singapore {
		t.Errorf("Singapore's evidence = %q, want its own line %q", got, singapore)
	}

	t.Run("a newer signature with one number changed replaces only that one", func(t *testing.T) {
		seedLaterMail(t, e, contact, "bob@numbers.example", fourNumberSignature(germanyNew), "1 hour")
		brain.fields = phoneReply(germanyNew, vietnam, singapore, thailand)
		runEnrichPass(t, e, brain)

		want := []string{"+491715550109", "+6595550103", "+66975550104", "+84355550102"}
		if got := liveNumbers(t, e, contact); !slices.Equal(got, want) {
			t.Fatalf("live numbers = %v, want the new German number beside the three unchanged", got)
		}
		evidence := numberEvidence(t, e, contact)
		if got := slices.Sorted(maps.Keys(evidence)); !slices.Equal(got, want) {
			t.Fatalf("evidence rows = %v, want the old German row taken over by the new one", got)
		}
		for number, row := range evidence {
			wantReplaced := ""
			if number == "+491715550109" {
				wantReplaced = "+491755550101"
			}
			if row.replaced != wantReplaced {
				t.Errorf("%s records it replaced %q, want %q", number, row.replaced, wantReplaced)
			}
		}
	})

	t.Run("undo brings back the German number and nothing else moves", func(t *testing.T) {
		store := contacts.NewStore(InstallationDB(e.Pool))
		if err := store.RestoreProfileField(e.Admin(), ids.From[ids.ContactKind](contact), "phone", "+491715550109"); err != nil {
			t.Fatalf("restore: %v", err)
		}
		want := []string{"+491755550101", "+6595550103", "+66975550104", "+84355550102"}
		if got := liveNumbers(t, e, contact); !slices.Equal(got, want) {
			t.Fatalf("live numbers = %v after the undo, want the old German number back and the rest untouched", got)
		}
		if got := slices.Sorted(maps.Keys(numberEvidence(t, e, contact))); !slices.Equal(got, want) {
			t.Fatalf("evidence rows = %v after the undo, want the replacement's row gone with its number", got)
		}
	})

	t.Run("a number a newer signature leaves out stays", func(t *testing.T) {
		seedLaterMail(t, e, contact, "bob@numbers.example",
			"Hi,\n\nBob Contact\nGermany: "+germany+"\nVietnam: "+vietnam, "2 hours")
		brain.fields = phoneReply(germany, vietnam)
		runEnrichPass(t, e, brain)

		want := []string{"+491755550101", "+6595550103", "+66975550104", "+84355550102"}
		if got := liveNumbers(t, e, contact); !slices.Equal(got, want) {
			t.Fatalf("live numbers = %v, want Singapore and Thailand kept: a trimmed signature retires nothing", got)
		}
		for number, row := range numberEvidence(t, e, contact) {
			if row.replaced != "" {
				t.Errorf("%s records it replaced %s after a signature that only left numbers out", number, row.replaced)
			}
		}
	})
}

// A trimmed signature that also changes one number replaces that number's own
// predecessor, never the number it simply stopped listing.
func TestAChangedNumberReplacesItsOwnCountryNotAnOmittedOne(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bob@trim.example", fourNumberSignature(germany))
	brain := &signatureScriptBrain{fields: phoneReply(germany, vietnam, singapore, thailand)}
	runEnrichPass(t, e, brain)

	const singaporeNew = "+65 9555 0199"
	seedLaterMail(t, e, contact, "bob@trim.example", "Hi,\n\nBob Contact\nSingapore: "+singaporeNew, "1 hour")
	brain.fields = phoneReply(singaporeNew)
	runEnrichPass(t, e, brain)

	want := []string{"+491755550101", "+6595550199", "+66975550104", "+84355550102"}
	if got := liveNumbers(t, e, contact); !slices.Equal(got, want) {
		t.Fatalf("live numbers = %v, want the new Singapore number in place of the old one and the rest kept", got)
	}
	if got := numberEvidence(t, e, contact)["+6595550199"].replaced; got != "+6595550103" {
		t.Errorf("the new Singapore number records it replaced %q, want the old Singapore number", got)
	}
}

func runEnrichPass(t *testing.T, e *integration.Env, brain *signatureScriptBrain) {
	t.Helper()
	enricher := NewCaptureEnricher(e.Pool, brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// seedLaterMail links a newer inbound mail from the contact, dated after
// what they sent before.
func seedLaterMail(t *testing.T, e *integration.Env, contact ids.UUID, from, body, after string) {
	t.Helper()
	mail := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source_system, source_id, source, captured_by)
			VALUES ($1, 'email', 'again', $2, 'inbound', now() + $3::interval, 'gmail', $4, 'gmail:seed', 'connector:gmail')`,
			mail, body, after, mail.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
			mail, contact); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, $3, 'from')`,
			mail, contact, from)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func liveNumbers(t *testing.T, e *integration.Env, contact ids.UUID) []string {
	t.Helper()
	var numbers []string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT phone FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL ORDER BY phone`, contact)
		if err != nil {
			return err
		}
		numbers, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return numbers
}

type numberRow struct{ snippet, replaced string }

// numberEvidence is the phone evidence rows, by number.
func numberEvidence(t *testing.T, e *integration.Env, contact ids.UUID) map[string]numberRow {
	t.Helper()
	out := map[string]numberRow{}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT value_key, evidence_snippet, coalesce(superseded_value, '')
			  FROM contact_profile_field WHERE contact_id = $1 AND field = 'phone'`, contact)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var number string
			var row numberRow
			if err := rows.Scan(&number, &row.snippet, &row.replaced); err != nil {
				return err
			}
			out[number] = row
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
