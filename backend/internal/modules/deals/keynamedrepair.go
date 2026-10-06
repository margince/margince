// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Renaming imported deals whose name is still the source system's key.
//
// The deal keeps no source key, so only the export can say which names are
// keys; a name that no longer equals its key was changed by a person and stays.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// KeyNamedDeal is one row of a source export: the key a deal was imported
// under and the title the source system shows for it.
type KeyNamedDeal struct{ SourceSystem, SourceKey, SourceTitle string }

// KeyNameOutcome says what the repair did, or would do, with one export row.
type KeyNameOutcome string

const (
	// KeyNameRenamed: the one matching deal now carries its new name.
	KeyNameRenamed KeyNameOutcome = "renamed"
	// KeyNameWouldRename: a dry run found the deal and the name it would take.
	KeyNameWouldRename KeyNameOutcome = "would-rename"
	// KeyNameNoMatch: no live deal of that source is still named by the key.
	KeyNameNoMatch KeyNameOutcome = "no-match"
	// KeyNameAmbiguous: more than one live deal shares the key, so none is touched.
	KeyNameAmbiguous KeyNameOutcome = "ambiguous"
	// KeyNameNoCompany: neither a title nor a company gives a better name.
	KeyNameNoCompany KeyNameOutcome = "no-company"
)

// KeyNameResult is the repair's answer for one export row.
type KeyNameResult struct {
	Entry   KeyNamedDeal
	DealID  ids.DealID // zero unless exactly one deal matched
	To      string     // empty when Outcome is not renamed / would-rename
	Outcome KeyNameOutcome
}

// keyNameReplacement picks the name a key-named deal should carry, and reports
// false when nothing better than the key is known.
func keyNameReplacement(key, title, company, stage string) (string, bool) {
	if t := strings.TrimSpace(title); t != "" && t != key {
		return t, true
	}
	if c := strings.TrimSpace(company); c != "" {
		return c + " · " + stage, true
	}
	return "", false
}

// RenameKeyNamedDeals resolves each export row to the one live deal still named
// by its key and, when apply is set, renames it through the ordinary update
// path. One transaction per row, so a failure leaves earlier renames committed
// and a re-run finds only what is left.
func (s *Store) RenameKeyNamedDeals(ctx context.Context, entries []KeyNamedDeal, apply bool) ([]KeyNameResult, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]KeyNameResult, 0, len(entries))
	for _, entry := range entries {
		var result KeyNameResult
		err := s.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			result, err = s.renameKeyNamedDealTx(ctx, tx, entry, apply, active)
			return err
		})
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

type keyNamedCandidate struct {
	id             ids.DealID
	company, stage string
}

func (s *Store) renameKeyNamedDealTx(ctx context.Context, tx pgx.Tx, entry KeyNamedDeal,
	apply bool, active []fieldcatalog.Column,
) (KeyNameResult, error) {
	result := KeyNameResult{Entry: entry}
	candidates, err := keyNamedCandidates(ctx, tx, entry)
	if err != nil {
		return result, err
	}
	switch len(candidates) {
	case 0:
		result.Outcome = KeyNameNoMatch
		return result, nil
	case 1:
	default:
		result.Outcome = KeyNameAmbiguous
		return result, nil
	}
	match := candidates[0]
	result.DealID = match.id
	to, ok := keyNameReplacement(entry.SourceKey, entry.SourceTitle, match.company, match.stage)
	if !ok {
		result.Outcome = KeyNameNoCompany
		return result, nil
	}
	result.To = to
	if !apply {
		result.Outcome = KeyNameWouldRename
		return result, nil
	}
	_, err = s.updateDealInTx(ctx, tx, match.id, UpdateDealInput{
		Name:  &to,
		Trail: storekit.AuditTrail{Evidence: map[string]any{"repair": "imported_key_name", "source_key": entry.SourceKey}},
	}, active)
	if err != nil {
		return result, fmt.Errorf("rename the deal imported as %q: %w", entry.SourceKey, err)
	}
	result.Outcome = KeyNameRenamed
	return result, nil
}

// keyNamedCandidates locks every live deal of the entry's source still named by
// its key. FOR UPDATE OF d: a person renaming one between this read and the
// rename waits, and a person who renamed it first takes it out of the match.
func keyNamedCandidates(ctx context.Context, tx pgx.Tx, entry KeyNamedDeal) ([]keyNamedCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id, coalesce(c.display_name, ''), st.name
		  FROM deal d
		  JOIN stage st ON st.id = d.stage_id
		  LEFT JOIN company c ON c.id = d.company_id
		 WHERE d.source_system = $1 AND d.name = $2 AND d.archived_at IS NULL
		 FOR UPDATE OF d`, entry.SourceSystem, entry.SourceKey)
	if err != nil {
		return nil, fmt.Errorf("find the deals imported as %q: %w", entry.SourceKey, err)
	}
	candidates, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (keyNamedCandidate, error) {
		var c keyNamedCandidate
		return c, r.Scan(&c.id, &c.company, &c.stage)
	})
	if err != nil {
		return nil, fmt.Errorf("read the deals imported as %q: %w", entry.SourceKey, err)
	}
	return candidates, nil
}
