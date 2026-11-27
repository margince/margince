// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// purposeCatalogLock serialises the label check and the insert of a purpose,
// so two admins naming one label at once cannot both pass the check.
const purposeCatalogLock = "consent.consent_purpose.create"

// CreatePurpose defines one purpose. A purpose is compliance configuration, so
// its grant is consent_config: every outreach decision is judged against it.
func (s *Store) CreatePurpose(ctx context.Context, key, label string, requiresDOI bool, frequency string) (Purpose, error) {
	if err := auth.Require(ctx, "consent_config", principal.ActionCreate); err != nil {
		return Purpose{}, err
	}
	key = normalizedPurposeKey(key)
	label = strings.TrimSpace(label)
	if key == "" || label == "" {
		return Purpose{}, &ValidationError{Field: fieldKey, Reason: "key and label are required"}
	}
	if !commsauthz.PurposeKeyPattern.MatchString(key) {
		return Purpose{}, &ValidationError{Field: fieldKey, Reason: "use only the letters a to z, the digits 0 to 9 and underscores, such as marketing_email"}
	}
	if frequency == "" {
		frequency = string(crmcontracts.ConsentPurposeFrequencyMonthly)
	}
	if !crmcontracts.ConsentPurposeFrequency(frequency).Valid() {
		return Purpose{}, &ValidationError{Field: "frequency", Reason: "pick weekly, monthly or quarterly"}
	}
	var p Purpose
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, purposeCatalogLock); err != nil {
			return err
		}
		if err := refuseConfusableLabel(ctx, tx, label); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO consent_purpose (key, label, requires_double_opt_in, frequency)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT ON CONSTRAINT consent_purpose_key_unique DO NOTHING
			RETURNING id, key, label, requires_double_opt_in, class, frequency, created_at`,
			key, label, requiresDOI, frequency).
			Scan(&p.ID, &p.Key, &p.Label, &p.RequiresDoubleOptIn, &p.Class, &p.Frequency, &p.CreatedAt)
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return purposeKeyTaken(ctx, tx, key)
	})
	return p, err
}

// refuseConfusableLabel refuses a label that reads as a listed purpose's label
// or key. A contact's consent and the pages a contact opens name a purpose by
// its label alone. Two that read alike cannot be told apart there, and an
// archived purpose shows on none of them.
func refuseConfusableLabel(ctx context.Context, tx pgx.Tx, label string) error {
	rows, err := tx.Query(ctx, `SELECT key, label FROM consent_purpose WHERE archived_at IS NULL`)
	if err != nil {
		return fmt.Errorf("consent: reading the listed purposes: %w", err)
	}
	listed, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Key, Label string }])
	if err != nil {
		return fmt.Errorf("consent: reading the listed purposes: %w", err)
	}
	wanted := purposeReading(label)
	for _, purpose := range listed {
		if purposeReading(purpose.Label) == wanted || purposeReading(purpose.Key) == wanted {
			return &ValidationError{Field: "label", Reason: fmt.Sprintf(
				"this label reads like %q; choose another label to tell the two purposes apart", purpose.Label)}
		}
	}
	return nil
}

// purposeReading is a label or key as a reader tells it apart: case, spacing,
// underscores and the encoding of its accents aside. Case maps up, then folds, so
// ΌΡΟΣ reads as Όρος, STRAẞE and STRASSE as Straße, and ı as i.
// privacy.purposes.tsx mirrors it.
func purposeReading(text string) string {
	upper := cases.Upper(language.Und).String(norm.NFC.String(text))
	return strings.Join(strings.Fields(strings.ReplaceAll(cases.Fold().String(upper), "_", " ")), " ")
}

// purposeKeyTaken explains a refused key. An archived purpose keeps its key
// because its consent history stays attached to it, so no second purpose takes it.
func purposeKeyTaken(ctx context.Context, tx pgx.Tx, key string) error {
	var archived bool
	if err := tx.QueryRow(ctx,
		`SELECT archived_at IS NOT NULL FROM consent_purpose WHERE key = $1`, key).Scan(&archived); err != nil {
		return fmt.Errorf("consent: reading the purpose that holds key %q: %w", key, err)
	}
	if archived {
		return fmt.Errorf("%w: the key %q belongs to an archived purpose and its consent history stays with it; restore that purpose instead of creating another", apperrors.ErrConflict, key)
	}
	return fmt.Errorf("%w: a purpose with the key %q already exists", apperrors.ErrConflict, key)
}
