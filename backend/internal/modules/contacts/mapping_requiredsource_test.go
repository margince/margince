// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A lead and a relationship name where they came from, as a contact, a company
// and a deal already must: a record with no recorded origin cannot be traced.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestALeadAndARelationshipMustNameTheirSource(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{"", "   ", "\u200b"} {
		_, leadErr := leadCreateInput(crmcontracts.CreateLeadRequest{Source: origin})
		_, relErr := relationshipCreateInput(crmcontracts.CreateRelationshipRequest{Source: origin})
		for name, err := range map[string]error{"lead": leadErr, "relationship": relErr} {
			assertOriginRequired(t, name, origin, err)
		}
	}
}

func TestALeadAndARelationshipKeepTheirSourceTrimmed(t *testing.T) {
	t.Parallel()
	lead, err := leadCreateInput(crmcontracts.CreateLeadRequest{Source: "  manual "})
	if err != nil || lead.Source != "manual" {
		t.Errorf("lead source = %q, %v; want manual", lead.Source, err)
	}
	rel, err := relationshipCreateInput(crmcontracts.CreateRelationshipRequest{Source: "  manual "})
	if err != nil || rel.Source != "manual" {
		t.Errorf("relationship source = %q, %v; want manual", rel.Source, err)
	}
}

func assertOriginRequired(t *testing.T, what, origin string, err error) {
	t.Helper()
	detailed, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || len(detailed.Fields) != 1 || detailed.Fields[0].Field != "source" || detailed.Fields[0].Code != "required" {
		t.Errorf("%s with source %q: err = %v, want the required refusal naming source", what, origin, err)
	}
}
