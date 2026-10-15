// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BodyFormatChange is an update to the body format. A field that is not Set
// keeps its stored value; Set with a nil value clears it.
type BodyFormatChange struct {
	TemplateSet    bool
	Template       *string
	ContentTypeSet bool
	ContentType    *string
}

// checkBodyFormat refuses a template the engine cannot render safely, and a
// content type with no template to shape.
func checkBodyFormat(template, contentType *string) error {
	if template == nil {
		if contentType != nil {
			return &BadInputError{Field: fieldBodyContentType, Reason: "a content type needs a body template; the standard envelope is always application/json"}
		}
		return nil
	}
	ct := ""
	if contentType != nil {
		ct = *contentType
	}
	_, err := parseBodyTemplate(*template, ct)
	return err
}

func setBodyFormat(p *storekit.Patch, current Subscription, change *BodyFormatChange) error {
	if change == nil {
		return nil
	}
	template, contentType := current.BodyTemplate, current.BodyContentType
	if change.TemplateSet {
		template = change.Template
		if template == nil && !change.ContentTypeSet {
			contentType = nil
		}
	}
	if change.ContentTypeSet {
		contentType = change.ContentType
	}
	if err := checkBodyFormat(template, contentType); err != nil {
		return err
	}
	if !equalOptional(template, current.BodyTemplate) {
		p.Set(fieldBodyTemplate, current.BodyTemplate, template)
	}
	if !equalOptional(contentType, current.BodyContentType) {
		p.Set(fieldBodyContentType, current.BodyContentType, contentType)
	}
	return nil
}

func equalOptional(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// replaceHeaders writes the sealed header set outside the patch, because
// the patch copies every value it sets into the audit image; the caller
// audits the returned names instead.
func (s *Store) replaceHeaders(ctx context.Context, tx pgx.Tx, id ids.UUID, in []CustomHeaderInput) ([]string, error) {
	var raw []byte
	var version int64
	if err := tx.QueryRow(ctx,
		"SELECT custom_headers, version FROM webhook_subscription WHERE id = $1 AND archived_at IS NULL", id).
		Scan(&raw, &version); err != nil {
		return nil, err
	}
	var existing []storedHeader
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, err
	}
	sealed, err := s.cipher.sealHeaders(id, in, existing)
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx,
		"UPDATE webhook_subscription SET custom_headers = $2 WHERE id = $1 AND archived_at IS NULL AND version = $3",
		id, sealed, version)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, apperrors.ErrVersionSkew
	}
	return headerNames(sealed), nil
}
