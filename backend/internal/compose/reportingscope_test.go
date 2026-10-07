// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/url"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestScopedReportHandleKeepsItsPopulation(t *testing.T) {
	owner := ids.NewV7()
	scope := &RequestedScope{Kind: "owner", ID: &owner}
	handle := scopedDerivationURL("pipeline-current", nil, []string{"stage_id"}, nil, nil, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), scope)
	parsed, err := url.Parse(handle)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := parseDerivationQuery(parsed.Query())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Scope == nil || decoded.Scope.Kind != "owner" || decoded.Scope.ID == nil || *decoded.Scope.ID != owner {
		t.Fatalf("scope lost: %+v", decoded.Scope)
	}
	if len(decoded.Predicates) != 0 {
		t.Fatalf("scope was mistaken for a field: %+v", decoded.Predicates)
	}
}

func TestReportWithoutOwnerDimensionRefusesExplicitPopulation(t *testing.T) {
	spec := prebuiltReports["activities-by-kind"]
	if err := checkReportScope(spec, &RequestedScope{Kind: "workspace"}); err == nil {
		t.Fatal("activity report silently ignored an owner population")
	}
	if err := checkReportScope(spec, nil); err != nil {
		t.Fatal(err)
	}
}
