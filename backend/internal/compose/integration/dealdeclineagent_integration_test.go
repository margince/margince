// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A member's agent declines a deal on a captured mail through its route and
// through its tool, on its member's live passport. The deal's own project link
// comes off with the deal, and the audit row names the agent, its passport and
// its member. A revoked passport, or a member who has left, changes nothing.
func TestAnAgentsDeclineTakesTheDealsProjectThroughBothDoors(t *testing.T) {
	d := newTwoDoors(t, "agent-deal-decline", "read", "write")
	d.e.DescribeCompany(t)
	const email, password = "granter@agent-deal-decline.test", "agent-deal-decline-password"
	granter := createdID(t, d.e, "/v1/users", AnyMap{"email": email, "display_name": "Dana Granter", "role": "admin"})
	apptest.SetPasswordByLink(t, d.e, granter, password)
	seat := d.e.AsSecondSeat(t, email, password)
	bearer, passport := apptest.PassportWithID(t, seat, "decline agent", "read", "write")
	agent := apptest.NewMCPClient(seat, strings.TrimPrefix(bearer["Authorization"], "Bearer "))

	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Decline Customer", "source": "manual"})
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Rollout", "company_id": company, "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := createdID(t, d.e, "/v1/deals", AnyMap{
		"name": "Decline rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"company_id": company, "project_id": project, "source": "manual",
	})
	mails := captureMailsOnDeal(t, d.e, granter, deal, "rest", "tool", "refused")
	for _, mail := range mails {
		wantMailLinks(t, d.e, mail, deal, project)
	}

	body := AnyMap{"deal_id": deal}
	type answer struct {
		Unlinked       bool `json:"unlinked"`
		ProjectUnfiled bool `json:"project_unfiled"`
		ProjectKept    bool `json:"project_kept"`
	}
	var byRoute json.RawMessage
	if status := seat.Call(t, "POST", "/v1/activities/"+mails[0]+"/deal-decline", body, bearer, &byRoute); status != http.StatusOK {
		t.Fatalf("the agent's decline through the route → %d %s", status, byRoute)
	}
	got := sameRecords[answer](t, "decline_deal", byRoute,
		agent.CallOK(t, "change_activities", map[string]any{"action": "decline_deal", "id": mails[1], "request_body": body}).Envelope(t).Data)
	if !got.Unlinked || !got.ProjectUnfiled || got.ProjectKept {
		t.Fatalf("the agent's decline answered %+v, want the deal and its project taken off", got)
	}
	for _, mail := range mails[:2] {
		wantMailLinks(t, d.e, mail)
		var actorType, passportID, onBehalfOf, decidedBy string
		if err := d.e.Owner.QueryRow(t.Context(), `
			SELECT actor_type, coalesce(passport_id::text, ''), coalesce(on_behalf_of::text, ''), evidence->>'decided_by_name'
			  FROM audit_log WHERE entity_id = $1 AND evidence->>'cause' = 'project_filing_undone'`, mail).Scan(
			&actorType, &passportID, &onBehalfOf, &decidedBy); err != nil {
			t.Fatalf("reading the undo's audit row of %s: %v", mail, err)
		}
		if actorType != "agent" || passportID != passport.String() || onBehalfOf != granter || decidedBy != "Dana Granter" {
			t.Errorf("the undo of %s is recorded as %s %s for %s by %q, want the agent on passport %s for %s, Dana Granter",
				mail, actorType, passportID, onBehalfOf, decidedBy, passport, granter)
		}
	}

	revoked, revokedID := apptest.PassportWithID(t, seat, "revoked agent", "read", "write")
	if status := seat.Call(t, "DELETE", "/v1/passports/"+revokedID.String(), nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("revoking the passport → %d", status)
	}
	refused := func(name string, header map[string]string) {
		t.Helper()
		if status := seat.Call(t, "POST", "/v1/activities/"+mails[2]+"/deal-decline", body, header, nil); status != http.StatusUnauthorized {
			t.Errorf("the decline on %s answered %d, want 401", name, status)
		}
		wantMailLinks(t, d.e, mails[2], deal, project)
	}
	refused("a revoked passport", revoked)
	if status := d.e.Call(t, "POST", "/v1/users/"+granter+"/deactivate", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivating the granting member → %d", status)
	}
	refused("the passport of a member who left", bearer)
}

