// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Right-to-erasure end-to-end: PII gone from the normalized rows, raw
// capture and embeddings; search returns nothing; the tombstone proves
// the erasure without re-storing PII; the suppression list makes
// re-capture skip the subject; legal hold blocks the whole path.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const subjectEmail = "selma.subject@example.test"

// seededPreferenceToken names the preference-center token seedSubject mints
// for a subject. It is a fixture stand-in, not a realistic credential: the
// real thing is 256 bits of crypto/rand behind a pref_ prefix. Derived from
// the contact id because the column is UNIQUE and a suite may seed more than
// one subject.
func seededPreferenceToken(contactID ids.UUID) string {
	return "pref_stands-in-for-a-token-" + contactID.String()
}

// seedSubject plants a contact with an email, a linked activity, a raw
// capture payload mentioning them, one embedding row, and the
// preference-center token a marketing send would have minted for them.
func seedSubject(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	contactID := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact (id, full_name, first_name, title, source, captured_by)
			 VALUES ($1, 'Selma Subject', 'Selma', 'CFO', 'manual', 'human:x')`, contactID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_email (contact_id, email, source, captured_by)
			 VALUES ($1, $2, 'manual', 'human:x')`, contactID, subjectEmail); err != nil {
			return err
		}
		activityID := ids.NewV7()
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			 VALUES ($1, 'note', 'Met Selma', now(), 'manual', 'human:x')`, activityID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity_link (activity_id, entity_type, contact_id)
			 VALUES ($1, 'contact', $2)`, activityID, contactID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO preference_token (contact_id, token, expires_at)
			 VALUES ($1, $2, now() + interval '30 days')`,
			contactID, seededPreferenceToken(contactID)); err != nil {
			return err
		}
		// The OTHER consent capability. A double-opt-in token is a bearer
		// secret in the subject's own mailbox whose only function is to
		// authorise a grant, so an erasure that left it standing would leave a
		// live invitation to consent outliving the certificate that says the
		// data was destroyed.
		purposeID := ids.NewV7()
		if _, err := tx.Exec(ctx,
			`INSERT INTO consent_purpose (id, key, label, requires_double_opt_in)
			 VALUES ($1, $2, 'Newsletter', true)`,
			purposeID, "doi_fixture_"+contactID.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO consent_doi_token (contact_id, purpose_id, token_hash, issued_at, expires_at)
			 VALUES ($1, $2, $3, now(), now() + interval '72 hours')`,
			contactID, purposeID, "doi-hash-"+contactID.String()); err != nil {
			return err
		}
		// The confirm link and what came back through it: a live capability that
		// DISPLAYS the record, and the subject's own words in plaintext.
		var confirmTokenID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO confirm_token (contact_id, token_hash, delivered_to, expires_at)
			 VALUES ($1, $2, 'selma@example.test', now() + interval '14 days')
			 RETURNING id`,
			contactID, "confirm-hash-"+contactID.String()).Scan(&confirmTokenID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_confirm_submission (contact_id, token_id, kind, field, proposed_value)
			 VALUES ($1, $2, 'correction', 'full_name', 'Selma Corrected')`,
			contactID, confirmTokenID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO raw_capture (source_system, source_id, payload)
			 VALUES ('gmail', 'msg-1', jsonb_build_object('from', $1::text, 'body', 'quarterly numbers'))`,
			subjectEmail); err != nil {
			return err
		}
		// The identity is a composite provider/model@dims stamp, not the
		// legacy fixed label — erasure must remove the row regardless of
		// which identity produced it (the DELETE at erasure.go carries no
		// model filter; a mixed-width store must still erase fully).
		vector := "[" + strings.TrimSuffix(strings.Repeat("0.1,", 1023), ",") + ",0.1]"
		_, err := tx.Exec(ctx,
			`INSERT INTO embedding (entity_type, entity_id, chunk_ix, chunk_hash, model, embedding)
			 VALUES ('contact', $1, 0, 'h', 'gemini/gemini-embedding-001@1024', $2::vector)`, contactID, vector)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return contactID
}

// assertSubjectErased verifies every store the subject touched after an
// erasure: emails, embeddings, search, suppression entry, PII-free
// tombstone, scrubbed raw capture.
func assertSubjectErased(t *testing.T, e *Env, contactID ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		checks := []struct {
			what  string
			query string
			want  int
		}{
			{"contact_email rows", `SELECT count(*) FROM contact_email WHERE contact_id = $1`, 0},
			{"embeddings", `SELECT count(*) FROM embedding WHERE entity_type = 'contact' AND entity_id = $1`, 0},
			{"search hits for the name", `SELECT count(*) FROM contact WHERE id = $1 AND search_tsv @@ plainto_tsquery('simple', 'Selma')`, 0},
			{"preference-center tokens", `SELECT count(*) FROM preference_token WHERE contact_id = $1`, 0},
			{"double-opt-in tokens", `SELECT count(*) FROM consent_doi_token WHERE contact_id = $1`, 0},
			{"confirm-details links", `SELECT count(*) FROM confirm_token WHERE contact_id = $1`, 0},
			{"confirm-page submissions", `SELECT count(*) FROM contact_confirm_submission WHERE contact_id = $1`, 0},
			{"suppression entries", `SELECT count(*) FROM erasure_suppression WHERE kind = 'email'`, 1},
			{"erase tombstones", `SELECT count(*) FROM audit_log WHERE action = 'erase' AND entity_id = $1`, 1},
		}
		for _, c := range checks {
			var got int
			args := []any{}
			if strings.Contains(c.query, "$1") {
				args = append(args, contactID)
			}
			if err := tx.QueryRow(ctx, c.query, args...).Scan(&got); err != nil {
				return fmt.Errorf("%s: %w", c.what, err)
			}
			if got != c.want {
				return fmt.Errorf("%s = %d, want %d", c.what, got, c.want)
			}
		}
		var name string
		if err := tx.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, contactID).Scan(&name); err != nil {
			return err
		}
		if name != "Erased Subject" {
			return fmt.Errorf("contact name = %q", name)
		}
		var rawLeft int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM raw_capture WHERE payload::text ILIKE '%' || $1 || '%'`, subjectEmail).Scan(&rawLeft); err != nil {
			return err
		}
		if rawLeft != 0 {
			return fmt.Errorf("raw capture still mentions the subject (%d rows)", rawLeft)
		}
		// The tombstone certifies WITHOUT re-storing: no address, no name.
		var piiInTombstone bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM audit_log WHERE action = 'erase' AND entity_id = $1
			  AND (after::text ILIKE '%' || $2 || '%' OR after::text ILIKE '%Selma%'))`,
			contactID, subjectEmail).Scan(&piiInTombstone); err != nil {
			return err
		}
		if piiInTombstone {
			return errors.New("the erasure tombstone re-stores PII")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestErasureRemovesPIIEverywhereAndSticksViaSuppression(t *testing.T) {
	e := Setup(t)
	contactID := seedSubject(t, e)
	admin := e.Admin()

	// The SAR sees the full picture BEFORE erasure — Art. 15 assembly.
	pkg, err := privacy.AssembleSAR(admin, e.DB(), ids.From[ids.ContactKind](contactID))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Subject["full_name"] != "Selma Subject" || len(pkg.Emails) != 1 ||
		len(pkg.Activities) != 1 || len(pkg.RawCapture) != 1 {
		t.Fatalf("SAR incomplete: subject=%v emails=%d activities=%d raw=%d",
			pkg.Subject["full_name"], len(pkg.Emails), len(pkg.Activities), len(pkg.RawCapture))
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(admin, contactID, "test"); err != nil {
		t.Fatal(err)
	}

	assertSubjectErased(t, e, contactID)

	// Re-capture of the erased address is skipped, not resurrected.
	sink := capture.NewSink(e.DB())
	connCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	connCtx = principal.WithCorrelationID(connCtx, ids.NewV7())
	connCtx = principal.WithActor(connCtx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:test",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"lead": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	_, err = sink.Upsert(connCtx, connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: "apollo", SourceID: "l-1"},
		Fields:     capture.LeadFields{FullName: "Selma Subject", Email: subjectEmail},
		Source:     "apollo:l-1",
		CapturedBy: "connector:test",
	})
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("re-capture of an erased subject → %v, want ErrSkip", err)
	}

	// A subject under legal hold cannot be erased.
	held := seedSubject(t, e)
	err = database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE contact SET legal_hold = true WHERE id = $1`, held)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(e.DB()).EraseContact(admin, held, "test"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("erasing a held subject → %v, want ErrConflict", err)
	}
}

