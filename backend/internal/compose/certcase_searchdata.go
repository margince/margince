// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/compose/searchplan"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func searchCaseVocabulary(c searchplan.Fixture) (string, error) {
	id, err := ids.Parse(c.SelfUserID)
	if err != nil {
		return "", err
	}
	grants := map[string]principal.ObjectGrant{}
	for _, object := range c.Objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: id, Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeOwn}})
	records, err := search.NewQuerySchemaResource(search.NewVocabularyResolver()).ReadResource(ctx, search.QuerySchemaURI)
	if err != nil {
		return "", err
	}
	catalog, err := (metricEvaluator{}).Catalog(ctx, textlang.English)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(searchVocabulary{
		Now: c.Now, Context: c.Context, Metrics: catalog, SelfUserID: c.SelfUserID,
		BaseCurrencyScale: values.MinorUnitScale(c.Context.BaseCurrency), MinorUnitExceptions: values.MinorUnitExceptions(),
		AnalyticsVocabulary: DescribeAnalyticsSchema(AnalyticsSchemaFor(ctx)), RecordVocabulary: json.RawMessage(records.Text),
	})
	return string(encoded), err
}
