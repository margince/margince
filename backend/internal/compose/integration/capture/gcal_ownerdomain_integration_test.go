// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// domainAdmin may change the workspace's own domains and consumer list.
func domainAdmin(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		SeatType: principal.SeatFull, Scopes: principal.NewScopeSet(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": {Create: true, Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// withoutTrustedDomains leaves the workspace trusting no own domain: the own
// company stops claiming the one the env seeded.
func withoutTrustedDomains(t *testing.T, env captureEnv) {
	t.Helper()
	if _, err := env.e.Owner.Exec(context.Background(),
		`UPDATE company_domain SET archived_at = now() WHERE domain = 'myco.example'`); err != nil {
		t.Fatal(err)
	}
}

func addConsumerDomain(kind, domain string) func(*testing.T, captureEnv) {
	return func(t *testing.T, env captureEnv) {
		t.Helper()
		if _, err := capturemod.NewFreemailDomains(env.e.DB()).Add(domainAdmin(env.e), domain, kind); err != nil {
			t.Fatalf("adding %s as %s: %v", domain, kind, err)
		}
	}
}

func registerOwnDomain(domain string) func(*testing.T, captureEnv) {
	return func(t *testing.T, env captureEnv) {
		t.Helper()
		if _, err := capturemod.NewOwnDomainStore(env.e.DB()).Add(domainAdmin(env.e), domain); err != nil {
			t.Fatalf("registering %s: %v", domain, err)
		}
	}
}

// carvableConsumerDomain is a shipped consumer domain other than skip that a
// carve-out can name. A carve-out is stored as a registrable domain, so the
// domain must be its own registrable domain.
func carvableConsumerDomain(t *testing.T, skip string) string {
	t.Helper()
	for _, domain := range freemail.Domains() {
		if domain != skip && freemail.Registrable(domain) == domain {
			return domain
		}
	}
	t.Fatal("the shipped consumer list holds no registrable domain")
	return ""
}

// connectGoogleCalendar connects the owner's stubbed Google Calendar holding
// one accepted meeting with guest. Its account label seeds the owner's domain
// as a candidate own domain before the pull.
func connectGoogleCalendar(t *testing.T, env captureEnv, owner, guest string) ids.UUID {
	t.Helper()
	return connectCalendarOf(t, env, owner, []map[string]any{
		lateGuestEvent("meeting", "confirmed", captureNow.AddDate(0, 0, -16), owner,
			selfEntry(owner, "accepted"), guestEntry(guest, "accepted")),
	})
}

// unlabelledCalendar is a calendar connector that cannot name its account, so
// the registry seeds no candidate domain for it. Its events go through the
// shared calendar rules and the real sink.
type unlabelledCalendar struct{ owner, guest string }

// unlabelledProvider is a calendar provider the connection table admits and
// this env registers no real connector for.
const unlabelledProvider = "graphcal"

func (c *unlabelledCalendar) Descriptor() connector.Descriptor {
	return connector.Descriptor{
		Name: unlabelledProvider, Version: "1",
		Scopes:   []principal.Scope{principal.ScopeRead},
		RiskTier: mcp.TierAutoExecute,
		Produces: []datasource.EntityType{datasource.EntityActivity},
	}
}

func (c *unlabelledCalendar) Authenticate(context.Context, connector.AuthRequest) (connector.Auth, error) {
	return connector.Auth("token"), nil
}

func (c *unlabelledCalendar) Sync(ctx context.Context, _ connector.Auth, _ connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	raw, err := json.Marshal(meetingmap.Event{
		ID: "meeting", Subject: "Kick-off", StartsAt: captureNow.AddDate(0, 0, -16),
		Organizer: meetingmap.Actor{Email: c.owner},
		Attendees: []meetingmap.Actor{{Email: c.owner}, {Email: c.guest}},
	})
	if err != nil {
		return nil, err
	}
	decode := func(raw []byte, _ string) (meetingmap.Event, error) {
		var ev meetingmap.Event
		return ev, json.Unmarshal(raw, &ev)
	}
	return connector.Cursor(`{}`), meetingmap.CaptureOne(ctx, raw, sink, c.owner, unlabelledProvider, decode)
}

func (c *unlabelledCalendar) Normalize(context.Context, connector.RawRecord) ([]connector.NormalizedRecord, error) {
	return nil, connector.ErrSkip
}

func (c *unlabelledCalendar) HealthCheck(context.Context, connector.Auth) error { return nil }

func connectUnlabelledCalendar(t *testing.T, env captureEnv, owner, guest string) ids.UUID {
	t.Helper()
	env.registry.Register(&unlabelledCalendar{owner: owner, guest: guest})
	connID, err := env.registry.Connect(humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead}), unlabelledProvider, connector.Auth("token"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return connID
}

// connectUnlabelledCalendarAfterAMailboxOnItsDomain first syncs a labelled
// calendar of a second address on the owner's domain. That sync contributes the
// domain as a candidate. Then it connects the unlabelled calendar.
func connectUnlabelledCalendarAfterAMailboxOnItsDomain(t *testing.T, env captureEnv, owner, guest string) ids.UUID {
	t.Helper()
	labelled := connectCalendarOf(t, env, "other@"+domainOfAddress(owner), nil)
	if err := env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), env.e.WS), labelled); err != nil {
		t.Fatalf("SyncOnce of the labelled calendar: %v", err)
	}
	return connectUnlabelledCalendar(t, env, owner, guest)
}

