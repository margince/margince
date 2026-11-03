// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// The document names every view read_view reads, in name order, with what each
// takes. A view with no query arguments lists none rather than null.
func TestTheViewsDocumentNamesEveryViewAndItsArguments(t *testing.T) {
	views := append(slices.Clone(viewsOfTwoShapes), ReadView{Name: "acquisition_sources", Pattern: "/v1/acquisition-sources"})
	contents, err := NewReadViewsResource(views).ReadResource(context.Background(), ReadViewsURI)
	if err != nil {
		t.Fatalf("reading the views document: %v", err)
	}
	var doc readViewsDoc
	if err := json.Unmarshal([]byte(contents.Text), &doc); err != nil {
		t.Fatalf("the document does not decode: %v", err)
	}
	var names []string
	for _, entry := range doc.Views {
		names = append(names, entry.View)
	}
	if strings.Join(names, ",") != "acquisition_sources,company_360,seat_names" {
		t.Fatalf("the document lists %v, want every view in name order", names)
	}
	company := doc.Views[1]
	if len(company.Path) != 1 || company.Path[0] != "id" || len(company.Query) != 2 || company.Query[0] != (ViewQuery{Name: "refresh", Type: "boolean"}) {
		t.Errorf("company_360 reads as %+v, want its path id and its two query arguments", company)
	}
	if !strings.Contains(contents.Text, `"view":"acquisition_sources","path":[],"query":[]`) {
		t.Errorf("a view with no arguments reads as %s, want empty lists", contents.Text)
	}
}

// The tool's schema names the document instead of reciting the views, and a
// view outside the catalog is refused with every view that exists.
func TestAnUnknownViewIsRefusedWithEveryViewThereIs(t *testing.T) {
	tool := readView{views: viewsOfTwoShapes}
	if schema := string(tool.Spec().InputSchema); strings.Contains(schema, `"enum"`) || !strings.Contains(schema, ReadViewsURI) {
		t.Errorf("the view argument reads %s, want it to name %s rather than list the views", schema, ReadViewsURI)
	}
	_, err := tool.Handle(context.Background(), json.RawMessage(`{"view":"company_card"}`))
	var bad *BadArgsError
	if !errors.As(err, &bad) || !strings.Contains(err.Error(), "company_360, seat_names") {
		t.Errorf("an unknown view answered %v, want a refusal naming company_360 and seat_names", err)
	}
}
