// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// weeklySeeders make what only a background job writes, through the writer
// the job calls. That is the reviews of the week that closed, and a site read.
func weeklySeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"weekly_review": func() string {
			s.assembleWeek(nil)
			return ""
		},
		"finished_site_read": func() string { return finishedSiteRead(s) },
		"review_team": func() string {
			team := s.post("/v1/teams", AnyMap{"name": "Sweep " + ids.NewV7().String()})
			admin := sessionUserID(s.t, s.e)
			s.call("PUT", "/v1/teams/"+team+"/members/"+admin, AnyMap{}, nil, nil)
			ctx := s.assembleWeek([]ids.UUID{ids.MustParse(team)})
			members := []weekly.TeamMember{{UserID: ids.MustParse(admin), DisplayName: "Admin"}}
			if _, _, err := s.weeklyEngine().AssembleTeamFor(ctx, ids.MustParse(team), "Sweep", members, clocktest.Now(s.t)); err != nil {
				s.t.Fatalf("writing the team's week: %v", err)
			}
			return team
		},
	}
}

func (s sweepSeed) weeklyEngine() *weekly.Engine {
	return weekly.NewEngine(s.e.Pool, identity.NewService(s.e.Pool))
}

// assembleWeek writes the admin's own review of the week that closed, as the
// admin on the teams named, and answers that principal.
func (s sweepSeed) assembleWeek(teams []ids.UUID) context.Context {
	admin := ids.MustParse(sessionUserID(s.t, s.e))
	ws := apptest.InstallationWorkspaceUUID(context.Background(), s.t, s.e.Pool)
	ctx := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), ws), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + admin.String(), UserID: admin, TeamIDs: teams, Permissions: AdminPerms,
	})
	if _, _, err := s.weeklyEngine().AssembleFor(ctx, clocktest.Now(s.t)); err != nil {
		s.t.Fatalf("writing the admin's week: %v", err)
	}
	return ctx
}

// finishedSiteRead is a site read of the admin's company with a draft, as the
// crawl job leaves it. It notes the version and hash a confirmation sends back.
func finishedSiteRead(s sweepSeed) string {
	read := s.create["site_read"]()
	admin := ids.MustParse(sessionUserID(s.t, s.e))
	ws := apptest.InstallationWorkspaceUUID(context.Background(), s.t, s.e.Pool)
	ctx := principal.WithCorrelationID(principal.WithActor(principal.WithWorkspaceID(context.Background(), ws), principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:deepread", UserID: admin, OnBehalfOf: admin,
	}), ids.MustParse(read))
	store := contacts.NewStore(compose.InstallationDB(s.e.Pool))
	if _, err := store.BeginSiteRead(ctx, ids.MustParse(read), 10*time.Minute); err != nil {
		s.t.Fatalf("beginning the site read: %v", err)
	}
	// The draft reads what the company already says, so confirming it asks no
	// one to choose between the read and a value a person typed.
	var current struct {
		OfferSummary string `json:"offer_summary"`
		ICP          string `json:"icp"`
	}
	s.call("GET", "/v1/company", nil, nil, &current)
	site := "https://example.test/role-sweep"
	fields := []contacts.DeepReadField{
		{Field: "offer_summary", Value: current.OfferSummary, EvidenceSnippet: current.OfferSummary, SourceURL: site, Confidence: 0.9},
		{Field: "icp", Value: current.ICP, EvidenceSnippet: current.ICP, SourceURL: site, Confidence: 0.9},
	}
	s.pool.note(read, "offer_summary", current.OfferSummary)
	s.pool.note(read, "icp", current.ICP)
	if err := store.FinishSiteRead(ctx, ids.MustParse(read), contacts.FinishSiteReadInput{
		Status: "partial", FactCount: len(fields), ProfileFields: fields,
		Pages: []contacts.SiteReadPage{{URL: site, Kind: "home"}}, ProposalHash: strings.Repeat("a", 64),
	}); err != nil {
		s.t.Fatalf("finishing the site read: %v", err)
	}
	var draft struct {
		DraftVersion int    `json:"draft_version"`
		ProposalHash string `json:"proposal_hash"`
	}
	s.call("GET", "/v1/company/site-reads/"+read, nil, nil, &draft)
	s.pool.note(read, "draft_version", strconv.Itoa(draft.DraftVersion))
	s.pool.note(read, "proposal_hash", draft.ProposalHash)
	return read
}
