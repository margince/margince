// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package analyticswire turns the analytics vocabulary into the contract's words.
package analyticswire

import (
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// FiltersOf lists the fields a question may narrow by and nothing
// else, with the shape of the value each takes. Nil when there are none, so the
// key is left out.
func FiltersOf(entity analyticsquery.Entity) *[]crmcontracts.AnalyticsFilterField {
	var out []crmcontracts.AnalyticsFilterField
	for _, name := range entity.FieldNames(analyticsquery.KindFilter) {
		shape := entity.Fields[name].Shape
		if shape == analyticsquery.ShapeText {
			shape = "text"
		}
		out = append(out, crmcontracts.AnalyticsFilterField{Name: name, Shape: crmcontracts.AnalyticsFilterFieldShape(shape)})
	}
	if out == nil {
		return nil
	}
	return &out
}
