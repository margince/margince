// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	_, err := parseBodyTemplate(*template, deref(contentType))
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

// sealHeadersFor seals the requested header set against the stored one, read
// under the caller's row lock. The caller writes it through the patch and
// audits the returned names in place of the sealed values.
func (s *Store) sealHeadersFor(ctx context.Context, tx pgx.Tx, id ids.UUID, in []CustomHeaderInput) (sealed []storedHeader, replaced []string, err error) {
	var raw []byte
	if err := tx.QueryRow(ctx, "SELECT custom_headers FROM webhook_subscription WHERE id = $1", id).Scan(&raw); err != nil {
		return nil, nil, err
	}
	var existing []storedHeader
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, nil, err
	}
	if sealed, err = s.cipher.sealHeaders(id, in, existing); err != nil {
		return nil, nil, err
	}
	return sealed, namesWithNewValues(sealed, in), nil
}

// namesWithNewValues names each stored header the request gave a value, so
// the audit says a credential changed without saying what it became.
func namesWithNewValues(sealed []storedHeader, in []CustomHeaderInput) []string {
	replaced := []string{}
	for i, h := range in {
		if h.Value != nil {
			replaced = append(replaced, sealed[i].Name)
		}
	}
	return replaced
}
