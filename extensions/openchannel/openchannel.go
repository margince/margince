// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"embed"

	"github.com/margince/margince/backend/pkg/extension"
)

// migrations carries the unit's SQL layer INTO the binary. Shipping the
// directory without setting the field below passes every gate — the SQL
// blessed, the catalog checked — and then boots against a database where the
// tables were never created.
//
//go:embed migrations
var migrations embed.FS

// New returns the unit's declaration: inert data, holding no handle into the
// core. Every field is a literal, because the operator manifest is derived from
// this function's AST without compiling it.
func New() extension.Extension {
	return extension.Extension{
		Name:    "openchannel",
		Version: "1.0.0",
		// User scope, and it is the whole authority story for the anonymous
		// edge: the secret an arriving request is verified against is the
		// OWNER's, so a request that verifies is one that member agreed to
		// receive. Boot refuses an inbound endpoint naming a secret the unit
		// did not declare, so this entry is what makes the edge mountable at
		// all rather than a nicety for the manifest.
		Secrets: []extension.SecretsRequest{
			{Key: "inbound", Scope: extension.SecretScopeUser},
		},
		Migrations: migrations,
	}
}