// The token resolve is the SOLE authorization decision on the anonymous
// preference edge, so the erasure that certifies a subject gone has to end
// there too: an archived or forwarded List-Unsubscribe URL must stop
// answering. Otherwise the erased subject keeps a live surface that reads
// their surviving consent state and writes fresh contact_consent,
// consent_event, audit and outbox rows against them — through the very
// capability the erasure destroyed the record of.
func TestErasureRetiresTheSubjectsPreferenceToken(t *testing.T) {
	e := Setup(t)
	contactID := seedSubject(t, e)
	token := seededPreferenceToken(contactID)
	store := consent.NewStore(e.DB())

	// The fixture is live first, so the assertion below measures the erasure
	// and not a token that never worked.
	ref, err := store.ResolvePreferenceToken(context.Background(), token)
	if err != nil {
		t.Fatalf("the seeded token does not resolve before erasure: %v", err)
	}
	if ref.ContactID.UUID != contactID {
		t.Fatalf("the token resolves to contact %s, want the seeded subject %s", ref.ContactID.UUID, contactID)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contactID, "art-17"); err != nil {
		t.Fatal(err)
	}

	// ABSENT, not merely refused: the same answer an unknown token reads as,
	// so the edge never becomes a "this subject was erased" oracle.
	if _, err := store.ResolvePreferenceToken(context.Background(), token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the erased subject's token still resolves → %v, want ErrNotFound", err)
	}
}

