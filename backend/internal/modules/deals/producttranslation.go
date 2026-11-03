// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A product's name and description in further languages. An offer line added
// in one of them takes its text from here, once, when it is added. A later edit
// never reaches a line already on an offer.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	productTranslationTable = "product_translation"
	languageField           = "language"
	translationNameLimit    = 500
	translationTextLimit    = 4000
	translationTooLong      = "too_long"
)

var languageShape = regexp.MustCompile(`^[a-z]{2}$`)

// checkLanguage refuses anything but a lower-case ISO 639-1 code.
func checkLanguage(language string) error {
	if !languageShape.MatchString(language) {
		return &values.ParseError{Field: languageField, Code: "invalid_language", Message: "language is an ISO 639-1 code, for example de"}
	}
	return nil
}

// ListProductTranslations answers a product's text in every further language.
func (s *Store) ListProductTranslations(ctx context.Context, id ids.ProductID) ([]crmcontracts.ProductTranslation, error) {
	if err := auth.Require(ctx, "product", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.ProductTranslation
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := readProduct(ctx, tx, id, storekit.IncludeArchived); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT language, name, description, version, updated_at
			FROM product_translation WHERE product_id = $1 ORDER BY language`, id)
		if err != nil {
			return fmt.Errorf("list product translations: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanProductTranslation)
		return err
	})
	if out == nil {
		out = []crmcontracts.ProductTranslation{}
	}
	return out, err
}

func scanProductTranslation(row pgx.CollectableRow) (crmcontracts.ProductTranslation, error) {
	var t crmcontracts.ProductTranslation
	var updated time.Time
	err := row.Scan(&t.Language, &t.Name, &t.Description, &t.Version, &updated)
	t.UpdatedAt = updated
	return t, err
}

// PutProductTranslation sets a product's text in one language.
func (s *Store) PutProductTranslation(ctx context.Context, id ids.ProductID, language, name string, description *string) (crmcontracts.ProductTranslation, error) {
	if err := auth.Require(ctx, "product", principal.ActionUpdate); err != nil {
		return crmcontracts.ProductTranslation{}, err
	}
	if err := checkLanguage(language); err != nil {
		return crmcontracts.ProductTranslation{}, err
	}
	name = strings.TrimSpace(name)
	if description != nil && strings.TrimSpace(*description) == "" {
		description = nil
	}
	if !values.HasVisibleText(name) {
		return crmcontracts.ProductTranslation{}, &RequiredFieldError{Field: dealNameColumn}
	}
	if utf8.RuneCountInString(name) > translationNameLimit ||
		(description != nil && utf8.RuneCountInString(*description) > translationTextLimit) {
		return crmcontracts.ProductTranslation{}, &values.ParseError{Field: dealNameColumn, Code: translationTooLong, Message: "the name is at most 500 and the description at most 4000 characters"}
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.ProductTranslation{}, err
	}
	var out crmcontracts.ProductTranslation
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := readProduct(ctx, tx, id, storekit.LiveOnly); err != nil {
			return err
		}
		before, err := readProductTranslation(ctx, tx, id, language)
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO product_translation (product_id, language, name, description, captured_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (product_id, language) DO UPDATE
			   SET name = EXCLUDED.name, description = EXCLUDED.description
			RETURNING language, name, description, version, updated_at`,
			id, language, name, description, by)
		var updated time.Time
		if err := row.Scan(&out.Language, &out.Name, &out.Description, &out.Version, &updated); err != nil {
			return fmt.Errorf("store a product translation: %w", err)
		}
		out.UpdatedAt = updated
		return auditTranslation(ctx, tx, id, before, &out)
	})
	return out, err
}

// DeleteProductTranslation removes a product's text in one language.
func (s *Store) DeleteProductTranslation(ctx context.Context, id ids.ProductID, language string) error {
	if err := auth.Require(ctx, "product", principal.ActionUpdate); err != nil {
		return err
	}
	if err := checkLanguage(language); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := readProduct(ctx, tx, id, storekit.LiveOnly); err != nil {
			return err
		}
		before, err := readProductTranslation(ctx, tx, id, language)
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM product_translation WHERE product_id = $1 AND language = $2`, id, language); err != nil {
			return fmt.Errorf("remove a product translation: %w", err)
		}
		return auditTranslation(ctx, tx, id, before, nil)
	})
}

func readProductTranslation(ctx context.Context, tx pgx.Tx, id ids.ProductID, language string) (*crmcontracts.ProductTranslation, error) {
	rows, err := tx.Query(ctx, `SELECT language, name, description, version, updated_at
		FROM product_translation WHERE product_id = $1 AND language = $2`, id, language)
	if err != nil {
		return nil, fmt.Errorf("read a product translation: %w", err)
	}
	found, err := pgx.CollectRows(rows, scanProductTranslation)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, apperrors.ErrNotFound
	}
	return &found[0], nil
}

// auditTranslation records the change on the product, as a product edit is:
// the translation is part of what the product says about itself.
func auditTranslation(ctx context.Context, tx pgx.Tx, id ids.ProductID, before, after *crmcontracts.ProductTranslation) error {
	image := func(t *crmcontracts.ProductTranslation) map[string]any {
		if t == nil {
			return nil
		}
		return map[string]any{"translation_" + t.Language: map[string]any{dealNameColumn: t.Name, descriptionColumn: t.Description}}
	}
	language := ""
	switch {
	case after != nil:
		language = after.Language
	case before != nil:
		language = before.Language
	}
	beforeImage, afterImage := image(before), image(after)
	if beforeImage == nil {
		beforeImage = map[string]any{"translation_" + language: nil}
	}
	if afterImage == nil {
		afterImage = map[string]any{"translation_" + language: nil}
	}
	if _, err := storekit.Audit(ctx, tx, "update", "product", id.UUID, beforeImage, afterImage); err != nil {
		return fmt.Errorf("audit a product translation: %w", err)
	}
	return nil
}

// ListProductTranslations implements GET /products/{id}/translations.
func (h Handlers) ListProductTranslations(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	list, err := h.store.ListProductTranslations(r.Context(), pathID[ids.ProductKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ProductTranslationListResponse{Data: list})
}

// PutProductTranslation implements PUT /products/{id}/translations/{language}.
func (h Handlers) PutProductTranslation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, language string) {
	var req crmcontracts.PutProductTranslationRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	saved, err := h.store.PutProductTranslation(r.Context(), pathID[ids.ProductKind](id), language, req.Name, req.Description)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, saved)
}

// DeleteProductTranslation implements DELETE /products/{id}/translations/{language}.
func (h Handlers) DeleteProductTranslation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, language string) {
	if err := h.store.DeleteProductTranslation(r.Context(), pathID[ids.ProductKind](id), language); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
