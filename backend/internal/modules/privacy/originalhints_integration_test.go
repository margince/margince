// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/originalstore"
)

func TestOriginalHintSQLNeverExcludesAnExistingContentMatch(t *testing.T) {
	e := setupSARIdentifiers(t)
	ctx := originalstore.Bind(e.ctx, e.db, blobstore.NewMemory())
	mentions := []string{"a@b", "sam@example.test", "A.CASE@EXAMPLE.TEST", "long.name+tag@other.example.test", "Kate@example.test", "İbrahim@example.test", "søren@example.test"}
	payload, err := json.Marshal(map[string]string{"text": strings.Join(mentions, " ") + strings.Repeat("padding ", originalstore.InlineLimit/5)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		_, ref, err := originalstore.PrepareTx(ctx, tx, payload)
		if err != nil {
			return err
		}
		if ref == nil || ref.Subjects == nil {
			t.Fatal("fixture did not create searchable external evidence")
		}
		positives := 0
		for _, email := range append(mentions, "kate@example.test", "ibrahim@example.test", "nobody@unrelated.test") {
			args := pgx.NamedArgs{"payload": payload, "source": "fixture", "emails": []string{email}, "providers": []string{}, "accounts": []string{}, "subject": e.contact.String(), "email_queries": originalstore.EmailHintQueries([]string{email}), "object": ref}
			var matched, candidate bool
			if err := tx.QueryRow(ctx, "SELECT "+rawSubjectMatchSQL("@payload::jsonb", "@source::text"), args).Scan(&matched); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, "SELECT "+externalOriginalCandidateSQL(rawSubjectScope{contact: e.contact, emails: []string{email}}, args)+" FROM (SELECT @object::jsonb AS payload_object,@source::text AS source_system) original", args).Scan(&candidate); err != nil {
				return err
			}
			if matched {
				positives++
				if !candidate {
					t.Errorf("search hints excluded the SQL match for %q", email)
				}
			}
			if email == "nobody@unrelated.test" && candidate {
				t.Error("the SQL filter never excludes unrelated evidence")
			}
		}
		if positives < 5 {
			t.Error("fixture did not exercise actual content matches")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
