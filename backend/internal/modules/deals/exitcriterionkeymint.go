// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// criterionKeyFallback is the key stem of a label with no a-z letters or
// digits, such as one written wholly in another script.
const criterionKeyFallback = "criterion"

// criterionKeyStemMax leaves room under maxCriterionKey for a clash suffix.
const criterionKeyStemMax = 48

// mintInsertAttempts bounds the retries after a lost race on a minted key.
const mintInsertAttempts = 5

const liveCriterionKeyIndex = "stage_exit_criterion_stage_key_live_ux"

// criterionKeyFromLabel turns a label into the stem of its key. Accents fold
// away and anything but a-z and digits becomes one underscore. A leading digit
// gets the fallback in front, because a key starts with a letter.
func criterionKeyFromLabel(label string) string {
	var out strings.Builder
	pendingGap := false
	for _, r := range norm.NFD.String(strings.ToLower(label)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if pendingGap && out.Len() > 0 {
				out.WriteByte('_')
			}
			pendingGap = false
			out.WriteRune(r)
		case unicode.Is(unicode.Mn, r):
		default:
			pendingGap = true
		}
	}
	stem := out.String()
	if len(stem) > criterionKeyStemMax {
		stem = strings.TrimRight(stem[:criterionKeyStemMax], "_")
	}
	switch {
	case stem == "":
		return criterionKeyFallback
	case stem[0] >= '0' && stem[0] <= '9':
		return criterionKeyFallback + "_" + stem
	}
	return stem
}

// mintCriterionKey answers the key of a criterion created without one. It is
// the label's stem, or the stem plus one above the highest number in use among
// the stage's live keys. One query reads it, so no ceiling caps same-stem keys.
func mintCriterionKey(ctx context.Context, tx pgx.Tx, stageID ids.StageID, label string) (string, error) {
	stem := criterionKeyFromLabel(label)
	var bareTaken bool
	var highest int64
	// The stem holds only a-z, 0-9 and `_`, so it is safe inside the pattern.
	// The suffix is capped at six digits so the cast cannot overflow.
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(bool_or(key = $2::text), false),
		       COALESCE(max(substring(key from '_([0-9]{1,6})$')::bigint) FILTER (WHERE key <> $2::text), 1)
		  FROM stage_exit_criterion
		 WHERE stage_id = $1 AND archived_at IS NULL
		   AND (key = $2::text OR key ~ ('^' || $2::text || '_[0-9]{1,6}$'))`,
		stageID, stem).Scan(&bareTaken, &highest)
	if err != nil {
		return "", fmt.Errorf("read the criterion keys sharing %q: %w", stem, err)
	}
	if !bareTaken {
		return stem, nil
	}
	return fmt.Sprintf("%s_%d", stem, highest+1), nil
}

// insertCriterionKeyed inserts the row. A key the caller chose that loses a
// race on the live-key index answers criterion_key_taken. A minted key asks
// `mint` again, a bounded number of times, because the winner is now visible.
func insertCriterionKeyed(
	ctx context.Context, tx pgx.Tx, in CreateCriterionInput, kind CriterionKind, position int,
	mint func() (string, error),
) (ids.ExitCriterionID, error) {
	attempts := 1
	if in.MintKey {
		attempts = mintInsertAttempts
	}
	var id ids.ExitCriterionID
	for range attempts {
		if in.MintKey {
			key, err := mint()
			if err != nil {
				return id, err
			}
			in.Key = key
		}
		var err error
		if id, err = insertCriterionSavepointed(ctx, tx, in, kind, position); !isLiveCriterionKeyClash(err) {
			return id, err
		}
	}
	return id, &values.ParseError{
		Field: criterionKeyField, Code: codeCriterionKeyTaken,
		Message: "this stage already asks for a criterion under that key",
	}
}

// insertCriterionSavepointed runs the insert inside a savepoint, so a unique
// violation leaves the surrounding transaction usable for a retry.
func insertCriterionSavepointed(
	ctx context.Context, tx pgx.Tx, in CreateCriterionInput, kind CriterionKind, position int,
) (ids.ExitCriterionID, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return ids.ExitCriterionID{}, fmt.Errorf("open the criterion savepoint: %w", err)
	}
	id, err := insertCriterion(ctx, sp, in, kind, position)
	if err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return id, errors.Join(err, rbErr)
		}
		return id, err
	}
	if err := sp.Commit(ctx); err != nil {
		return id, fmt.Errorf("release the criterion savepoint: %w", err)
	}
	return id, nil
}

func isLiveCriterionKeyClash(err error) bool {
	constraint, ok := storekit.UniqueViolation(err)
	return ok && constraint == liveCriterionKeyIndex
}
