// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The shape a company-read answer is decoded under, and the two closed sets it
// shares with the validator in onboardingsitereadmessage.go.

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// companyReadMessageSchema is the answer every onboarding conversation site
// shares, in the order the model writes it: what kind of answer, the answer,
// then the changes and their sources.
//
// Its two closed sets are READ from the lists the validator holds the reply
// to — the kinds from companyConversationKinds, the fields from the contract's
// own enum — so a value the model may write is exactly one the validator keeps.
//
// The bounds are the validator's alone: at most companyReadChangeLimit
// changes, and no source cited twice. `maxItems` and `uniqueItems` are outside
// the builder's vocabulary (see package schema), so neither is stated here.
var companyReadMessageSchema = schema.Must(schema.Record(
	schema.Field("kind", schema.Enum(companyConversationKinds...)),
	schema.Field("message", schema.String()),
	schema.Field("proposed_changes", schema.Array(schema.Record(
		schema.Field("field", schema.Enum(companyReadChangeFields()...)),
		schema.Field("value", schema.String()),
		schema.Field("reason", schema.String()),
		schema.Field("source_ids", schema.Array(schema.String())),
	))),
	schema.Field("source_ids", schema.Array(schema.String())),
))

// companyConversationKinds names the kinds a company-read answer may carry.
var companyConversationKinds = []string{
	companyConversationStatus, "answer", companyConversationRecommendation, companyConversationCorrection,
	"confirmation", "clarification", "off_topic",
}

// companyReadChangeFields is the extraction vocabulary narrowed to the fields
// a site-read change may name — the contract's enum is the authority, and the
// extraction list supplies the order the model is shown.
func companyReadChangeFields() []string {
	fields := make([]string, 0, len(extractionFieldNames))
	for _, name := range extractionFieldNames {
		if crmcontracts.CompanySiteReadSuggestedChangeField(name).Valid() {
			fields = append(fields, name)
		}
	}
	return fields
}

func companyConversationKindValid(kind string) bool {
	return slices.Contains(companyConversationKinds, kind)
}
