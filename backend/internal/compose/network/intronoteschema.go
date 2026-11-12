// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/schema"
)

// noteSchema is the shape the model must answer in.
func noteSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"subject": schema.String(),
			"body":    schema.String(),
		},
		"subject", "body",
	))
}
