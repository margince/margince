// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// technicalLeaves are the four the organization engine gained, with the field
// each one pins in organization_fact.
//
// gatekit:fixture the leaf name each technical filter carries, mapped to the
// organization_fact field it must pin
var technicalLeaves = map[string]string{
	"mail_provider":    "mail_provider",
	"hosting_provider": "hosting_provider",
	"operated_service": "operated_service",
	"technology":       "technology",
}

// Every technical leaf must correlate to the account it is filtering. A leaf
// that forgot the correlation selects every account the moment ANY account
// carries the value, which reads as a working segment and is not one.
func TestEveryTechnicalLeafCorrelatesToTheAccount(t *testing.T) {
	t.Parallel()
	for name := range technicalLeaves {
		field, ok := segmentEngines["organization"].Fields[name]
		if !ok {
			t.Errorf("organizations cannot be filtered by %s", name)
			continue
		}
		if !strings.Contains(field.Link, "tf.organization_id = t.id") {
			t.Errorf("the %s leaf does not correlate to the account: %q", name, field.Link)
		}
	}
}

// Each leaf pins its own field, or the four would answer each other's
// questions: a "webshop" clause reaching mail_provider rows selects nothing,
// and a technology clause reaching every field selects far too much.
func TestEveryTechnicalLeafPinsItsOwnField(t *testing.T) {
	t.Parallel()
	for name, field := range technicalLeaves {
		leaf, ok := segmentEngines["organization"].Fields[name]
		if !ok {
			t.Errorf("organizations cannot be filtered by %s", name)
			continue
		}
		if !strings.Contains(leaf.Link, "tf.field = '"+field+"'") {
			t.Errorf("the %s leaf does not pin its field: %q", name, leaf.Link)
		}
	}
}

// `technology` is a field two categories can use, and only the signal one is an
// observed fact. Without the category pin a segment would silently widen the
// day a non-signal technology fact is written.
func TestEveryTechnicalLeafPinsTheSignalCategory(t *testing.T) {
	t.Parallel()
	for name := range technicalLeaves {
		leaf, ok := segmentEngines["organization"].Fields[name]
		if !ok {
			continue
		}
		if !strings.Contains(leaf.Link, "tf.category = 'signal'") {
			t.Errorf("the %s leaf does not pin the signal category: %q", name, leaf.Link)
		}
	}
}

// A person correcting a machine-read value rewrites the row's source to
// `human`. A leaf constrained by source would drop exactly the accounts
// somebody cared enough to fix, which is the opposite of what naming that
// value means.
func TestNoTechnicalLeafFiltersOnSource(t *testing.T) {
	t.Parallel()
	for name := range technicalLeaves {
		leaf, ok := segmentEngines["organization"].Fields[name]
		if !ok {
			continue
		}
		if strings.Contains(leaf.Link, "tf.source") {
			t.Errorf("the %s leaf constrains source, so a corrected fact stops matching: %q", name, leaf.Link)
		}
	}
}

// The three classified sets are picklists so a picker can offer their values;
// technology is text because its vocabulary is the fingerprint ruleset, which
// grows whenever a rule is added.
func TestTheTechnicalLeavesAreTypedByWhetherTheirSetIsClosed(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]storekit.FieldType{
		"mail_provider":    storekit.FieldPicklist,
		"hosting_provider": storekit.FieldPicklist,
		"operated_service": storekit.FieldPicklist,
		"technology":       storekit.FieldText,
	} {
		leaf, ok := segmentEngines["organization"].Fields[name]
		if !ok {
			t.Errorf("organizations cannot be filtered by %s", name)
			continue
		}
		if leaf.Type != want {
			t.Errorf("the %s leaf is typed %v, want %v", name, leaf.Type, want)
		}
		if want == storekit.FieldPicklist && len(leaf.Options) == 0 {
			t.Errorf("the %s leaf is a picklist offering no values, so a picker shows an empty list", name)
		}
	}
}
