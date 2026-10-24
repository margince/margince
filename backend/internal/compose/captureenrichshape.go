// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The shape of the signature reply: what the retry validator checks and what
// the provider is constrained to at generation.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// signatureShapeValid is the retry validator: parseable fields from the closed
// vocabulary. Evidence checking stays in gateEvidence, because the model must
// not certify itself.
func signatureShapeValid(text string) error {
	var parsed struct {
		Fields []extractedField `json:"fields"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &parsed); err != nil {
		return fmt.Errorf("output is not the required JSON shape: %w", err)
	}
	if err := signatureNameShapeValid(text); err != nil {
		return err
	}
	for _, f := range parsed.Fields {
		// The field name is MODEL output. It is bounded before it reaches an
		// error, which is logged and fed back into the prompt on a retry.
		if !enrichFieldNames[f.Field] {
			return fmt.Errorf("field %q is not in the allowed set", clampToken(f.Field))
		}
	}
	return nil
}

// signatureEnrichSchema is the generation-time shape guardrail. The field enum
// says in its description that phone repeats. A structured reply follows the
// schema at least as closely as the prose.
func signatureEnrichSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			laneFields: schema.Array(schema.Object(
				map[string]schema.Node{
					extractionFieldKey: schema.Enum(signatureFieldNames...).
						Describe("phone once per number the signature lists; every other field at most once"),
					"value":            schema.String(),
					"evidence_snippet": schema.String(),
					"confidence":       schema.Number(),
				},
				"field", "value", "evidence_snippet", "confidence",
			)),
			signatureNameKey: signatureNameSchema(),
		},
		"fields",
	))
}