// TestErasurePreservesActivityUnderTransitiveHold pins F-011: a contact is
// not itself held, but one of its subject-only activities is ALSO linked to
// a company under legal_hold. Retention freezes such an activity
// transitively ("a hold on the subject must cover the evidence about them"),
// so the contact-erase cascade must not destroy it either. A sibling
// subject-only activity with no hold IS redacted, proving the predicate
// discriminates rather than blanket-skipping.
func TestErasurePreservesActivityUnderTransitiveHold(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	contactID := ids.NewV7()
	heldActivity := ids.NewV7()
	freeActivity := ids.NewV7()
	companyID := ids.NewV7()

	err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact (id, full_name, first_name, source, captured_by)
			 VALUES ($1, 'Held Subject', 'Held', 'manual', 'human:x')`, contactID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO company (id, display_name, legal_hold, source, captured_by)
			 VALUES ($1, 'Counterparty GmbH', true, 'manual', 'human:x')`, companyID); err != nil {
			return err
		}
		// The held-evidence note: subject-only to the contact, but also linked
		// to the company under legal_hold. A 'note' kind (not correspondence)
		// carries no statutory floor, so its survival here is attributable to
		// the legal_hold alone, not the GoBD floor this binary also arms.
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			 VALUES ($1, 'note', 'Contract terms', 'The signed terms are attached.', now(), 'manual', 'human:x')`,
			heldActivity); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity_link (activity_id, entity_type, contact_id)
			 VALUES ($1, 'contact', $2)`, heldActivity, contactID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity_link (activity_id, entity_type, company_id)
			 VALUES ($1, 'company', $2)`, heldActivity, companyID); err != nil {
			return err
		}
		// The sibling note: subject-only, NOT transitively held, and (being a
		// note) not floor-shielded either — so it IS redacted.
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			 VALUES ($1, 'note', 'Lunch?', 'Free on Friday?', now(), 'manual', 'human:x')`,
			freeActivity); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO activity_link (activity_id, entity_type, contact_id)
			 VALUES ($1, 'contact', $2)`, freeActivity, contactID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(admin, contactID, "test"); err != nil {
		t.Fatalf("erasing an unheld contact → %v", err)
	}

	err = database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		// The contact's own PII is still erased — the cascade ran.
		var name string
		if err := tx.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, contactID).Scan(&name); err != nil {
			return err
		}
		if name != "Erased Subject" {
			return fmt.Errorf("contact not erased: full_name = %q", name)
		}
		// The transitively-held note is untouched: retention would freeze it,
		// so the erase cascade must too.
		var subject string
		var body *string
		if err := tx.QueryRow(ctx,
			`SELECT subject, body FROM activity WHERE id = $1`, heldActivity).Scan(&subject, &body); err != nil {
			return err
		}
		if subject != "Contract terms" || body == nil || *body != "The signed terms are attached." {
			return fmt.Errorf("held evidence was destroyed: subject=%q body=%v", subject, body)
		}
		// The sibling note carries no hold and IS redacted.
		var freeSubject string
		var freeBody *string
		if err := tx.QueryRow(ctx,
			`SELECT subject, body FROM activity WHERE id = $1`, freeActivity).Scan(&freeSubject, &freeBody); err != nil {
			return err
		}
		if freeSubject != "Erased Subject" || freeBody != nil {
			return fmt.Errorf("unheld subject-only note not redacted: subject=%q body=%v", freeSubject, freeBody)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEraseContactHonoursCommercialCorrespondenceFloor pins F-012: the
// contact-erase cascade applies the SAME statutory correspondence floor the
// retention activity selectors do. With the GoBD floor armed in this binary
// (retention_jurisdiction_integration_test.go's init), a recent email is a
// Handelsbrief the floor shields — erasing the contact it hangs off must not
// null its body. A same-age note is not correspondence and IS redacted, so
// the floor discriminates rather than blanket-skipping the whole timeline.
func TestEraseContactHonoursCommercialCorrespondenceFloor(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()

	contactID := ids.NewV7()
	email := ids.NewV7()
	note := ids.NewV7()

	err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact (id, full_name, first_name, source, captured_by)
			 VALUES ($1, 'Floored Subject', 'Floored', 'manual', 'human:x')`, contactID); err != nil {
			return err
		}
		// A recent external email — commercial correspondence within the floor.
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			 VALUES ($1, 'email', 'Invoice 2026-0042', 'Please find the invoice attached.', now() - interval '400 days', 'manual', 'human:x')`,
			email); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity_link (activity_id, entity_type, contact_id)
			 VALUES ($1, 'contact', $2)`, email, contactID); err != nil {
			return err
		}
		// A same-age internal note — no statutory floor.
		if _, err := tx.Exec(ctx,
			`INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
			 VALUES ($1, 'note', 'Internal jotting', 'Chase them next week.', now() - interval '400 days', 'manual', 'human:x')`,
			note); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO activity_link (activity_id, entity_type, contact_id)
			 VALUES ($1, 'contact', $2)`, note, contactID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// The email is a Handelsbrief because it concerns a concluded transaction;
	// the note is not correspondence at all. A165 narrowed the floor to that
	// distinction, so the deal is what the shielding now rests on.
	e.SeedWonDealLinkedTo(t, email)

	if err := privacy.NewEraser(e.DB()).EraseContact(admin, contactID, "test"); err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}

	err = database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var name string
		if err := tx.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, contactID).Scan(&name); err != nil {
			return err
		}
		if name != "Erased Subject" {
			return fmt.Errorf("contact not erased: full_name = %q", name)
		}
		// The Handelsbrief is shielded by the floor.
		var subject string
		var body *string
		if err := tx.QueryRow(ctx, `SELECT subject, body FROM activity WHERE id = $1`, email).Scan(&subject, &body); err != nil {
			return err
		}
		if subject != "Invoice 2026-0042" || body == nil || *body != "Please find the invoice attached." {
			return fmt.Errorf("correspondence destroyed below the GoBD floor: subject=%q body=%v", subject, body)
		}
		// The note carries no floor and IS redacted.
		var noteSubject string
		var noteBody *string
		if err := tx.QueryRow(ctx, `SELECT subject, body FROM activity WHERE id = $1`, note).Scan(&noteSubject, &noteBody); err != nil {
			return err
		}
		if noteSubject != "Erased Subject" || noteBody != nil {
			return fmt.Errorf("unfloored note not redacted: subject=%q body=%v", noteSubject, noteBody)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
