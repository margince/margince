// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FoldAudience is how an audience is compared: trimmed, no case, no accents,
// and đ as d, so "Bếp" and "bep" are one audience.
func FoldAudience(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "đ", "d"), "Đ", "D")
	out, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s)
	if err != nil {
		out = s
	}
	return strings.ToLower(strings.TrimSpace(out))
}

// audienceMatcher is the set of folded audiences a caller may read. A nil
// matcher is "no filter".
type audienceMatcher map[string]bool

// newAudienceMatcher answers nil when the caller named no audience, so the ask
// is unchanged. Blank values name nothing and are dropped.
func newAudienceMatcher(audiences []string) audienceMatcher {
	var m audienceMatcher
	for _, a := range audiences {
		if f := FoldAudience(a); f != "" {
			if m == nil {
				m = audienceMatcher{}
			}
			m[f] = true
		}
	}
	return m
}

// allows fails closed: a document that states no audience is not allowed.
func (m audienceMatcher) allows(meta map[string]string) bool {
	f := FoldAudience(meta["audience"])
	return f != "" && m[f]
}

// allowedDocumentsIn lists the live documents of the corpus the filter allows.
// The ranking query takes this list, so a passage of any other document is never
// selected, never reaches the model and never reaches the quote check.
func allowedDocumentsIn(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, m audienceMatcher) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT id::text, meta FROM knowledge_document WHERE corpus_id = $1 AND archived_at IS NULL`, corpusID)
	if err != nil {
		return nil, fmt.Errorf("list the documents an audience may read: %w", err)
	}
	defer rows.Close()
	allowed := []string{} // empty, not nil: "nothing is allowed" must not read as "no filter"
	for rows.Next() {
		var id string
		var meta map[string]string
		if err := rows.Scan(&id, &meta); err != nil {
			return nil, err
		}
		if m.allows(meta) {
			allowed = append(allowed, id)
		}
	}
	return allowed, rows.Err()
}