// domainOfAddress is the part of address after its last "@".
func domainOfAddress(address string) string {
	return address[strings.LastIndex(address, "@")+1:]
}

// A meeting whose parties are all on the calendar owner's domain is dropped
// only when the workspace's own decision says it is internal. A consumer domain
// never says so. While no own domain is trusted, any other owner domain does.
// Once one is, the owner's domain must be claimed, confirmed or a candidate a
// connected mailbox contributed. Each setting has a meeting that is captured
// and one that is not.
func TestTheOwnersDomainDropsAMeetingOnlyWhereTheWorkspaceCallsItInternal(t *testing.T) {
	shipped := freemail.Domains()[0]
	carvedOut := carvableConsumerDomain(t, shipped)
	const added = "regional-provider.example"
	const outsider = "buyer@outside-co.example"
	const foreign = "client-co.example"
	type setups = []func(*testing.T, captureEnv)
	untrusted := setups{withoutTrustedDomains}
	addedAsConsumer := setups{withoutTrustedDomains, addConsumerDomain(capturemod.FreemailKindExtra, added)}
	carvedOutAsOwn := setups{addConsumerDomain(capturemod.FreemailKindNever, carvedOut), registerOwnDomain(carvedOut)}
	cases := []struct {
		name     string
		owner    string
		setup    []func(*testing.T, captureEnv)
		guest    string
		captured bool
		connect  func(t *testing.T, env captureEnv, owner, guest string) ids.UUID
	}{
		{"registered owner, outside guest", captureOwner, nil, outsider, true, connectGoogleCalendar},
		{"registered owner, colleague", captureOwner, nil, "peer@myco.example", false, connectGoogleCalendar},
		{"owner on an unregistered candidate domain, same domain", "rep@client-co.example", nil, "buyer@client-co.example", false, connectGoogleCalendar},
		{"owner on an unregistered candidate domain, colleague", "rep@client-co.example", nil, "peer@myco.example", false, connectGoogleCalendar},
		{"no trusted domain, same domain", captureOwner, untrusted, "peer@myco.example", false, connectGoogleCalendar},
		{"no trusted domain, outside guest", captureOwner, untrusted, outsider, true, connectGoogleCalendar},
		{"shipped consumer owner, same provider", "rep@" + shipped, nil, "friend@" + shipped, true, connectGoogleCalendar},
		{"shipped consumer owner, no trusted domain", "rep@" + shipped, untrusted, "friend@" + shipped, true, connectGoogleCalendar},
		{"shipped consumer owner, colleague", "rep@" + shipped, nil, "peer@myco.example", false, connectGoogleCalendar},
		{"workspace-added consumer owner, same provider", "rep@" + added, addedAsConsumer, "friend@" + added, true, connectGoogleCalendar},
		{"unlisted owner domain, no trusted domain", "rep@" + added, untrusted, "friend@" + added, false, connectGoogleCalendar},
		{"carved-out consumer registered as own, same domain", "rep@" + carvedOut, carvedOutAsOwn, "peer@" + carvedOut, false, connectGoogleCalendar},
		{"carved-out consumer registered as own, outside guest", "rep@" + carvedOut, carvedOutAsOwn, outsider, true, connectGoogleCalendar},
		{"owner on a foreign domain no mailbox contributed, same domain", "rep@" + foreign, nil, "buyer@" + foreign, true, connectUnlabelledCalendar},
		{"owner on a foreign domain another mailbox contributed, same domain", "rep@" + foreign, nil, "buyer@" + foreign, false, connectUnlabelledCalendarAfterAMailboxOnItsDomain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			for _, setup := range tc.setup {
				setup(t, env)
			}
			connID := tc.connect(t, env, tc.owner, tc.guest)
			if err := env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), env.e.WS), connID); err != nil {
				t.Fatalf("SyncOnce: %v", err)
			}
			var audience string
			err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(),
					`SELECT audience FROM activity WHERE kind = 'meeting' AND source_id = 'meeting'`).Scan(&audience)
			})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("reading the captured meeting: %v", err)
			}
			if captured := err == nil; captured != tc.captured {
				t.Fatalf("captured = %v (%v), want %v", captured, err, tc.captured)
			}
			if tc.captured && audience != "participants" {
				t.Errorf("audience = %q, want participants: a meeting filed under no record stays with its participants", audience)
			}
		})
	}
}
