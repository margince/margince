// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A value a user's agent wrote is that user's. It holds against automatic work
// as the user's own does, and automatic work's value does not.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asUsersAgent is the rep's agent on a passport: its actor names the passport.
func (e *dedupeEnv) asUsersAgent(t *testing.T) context.Context {
	t.Helper()
	ctx := e.asAgentFor(e.rep)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the agent context carries no actor")
	}
	passport := ids.NewV7()
	actor.ID, actor.PassportID, actor.OnBehalfOf = "agent:"+passport.String(), passport, e.rep
	return principal.WithActor(ctx, actor)
}

func TestASiteReadLeavesTheDescriptionAUsersAgentWrote(t *testing.T) {
	e := setupDedupe(t)
	company, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: "Agent Held GmbH", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	fromAgent := "What the user told their agent."
	if _, err := e.store.UpdateCompany(e.asUsersAgent(t), companyID, UpdateCompanyInput{Description: &fromAgent}); err != nil {
		t.Fatal(err)
	}

	applySiteReadDescription(e.as(), t, e, companyID, "What the website says.")

	reread, err := e.store.GetCompany(e.as(), companyID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Description == nil || *reread.Description != fromAgent {
		t.Fatalf("description after the site read = %v, want the agent's %q", reread.Description, fromAgent)
	}
}

func TestALogoIsHeldByAUsersAgentAndNotByAutomaticWork(t *testing.T) {
	passport := "agent:" + ids.NewV7().String()
	for stamp, want := range map[string]bool{passport: true, "agent:enrich": false} {
		t.Run(stamp, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			companyID := seedLogoCompany(ctx, t, e, "Nordwind Energie AG", "nordwind.test")
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `UPDATE company SET logo_object_key = 'k', logo_origin = 'upload' WHERE id = $1`, companyID); err != nil {
					return err
				}
				return storekit.StampFields(ctx, tx, "company", companyID.UUID, CompanySourceAgent, stamp,
					[]storekit.FieldStamp{{Field: "logo"}})
			}); err != nil {
				t.Fatal(err)
			}
			held, err := e.store.LogoHeldByHuman(ctx, companyID)
			if err != nil {
				t.Fatal(err)
			}
			if held != want {
				t.Fatalf("held = %t, want %t", held, want)
			}
		})
	}
}

func TestATechnicalValueAUsersAgentCorrectedIsNotAReplacedMachineValue(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := seedTechnicalCompany(ctx, t, e, "Entschieden GmbH", "entschieden.de")
	read := TechnicalEnrichment{
		CompanyID:    companyID,
		Completed:    []TechnicalLane{LaneDNS},
		Observations: []TechnicalObservation{observation(FactMailProvider, "google_workspace", "Google Workspace")},
		ObservedAt:   technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatal(err)
	}
	stamp := "agent:" + ids.NewV7().String()
	for name, tc := range map[string]struct {
		stamp string
		want  bool
	}{
		"a user's agent": {stamp, true},
		"automatic work": {"agent:enrich", false},
	} {
		t.Run(name, func(t *testing.T) {
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `UPDATE company_fact SET captured_by = $2, source = 'agent' WHERE company_id = $1 AND field = $3`,
					companyID, tc.stamp, FactMailProvider); err != nil {
					return err
				}
				held, err := readTechnicalFacts(ctx, tx, companyID, []string{FactMailProvider})
				if err != nil {
					return err
				}
				if len(held) != 1 || held[0].HumanHeld != tc.want {
					t.Errorf("held = %+v, want HumanHeld %t", held, tc.want)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTheRequesterVatNumberTrustsAUsersAgentAndNotAutomaticWork(t *testing.T) {
	passport := "agent:" + ids.NewV7().String()
	for name, tc := range map[string]struct {
		source, stamp, want string
	}{
		"a user":         {CompanySourceHuman, "human:" + ids.NewV7().String(), "DE123456789"},
		"a user's agent": {CompanySourceAgent, passport, "DE123456789"},
		"automatic work": {CompanySourceAgent, "agent:enrich", ""},
	} {
		t.Run(name, func(t *testing.T) {
			e := newAnchorEnv(t)
			if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(e.ctx, `
					INSERT INTO company_profile_field (company_id, field, value, source, captured_by)
					VALUES ($1, 'register_vat', 'DE123456789', $2, $3)
					ON CONFLICT (company_id, field) DO UPDATE SET source = EXCLUDED.source, captured_by = EXCLUDED.captured_by`,
					e.anchorID, tc.source, tc.stamp)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			got, err := e.store.RequesterVatNumber(e.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("RequesterVatNumber = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnAnchorLogoNamesTheAgentThatSetIt(t *testing.T) {
	e := newAnchorEnv(t)
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the anchor context carries no actor")
	}
	passport := ids.NewV7()
	actor.Type, actor.ID, actor.PassportID, actor.OnBehalfOf = principal.PrincipalAgent, "agent:"+passport.String(), passport, actor.UserID
	agent := principal.WithActor(e.ctx, actor)
	if _, err := e.store.SetAnchorCompanyLogo(agent, LogoWide, "anchor/logo", "logo.png"); err != nil {
		t.Fatal(err)
	}
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		var source string
		if err := tx.QueryRow(e.ctx, `
			SELECT source FROM field_provenance
			 WHERE object_type = 'company' AND object_id = $1 AND field_name = 'logo'
			 ORDER BY captured_at DESC, id DESC LIMIT 1`, e.anchorID).Scan(&source); err != nil {
			return err
		}
		if source != CompanySourceAgent {
			t.Errorf("the anchor logo's source = %q, want %q", source, CompanySourceAgent)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAColleaguesWorkCountsThroughTheirAgentAndNotThroughAutomaticWork(t *testing.T) {
	e := setupDedupe(t)
	var before time.Time
	worked := func(t *testing.T, id ids.ContactID) bool {
		t.Helper()
		ctx := e.as()
		var human, colleague bool
		if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
			if human, err = storekit.HumanActedSince(ctx, tx, entityContact, id.UUID, before); err != nil {
				return err
			}
			colleague, err = ColleagueWorkedOnSince(ctx, tx, entityContact, id.UUID, before, ids.Nil)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if human != colleague {
			t.Errorf("the two readers disagree: HumanActedSince %t, ColleagueWorkedOnSince %t", human, colleague)
		}
		return colleague
	}
	title := "Head of Something"
	for name, tc := range map[string]struct {
		as   func() context.Context
		want bool
	}{
		"a user's agent": {func() context.Context { return e.asUsersAgent(t) }, true},
		"automatic work": {e.asAgent, false},
	} {
		t.Run(name, func(t *testing.T) {
			id := e.seedByAutomaticWork(t, name)
			if _, err := e.store.UpdateContact(tc.as(), id, UpdateContactInput{Title: &title}); err != nil {
				t.Fatal(err)
			}
			if got := worked(t, id); got != tc.want {
				t.Fatalf("worked = %t, want %t", got, tc.want)
			}
		})
	}
}

// seedByAutomaticWork creates a contact no user has touched.
func (e *dedupeEnv) seedByAutomaticWork(t *testing.T, name string) ids.ContactID {
	t.Helper()
	return e.seedContact(e.asAgent(), t, name, []string{ids.NewV7().String() + "@agent.test"}, nil)
}