// captureMailsOnDeal captures one inbound mail per name through the production
// sink, on a mailbox the member connected, filed under deal as a connector
// files it. Capture's project ladder then copies the deal's project onto each.
func captureMailsOnDeal(t *testing.T, e *apptest.AppEnv, member, deal string, names ...string) []string {
	t.Helper()
	var ws ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM workspace ORDER BY created_at LIMIT 1`).Scan(&ws); err != nil {
		t.Fatalf("reading the workspace: %v", err)
	}
	user, err := ids.Parse(member)
	if err != nil {
		t.Fatalf("parsing the member id: %v", err)
	}
	dealID, err := ids.Parse(deal)
	if err != nil {
		t.Fatalf("parsing the deal id: %v", err)
	}
	mailbox := &dealMailbox{}
	for _, name := range names {
		address := "buyer-" + name + "@decline-customer.test"
		mailbox.records = append(mailbox.records, connector.NormalizedRecord{
			EntityType: datasource.EntityActivity,
			NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "<decline-" + name + "@decline-customer.test>"},
			Fields:     capture.ActivityFields{Kind: "email", Subject: "Rollout status " + name, Body: "Where are we?", Direction: connector.DirectionInbound},
			Links:      []datasource.EntityRef{{Type: datasource.EntityDeal, ID: dealID}},
			Source:     "gmail:" + name, CapturedBy: "connector:gmail",
			Raw:          []byte("From: " + address + "\r\nSubject: Rollout status\r\n\r\nWhere are we?"),
			Counterparty: connector.Counterparty{Email: address, Domain: "decline-customer.test", Direction: connector.DirectionInbound},
			Addresses:    []string{address},
		})
	}
	registry := compose.NewCaptureRegistry(e.Pool, e.Vault, compose.CaptureConfig{})
	registry.Register(mailbox)
	grant := principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), ws), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + member, UserID: user, SeatType: principal.SeatFull,
			Scopes: principal.NewScopeSet(principal.ScopeRead),
		})
	connection, err := registry.Connect(grant, "gmail", connector.Auth("deal-mail-token"))
	if err != nil {
		t.Fatalf("connecting the mailbox: %v", err)
	}
	if err := registry.SyncOnce(principal.WithWorkspaceID(context.Background(), ws), connection); err != nil {
		t.Fatalf("capturing the mail: %v", err)
	}
	out := make([]string, 0, len(names))
	for _, record := range mailbox.records {
		var id string
		if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM activity WHERE source_id = $1`, record.NaturalKey.SourceID).Scan(&id); err != nil {
			t.Fatalf("finding the captured mail %s: %v", record.NaturalKey.SourceID, err)
		}
		out = append(out, id)
	}
	return out
}

// wantMailLinks fails unless mail's deal and project links are want, in order.
func wantMailLinks(t *testing.T, e *apptest.AppEnv, mail string, want ...string) {
	t.Helper()
	var got []string
	rows, err := e.Owner.Query(t.Context(), `SELECT coalesce(deal_id, project_id)::text FROM activity_link
		WHERE activity_id = $1 AND entity_type IN ('deal', 'project') ORDER BY entity_type`, mail)
	if err != nil {
		t.Fatalf("reading the links of %s: %v", mail, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || (len(want) == 2 && (got[0] != want[0] || got[1] != want[1])) {
		t.Errorf("mail %s is on deal and project %v, want %v", mail, got, want)
	}
}

// dealMailbox is a mailbox whose provider files each mail under a deal, as the
// offline-demo mail generator does.
type dealMailbox struct {
	records []connector.NormalizedRecord
}

func (*dealMailbox) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		Name: "gmail", Version: "1", Scopes: []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute, Produces: []datasource.EntityType{datasource.EntityActivity},
	}
}

func (*dealMailbox) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("deal-mail-token"), nil
}

func (m *dealMailbox) Sync(ctx context.Context, _ connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	for _, record := range m.records {
		if _, err := sink.Upsert(ctx, record); err != nil {
			return nil, err
		}
	}
	return cursor, nil
}

func (*dealMailbox) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, connector.ErrSkip
}

func (*dealMailbox) HealthCheck(context.Context, connector.Auth) error { return nil }
