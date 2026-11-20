// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// declaredPurpose is the consent purpose a send's author named, and that
// purpose's class at send time. The engine lets a send go under the class it
// declares, so this record is what shows a misclassified send afterwards.
type declaredPurpose struct {
	key   string
	class string
}

// declaredPurposeOf is the one key a send names: the consent key, or the
// marketing purpose when only that was given. Two keys that disagree are
// refused at staging, and the record keeps the consent key the request named.
func declaredPurposeOf(consentPurpose, marketingPurpose string) declaredPurpose {
	named := commsauthz.Request{LegacyPurposeKey: consentPurpose, MarketingPurpose: marketingPurpose}
	if canonical, agree := named.CanonicalPurpose(); agree {
		named = canonical
	}
	return declaredPurpose{key: commsauthz.NormalizedPurposeKey(named.LegacyPurposeKey)}
}

// resolveDeclaredClass reads the class of the declared key in the send's own
// transaction. A key no purpose defines keeps an empty class; the engine
// refuses such a send, and a send that commits under observe still names it.
func resolveDeclaredClass(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if in.declared.key == "" {
		return nil
	}
	err := tx.QueryRow(ctx, `SELECT class FROM consent_purpose WHERE key = $1`, in.declared.key).Scan(&in.declared.class)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// declaredPurposeInsertFragment stamps a send's declared purpose; every other
// row keeps both columns NULL.
func declaredPurposeInsertFragment(declared declaredPurpose, base []any) (cols, holders string, args []any) {
	if declared.key == "" {
		return "", "", base
	}
	args = append(append(make([]any, 0, len(base)+2), base...), declared.key, nullIfEmpty(declared.class))
	return ", consent_purpose, consent_class", storekit.SQLf(", $%d, $%d", len(args)-1, len(args)), args
}

// auditDeclaredPurpose names the declared purpose in the send's audit row.
func auditDeclaredPurpose(after map[string]any, declared declaredPurpose) {
	if declared.key == "" {
		return
	}
	after[fieldConsentPurpose] = declared.key
	after[fieldConsentClass] = nullIfEmpty(declared.class)
}

const (
	fieldConsentPurpose = "consent_purpose"
	fieldConsentClass   = "consent_class"
)
