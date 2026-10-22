// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The findings for one population are the ones on that population's deals,
// narrowed by the rule the forecast and reports use for it.
func TestFindingsFollowTheRequestedPopulation(t *testing.T) {
	e := setupForecast(t)
	team1Deal := e.seedOpenDeal(t, "North deal", 20, &e.Rep1, nil, nil)
	team2Deal := e.seedOpenDeal(t, "South deal", 20, &e.Rep3, nil, nil)
	for _, deal := range []ids.UUID{team1Deal, team2Deal} {
		if _, err := e.owner.Exec(t.Context(),
			`INSERT INTO assurance_exception (logical_key, type, subject_kind, subject_id, severity, captured_by)
			 VALUES ($1, 'close_past', 'deal', $2, 'high', 'test')`,
			"close_past:"+deal.String(), deal); err != nil {
			t.Fatal(err)
		}
	}
	admin := e.Admin()
	read := func(population assurance.Population) ([]ids.UUID, error) {
		var subjects []ids.UUID
		err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
			found, err := AssuranceExceptionsIn(admin, tx, population)
			for _, finding := range found {
				subjects = append(subjects, finding.SubjectID)
			}
			return err
		})
		return subjects, err
	}

	cases := []struct {
		name       string
		population assurance.Population
		want       []ids.UUID
	}{
		{"unscoped", assurance.Population{}, []ids.UUID{team1Deal, team2Deal}},
		{"team", assurance.Population{Kind: "team", ID: &e.Team1}, []ids.UUID{team1Deal}},
		{"owner", assurance.Population{Kind: "owner", ID: &e.Rep3}, []ids.UUID{team2Deal}},
	}
	for _, c := range cases {
		got, err := read(c.population)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		slices.SortFunc(got, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
		slices.SortFunc(c.want, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: subjects = %v, want %v", c.name, got, c.want)
		}
	}

	if _, err := read(assurance.Population{ID: &e.Team1}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("an id without a kind: err = %v, want invalid argument", err)
	}
}
