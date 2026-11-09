// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Renumber declares that the migration Name shipped as version Old before it
// moved to New with the same bytes. A database that applied Old has already
// run its DDL. Up moves that ledger row to New instead of running it again.
// OldDigest is Digest of the files that shipped as Old, taken from history.
type Renumber struct {
	Old       string
	New       string
	Name      string
	OldDigest string
}

// renumberStep is one ledger row Up moves. dropOld is set when the ledger
// already holds the new version, so only the old row goes.
type renumberStep struct {
	pair    Renumber
	digest  string
	dropOld bool
}

// renumberPlan holds the moves one namespace needs, and the ledger as it reads
// once they are written.
type renumberPlan struct {
	steps  []renumberStep
	ledger map[string]LedgerRow
}

// Reconciled returns the ledger as Up leaves it after moving the declared
// renumbers, or the error Up refuses with. The test-database head probe asks
// it so that it judges a template by the same ledger the migrator would.
func Reconciled(ns Namespace, recorded map[string]LedgerRow) (map[string]LedgerRow, error) {
	plan, err := planRenumbers(ns, recorded)
	return plan.ledger, err
}

// planRenumbers decides every move before any is written. A row moves only
// when its recorded digest equals the pair's OldDigest, and Digest of today's
// source under the old version equals it too. Digest covers the version, name
// and both halves. So the two equalities prove the database ran the bytes that
// now ship as New.
//
// Pairs are taken in old-version order. When two old rows name one new
// version, the first moves and the second is dropped once it proves the same
// content, on every run.
func planRenumbers(ns Namespace, recorded map[string]LedgerRow) (renumberPlan, error) {
	plan := renumberPlan{ledger: maps.Clone(recorded)}
	if len(ns.Renumbered) == 0 {
		return plan, nil
	}
	source := make(map[string]Migration, len(ns.Migrations))
	for _, m := range ns.Migrations {
		source[m.Version] = m
	}
	pairs := slices.SortedFunc(slices.Values(ns.Renumbered), func(a, b Renumber) int {
		return strings.Compare(a.Old, b.Old)
	})
	for _, pair := range pairs {
		old, ok := plan.ledger[pair.Old]
		if !ok {
			continue
		}
		current, err := provenRenumber(ns.Name, pair, old, source)
		if err != nil {
			return renumberPlan{}, err
		}
		step := renumberStep{pair: pair, digest: Digest(current)}
		if row, ok := plan.ledger[pair.New]; ok {
			if row.Name != pair.Name || row.Digest == nil || *row.Digest != step.digest {
				return renumberPlan{}, renumberRefusal(ns.Name, pair,
					"the ledger also holds the new version, and that row does not record today's content")
			}
			step.dropOld = true
		} else {
			plan.ledger[pair.New] = LedgerRow{Name: pair.Name, Digest: &step.digest}
		}
		delete(plan.ledger, pair.Old)
		plan.steps = append(plan.steps, step)
	}
	return plan, nil
}

// provenRenumber returns the migration shipping at pair.New once the old
// ledger row is proved to be its exact content, and refuses otherwise. A NULL
// digest is refused too: here it would vouch for a move, and nothing it
// recorded can prove one.
func provenRenumber(namespace string, pair Renumber, old LedgerRow, source map[string]Migration) (Migration, error) {
	if _, shipped := source[pair.Old]; shipped {
		return Migration{}, renumberRefusal(namespace, pair, "the old version still ships, so it cannot be a renumber")
	}
	current, ok := source[pair.New]
	if !ok || current.Name != pair.Name {
		return Migration{}, renumberRefusal(namespace, pair, "the source at the new version does not carry that name")
	}
	if old.Name != pair.Name {
		return Migration{}, renumberRefusal(namespace, pair, fmt.Sprintf("the ledger recorded the old version as %q", old.Name))
	}
	if old.Digest == nil {
		return Migration{}, renumberRefusal(namespace, pair, "the ledger row records no content digest")
	}
	asOld := current
	asOld.Version = pair.Old
	if Digest(asOld) != pair.OldDigest {
		return Migration{}, renumberRefusal(namespace, pair, "the migration that ships no longer has the declared old content")
	}
	if *old.Digest != pair.OldDigest {
		return Migration{}, renumberRefusal(namespace, pair, "the ledger row records different content from the migration that ships")
	}
	return current, nil
}

func renumberRefusal(namespace string, pair Renumber, why string) error {
	return fmt.Errorf(
		"pgmigrate: %s: %s_%s is declared renumbered to %s, but %s. Nothing was applied. "+
			"Restore this database from a backup taken before it ran a different migration under that name, "+
			"or rebuild it (make dev-fresh)",
		namespace, pair.Old, pair.Name, pair.New, why,
	)
}

// reconcileRenumbered writes one namespace's plan in one transaction, so a
// crash leaves either every row moved or none. Each statement matches the old
// row by version, name and digest, and must touch one row. Any other count
// means the ledger changed under the advisory lock.
func reconcileRenumbered(ctx context.Context, conn *pgx.Conn, table string, plan renumberPlan) error {
	if len(plan.steps) == 0 {
		return nil
	}
	return inTx(ctx, conn, func(tx pgx.Tx) error {
		for _, step := range plan.steps {
			statement := fmt.Sprintf(`UPDATE %s SET version = $2, content_digest = $3
				WHERE version = $1 AND name = $4 AND content_digest = $5`, table)
			args := []any{step.pair.Old, step.pair.New, step.digest, step.pair.Name, step.pair.OldDigest}
			if step.dropOld {
				statement = fmt.Sprintf(`DELETE FROM %s WHERE version = $1 AND name = $2 AND content_digest = $3`, table)
				args = []any{step.pair.Old, step.pair.Name, step.pair.OldDigest}
			}
			tag, err := tx.Exec(ctx, statement, args...)
			if err != nil {
				return fmt.Errorf("%s_%s: %w", step.pair.Old, step.pair.Name, err)
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("%s_%s: changed %d ledger rows, want 1", step.pair.Old, step.pair.Name, tag.RowsAffected())
			}
		}
		return nil
	})
}
