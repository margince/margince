// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RoutingSchema is the routing document's JSON Schema: the $defs the editor
// gate holds to this parser, embedded at build time, so the admin screen is
// described by the same document every other editor reads.
func (s *RoutingStore) RoutingSchema(ctx context.Context) ([]byte, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return nil, err
	}
	return RoutingSchemaDocument(), nil
}

// RoutingSchemaDocument is the embedded schema itself, for the gate that holds
// it to the configuration schema it was generated from.
func RoutingSchemaDocument() []byte { return []byte(routingSchemaJSON) }
