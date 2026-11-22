// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A drill-through row that names a partner and no other company reads as
// though the partner were the deal's company.
func TestADrillThroughThatNamesAPartnerAlsoNamesTheDealsCompany(t *testing.T) {
	judged := 0
	for name, spec := range prebuiltReports {
		columns, _ := derivationColumns(spec)
		if !slices.Contains(columns, fieldPartnerCompanyID) {
			continue
		}
		judged++
		if !slices.Contains(columns, fieldCompanyID) {
			t.Errorf("%s's drill-through rows carry %s and not %s", name, fieldPartnerCompanyID, fieldCompanyID)
		}
	}
	if judged == 0 {
		t.Fatal("no drill-through carried a partner; this gate proves nothing")
	}
}

// A drill column is shown and never grouped by.
func TestADrillColumnIsNeverAGroup(t *testing.T) {
	judged := 0
	for name, spec := range prebuiltReports {
		for field := range spec.drillColumns {
			judged++
			if _, isDimension := spec.dimensions[field]; isDimension {
				t.Errorf("%s declares %q as both a drill column and a dimension", name, field)
			}
			if _, isMeasure := spec.measures[field]; isMeasure {
				t.Errorf("%s declares %q as both a drill column and a measure", name, field)
			}
			var refused *FieldNotAllowedError
			if _, err := compileDerivation(spec, derivationQuery{GroupBy: []string{field}}); !errors.As(err, &refused) {
				t.Errorf("%s: a handle grouping by drill column %q = %v, want FieldNotAllowedError", name, field, err)
			}
		}
	}
	if judged == 0 {
		t.Fatal("no spec declared a drill column; this gate proves nothing")
	}
}

// The deal's company is blanked the same way as its partner.
func TestTheDealsCompanyIsBlankedLikeItsPartner(t *testing.T) {
	spec := pipelineCurrentSpec()
	plan, err := compileDerivation(spec, derivationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	reader := principal.WithActor(context.Background(), actorWithScope(principal.RowScopeOwn))
	selects, err := maskedDerivationSelects(reader, spec, plan, func(any) int { return 1 })
	if err != nil {
		t.Fatal(err)
	}
	for column, expr := range map[string]string{fieldCompanyID: colCompanyID, fieldPartnerCompanyID: colPartnerCompanyID} {
		at := slices.Index(plan.columns, column)
		if at < 0 {
			t.Fatalf("the drill-through returns no %s column", column)
		}
		got := selects[at]
		if !strings.HasPrefix(got, "(CASE WHEN EXISTS (SELECT 1 FROM "+tableCompany+" ref WHERE ref.id = "+expr+" AND ") ||
			!strings.HasSuffix(got, " THEN "+expr+" END) AS "+column) {
			t.Errorf("%s is selected as %q, want it blanked outside the reader's companies", column, got)
		}
	}
}
