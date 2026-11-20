// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// criterionKeyFallback is the key stem of a label with no a-z letters or
// digits, such as one written wholly in another script.
const criterionKeyFallback = "criterion"

// criterionKeyStemMax leaves room under maxCriterionKey for a clash suffix.
const criterionKeyStemMax = 48

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

// mintCriterionKey answers the key of a criterion created without one: the
// label's stem, or the stem with the lowest free number among live keys.
// The caller holds the stage's criteria lock, so the answer stays free.
func mintCriterionKey(ctx context.Context, tx pgx.Tx, stageID ids.StageID, label string) (string, error) {
	stem := criterionKeyFromLabel(label)
	for next := 1; next <= 1000; next++ {
		candidate := stem
		if next > 1 {
			candidate = fmt.Sprintf("%s_%d", stem, next)
		}
		var taken bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM stage_exit_criterion
				 WHERE stage_id = $1 AND key = $2 AND archived_at IS NULL)`,
			stageID, candidate).Scan(&taken); err != nil {
			return "", fmt.Errorf("check the minted criterion key: %w", err)
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("every key number up to 1000 is in use for the stem %q", stem)
}
