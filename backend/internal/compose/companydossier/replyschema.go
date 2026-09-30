// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

// The shapes the two model lanes must answer in, enforced at generation where
// the provider supports it. Each mirrors the struct its parser decodes into and
// the shape line its prompt states: a claim is always an object, because a
// bare string where ParseGrowthFit expects a claims.Sentence fails the whole
// reply, not just the one field.

import (
	"encoding/json"
	"maps"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// citedRecord is one claims.Evidence as the model writes it. Name is left out:
// it is ours to fill from the record, never the model's to assert.
func citedRecord() schema.Node {
	return schema.Record(
		schema.Field("entity_type", schema.Enum(citeCompany, citeFact, citeProfileField)),
		schema.Field("entity_id", schema.String()),
	)
}

// claimSchema is one claims.Sentence, its nature closed to the ones the bucket
// holding it keeps — anything else would be generated only to be dropped.
func claimSchema(natures ...string) schema.Node {
	return schema.Record(
		schema.Field("text", schema.String()),
		schema.Field("nature", schema.Enum(natures...)),
		schema.Field("evidence", schema.Array(citedRecord())),
	)
}

// dossierSchema is ParseDossier's input: sections of facts.
func dossierSchema() json.RawMessage {
	return schema.Must(schema.Record(
		schema.Field("sections", schema.Array(schema.Record(
			schema.Field("kind", schema.Enum(sectionOrder...)),
			schema.Field("sentences", schema.Array(claimSchema(natureFact))),
		))),
	))
}

// growthFitSchema is growthFitClaims.
func growthFitSchema() json.RawMessage {
	observed := slices.Sorted(maps.Keys(observations))
	return schema.Must(schema.Record(
		schema.Field("band", schema.Enum(schema.Names(judgeableBands()...)...)),
		schema.Field("sub_scores", schema.Array(schema.Record(
			schema.Field("dimension", schema.Enum(slices.Sorted(maps.Keys(growthFitDimensions))...)),
			schema.Field("score", schema.Integer()),
			schema.Field("reason", schema.String()),
			schema.Field("evidence", schema.Array(citedRecord())),
		))),
		schema.Field("positive_factors", schema.Array(claimSchema(observed...))),
		schema.Field("negative_factors", schema.Array(claimSchema(observed...))),
		schema.Field("whitespace", schema.Array(claimSchema(observed...))),
		schema.Field("objections", schema.Array(claimSchema(observed...))),
		schema.Field("recommended_angle", claimSchema(slices.Sorted(maps.Keys(suggestions))...)),
	))
}

// judgeableBands is the bands the model may propose, strongest first. The
// abstention is left out because ParseGrowthFit refuses it.
func judgeableBands() []crmcontracts.GrowthFitBand {
	bands := slices.DeleteFunc(slices.Collect(maps.Keys(bandRank)), func(band crmcontracts.GrowthFitBand) bool {
		return !BandIsJudgeable(band)
	})
	slices.SortFunc(bands, func(a, b crmcontracts.GrowthFitBand) int { return bandRank[b] - bandRank[a] })
	return bands
}
