// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A report spec's filters are part of what can be asked. The analytics
// vocabulary served only its dimensions and measures.
//
// So an agent asking which deals are stuck was told there is no such field.
// The deals board answered that question from the same spec, through its own
// dial.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

// everySeat holds every grant the pipeline spec's vocabulary asks for, so a
// field missing from this schema is missing by derivation and not by grant.
func everySeat() []string {
	return []string{"deal", "activity", "contact", "company", "project", "partner"}
}

func TestAFilterOnlyFieldIsServedAsAFilter(t *testing.T) {
	t.Parallel()
	schema := AnalyticsSchemaFor(seatWith(everySeat()...))
	entity, ok := schema.Entities["deals-by-stage"]
	if !ok {
		t.Fatal("the pipeline population is absent from a seat holding every grant")
	}
	field, served := entity.Lookup("stalled")
	if !served {
		t.Fatalf("stalled is not in the vocabulary; it offers %s", strings.Join(entity.FilterableNames(), ", "))
	}
	if field.Kind != analyticsquery.KindFilter {
		t.Errorf("stalled is served as a %s, want a filter: grouping by a threshold answers no question", field.Kind)
	}
}

// A dimension that is also a filter keeps the kind that can do both. The two
// vocabularies overlap in every spec, and serving the filter kind for an
// overlapping name would take away a grouping the screens rely on.
func TestANameInBothVocabulariesStaysADimension(t *testing.T) {
	t.Parallel()
	entity, ok := AnalyticsSchemaFor(seatWith(everySeat()...)).Entities["deals-by-stage"]
	if !ok {
		t.Fatal("the pipeline population is absent from a seat holding every grant")
	}
	both := []string{}
	for name := range prebuiltReports["deals-by-stage"].dimensions {
		if _, isFilter := prebuiltReports["deals-by-stage"].filters[name]; isFilter {
			both = append(both, name)
		}
	}
	if len(both) == 0 {
		t.Skip("the pipeline spec no longer names anything in both vocabularies")
	}
	for _, name := range both {
		field, served := entity.Lookup(name)
		if !served {
			t.Errorf("%s is in both vocabularies and is served by neither", name)
			continue
		}
		if field.Kind != analyticsquery.KindDimension {
			t.Errorf("%s is served as a %s, want a dimension: it is groupable as well as filterable", name, field.Kind)
		}
	}
}

// The schema's own promise, held for the new kind. A field the caller may not
// read is absent, so naming it answers "no such field" rather than naming a
// narrowing they cannot have.
func TestAFilterReadingAnotherRecordTypeFollowsItsGrant(t *testing.T) {
	t.Parallel()
	spec := prebuiltReports["activities-by-kind"]
	granted := ""
	for name, object := range spec.grants {
		if _, isFilter := spec.filters[name]; isFilter && object != string(spec.entity) {
			granted = name
			break
		}
	}
	if granted == "" {
		t.Skip("no filter on this spec reads another record type")
	}
	withGrant := AnalyticsSchemaFor(seatWith(everySeat()...)).Entities["activities-by-kind"]
	if _, served := withGrant.Lookup(granted); !served {
		t.Errorf("%s is withheld from a seat holding every grant", granted)
	}
	withoutGrant, ok := AnalyticsSchemaFor(seatWith("activity")).Entities["activities-by-kind"]
	if !ok {
		t.Fatal("the activity population is absent from a seat holding activity.read")
	}
	if _, served := withoutGrant.Lookup(granted); served {
		t.Errorf("%s is served to a seat lacking the grant it reads through", granted)
	}
}

// Through the REAL vocabulary, because the shape comes from the seam and not
// from the field a test builds. A boolean filter served as text refuses the
// caller's `true`, which refuses the question on its value rather than on its
// name.
func TestABooleanFilterTakesABooleanValue(t *testing.T) {
	t.Parallel()
	schema := AnalyticsSchemaFor(seatWith(everySeat()...))
	for _, name := range []string{"stalled", "partner_sourced"} {
		field, served := schema.Entities["deals-by-stage"].Lookup(name)
		if !served {
			t.Fatalf("%s is absent from a seat holding every grant", name)
		}
		if field.Shape != analyticsquery.ShapeBoolean {
			t.Errorf("%s is served with shape %q, want boolean: its expression yields true or false",
				name, field.Shape)
		}
	}
}
