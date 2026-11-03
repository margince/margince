// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// leadTeamReader reads leads on team scope with the given object grants.
func leadTeamReader(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		Permissions: principal.Permissions{RowScope: principal.RowScopeTeam, Objects: objects},
	})
}

var leadReadOnly = map[string]principal.ObjectGrant{leadEntity: {Read: true}}

// leadCompanyPartnerRead reads leads, companies and partner programmes: every
// dial the lead list has.
var leadCompanyPartnerRead = map[string]principal.ObjectGrant{
	leadEntity: {Read: true}, companyEntity: {Read: true}, partnerMaskObject: {Read: true},
}

// The company dials take company read. The partner dials take partner read and
// company read, because they read the partner through the caller's company scope.
func TestRefusedLeadFiltersNamesTheDialsASeatCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want []string
	}{
		{"a reader of every row", principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalSystem, ID: "system:test",
		}), []string{}},
		{"a seat reading leads, companies and partners", leadTeamReader(leadCompanyPartnerRead), []string{}},
		{"a seat reading leads and companies", leadTeamReader(map[string]principal.ObjectGrant{
			leadEntity: {Read: true}, companyEntity: {Read: true},
		}), []string{filterPartnerAccepted, leadPartnerField}},
		{"a seat reading leads and partners", leadTeamReader(map[string]principal.ObjectGrant{
			leadEntity: {Read: true}, partnerMaskObject: {Read: true},
		}), []string{filterCountry, filterIndustry, filterPartnerAccepted, leadPartnerField, filterSizeBand}},
		{"a seat reading leads alone", leadTeamReader(leadReadOnly), []string{
			filterCountry, filterIndustry, filterPartnerAccepted, leadPartnerField, filterSizeBand,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := refusedLeadFilters(tc.ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("refused %v, want %v", got, tc.want)
			}
		})
	}
}

// The probe walks the list's own filter set, so a filter added there is probed.
// Each operand it sends is one a reader of every row is not refused.
func TestTheReachProbeWalksEveryLeadListFilter(t *testing.T) {
	reach, err := LeadListFilterReach(leadTeamReader(leadReadOnly), nil)
	if err != nil {
		t.Fatal(err)
	}
	probed := make([]string, 0, len(reach))
	everyRow := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	for _, probe := range reach {
		probed = append(probed, probe.Parameter)
		var in ListLeadsInput
		if err := leadListFilters.Apply(&in, map[string]string{probe.Parameter: probe.Operand}); err != nil {
			t.Fatalf("the probe's %s operand does not bind: %v", probe.Parameter, err)
		}
		if _, err := leadListWhere(everyRow, in, nil, nil, leadSLAPolicy{}, func(any) int { return 1 }); err != nil {
			t.Errorf("the probe's %s operand faults for a reader of every row: %v", probe.Parameter, err)
		}
	}
	if !slices.Equal(probed, leadListFilters.Names()) {
		t.Errorf("probed %v, want every filter the list binds: %v", probed, leadListFilters.Names())
	}
}

// list_id is refused where the lead list resolves the list, before any lead
// clause runs. The probe resolves it through the same resolver.
func TestTheLeadReachProbeResolvesTheListFilterThroughTheListResolver(t *testing.T) {
	answering := func(err error) storekit.ListMemberFilterResolver {
		return func(context.Context, ids.UUID, string) (storekit.ListMemberFilter, error) { return nil, err }
	}
	readsBoth := leadTeamReader(leadCompanyPartnerRead)
	for _, tc := range []struct {
		name    string
		members storekit.ListMemberFilterResolver
		want    []string
	}{
		{"lists switched off", nil, []string{}},
		{"a reader who finds no such list", answering(apperrors.ErrNotFound), []string{}},
		{"a reader refused the list", answering(apperrors.ErrPermissionDenied), []string{filterListID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := refusedLeadFilters(readsBoth, tc.members)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("refused %v, want %v", got, tc.want)
			}
		})
	}
}

// A code no company could hold is refused by the field it names, before the
// grant is asked. The empty page it would answer reads as no such leads.
func TestACompanyFactDialRefusesAValueNoCompanyHolds(t *testing.T) {
	for _, tc := range []struct {
		filters LeadCompanyFactFilters
		field   string
	}{
		{LeadCompanyFactFilters{Industry: new("logistics")}, filterIndustry},
		{LeadCompanyFactFilters{Country: new("DEU")}, filterCountry},
		{LeadCompanyFactFilters{SizeBand: new("2-9")}, filterSizeBand},
	} {
		_, err := tc.filters.clauses(leadTeamReader(leadReadOnly), func(any) int { return 1 })
		if got := faultedField(err); got != tc.field {
			t.Errorf("%+v answered %v, want a fault naming %s", tc.filters, err, tc.field)
		}
	}
}

// faultedField is the one field a 422 names, or "" for any other answer.
func faultedField(err error) string {
	if fault, ok := errors.AsType[*values.ParseError](err); ok {
		return fault.Field
	}
	if detailed, ok := errors.AsType[*httperr.DetailedError](err); ok && len(detailed.Fields) == 1 {
		return detailed.Fields[0].Field
	}
	return ""
}

// A country is matched in either case, as the deal list matches one.
func TestTheCountryDialTakesEitherCase(t *testing.T) {
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }
	if _, err := (LeadCompanyFactFilters{Country: new("de")}).clauses(leadTeamReader(map[string]principal.ObjectGrant{
		leadEntity: {Read: true}, companyEntity: {Read: true},
	}), arg); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(bound, any("DE")) {
		t.Errorf("bound %v, want the country folded to DE", bound)
	}
}

// A lead's industry is stored as the code the company writer maps its text to. Text naming
// no code is refused: a lead has no description to keep it in.
func TestALeadIndustryWriteStoresTheCodeTheCompanyWriterWould(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    *string
		refused bool
	}{
		{"logistics_transport", new("logistics_transport"), false},
		{"Logistik", new("logistics_transport"), false},
		{"B2B SaaS", new("software"), false},
		{"  ", nil, false},
		{"widgets and gadgets", nil, true},
	} {
		got, err := leadIndustryWrite(&tc.raw)
		if (err != nil) != tc.refused {
			t.Errorf("%q: error %v, want refused=%v", tc.raw, err, tc.refused)
			continue
		}
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%q stored %v, want %v", tc.raw, got, tc.want)
		}
	}
}
