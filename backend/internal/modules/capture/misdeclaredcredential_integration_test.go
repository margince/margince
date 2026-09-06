// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A transport that says its credential belongs to one member, on a capture that
// names no member.
//
// It is a misdeclaration rather than a runtime condition — the extension ingress
// resolves the member before capture runs, and refuses the ingest outright when
// the member has deposited nothing. But the Sink's door is wider than that one
// path, and both ways of carrying on are wrong: publishing the message defies
// whatever the workspace floor was set to, and holding it writes a row that
// satisfies no arm of the audience gate, which is the unreadable message this
// whole axis exists to prevent. So the capture is refused and the transport is
// named.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestAMemberBoundTransportWithNoMemberBehindItIsRefused(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	const provider = "orphan_chat"
	registerTransport(t, owner, provider, "per_member")

	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)))

	// The connector principal a workspace bot's ingest mints: acting for no
	// human, which is exactly the shape this transport declared it never has.
	_, err := sink.Upsert(channelSinkContext(ctx, ws, "connector:"+provider),
		aCensusShapedRecord(provider))
	if err == nil {
		t.Fatal("the capture succeeded — a member-bound transport with no member behind it must be refused, not published or held")
	}
	if !strings.Contains(err.Error(), provider) {
		t.Fatalf("Upsert → %v, want an error naming %q — the operator has to be told which transport is misdeclared", err, provider)
	}
	var landed int
	if err := owner.QueryRow(ctx,
		`SELECT count(*) FROM activity WHERE channel_provider = $1`, provider).Scan(&landed); err != nil {
		t.Fatalf("counting what landed: %v", err)
	}
	if landed != 0 {
		t.Errorf("%d activity rows landed, want none — the refusal must land nothing", landed)
	}
}

// A non-message may not name a transport at all, whatever credential it spends.
//
// This is the constraint decideBirthTx's reasoning rests on: `memberBound` means
// a record naming a member-bound transport, and it means a MESSAGE only because
// `activity_message_has_provider` is a biconditional. Without that, a meeting or
// a note on a member-bound transport would reach the birth rungs written for
// chats — and a meeting is a shared appointment however it travelled.
//
// Asserted for BOTH credential models, because a constraint that held for one
// would leave the other's reasoning resting on nothing. It is asked of the real
// insert rather than of the DDL text: a CHECK read as a string is a claim about
// the schema file, and this is a claim about what the database does.
func TestANonMessageMayNotNameATransportAtAll(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)))
	member := ids.NewV7()
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Kind Member')`,
		member, "kinds-"+member.String()+"@kinds.test"); err != nil {
		t.Fatalf("seeding the member: %v", err)
	}
	for _, model := range []struct{ name, provider, credential string }{
		{"member-bound", "kinds_member", "per_member"},
		{"workspace bot", "kinds_bot", "workspace_bot"},
	} {
		t.Run(model.name, func(t *testing.T) {
			registerTransport(t, owner, model.provider, model.credential)
			rec := aCensusShapedRecord(model.provider)
			fields, ok := rec.Fields.(capture.ActivityFields)
			if !ok {
				t.Fatalf("the fixture's fields are %T, not the activity fields this test rewrites", rec.Fields)
			}
			fields.Kind = meetingKindForTest
			rec.Fields = fields
			// A member behind it, so the misdeclaration guard cannot be what
			// refuses this and the assertion stays about the kind.
			capturing := withCapturingMember(
				channelSinkContext(ctx, ws, "connector:"+model.provider), member)
			if _, err := sink.Upsert(capturing, rec); err == nil {
				t.Fatalf("a %s on a %s transport was captured — decideBirthTx reads memberBound as "+
					"'a message on that credential' and nothing else holds that any more",
					meetingKindForTest, model.credential)
			}
			var landed int
			if err := owner.QueryRow(ctx,
				`SELECT count(*) FROM activity WHERE channel_provider = $1`, model.provider).Scan(&landed); err != nil {
				t.Fatalf("counting what landed: %v", err)
			}
			if landed != 0 {
				t.Errorf("%d activity rows landed, want none", landed)
			}
		})
	}
}

// meetingKindForTest is the non-message kind this file drives the constraint
// with. A meeting rather than a note because it is the kind a transport could
// most plausibly be argued to carry, and so the one whose refusal is worth
// pinning.
const meetingKindForTest = "meeting"

// A transport the installation has never registered is refused by name.
//
// The insert would fail on the foreign key a statement later, with a constraint
// name and no account of what is wrong. Asking the registry first is what turns
// that into a sentence an operator can act on — and it is asked first anyway,
// because whose credential the transport spends decides what the message is born
// as.
func TestAnUnregisteredTransportIsRefusedByName(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	const provider = "never_registered"

	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)))
	_, err := sink.Upsert(channelSinkContext(ctx, ws, "connector:"+provider),
		aCensusShapedRecord(provider))
	if err == nil {
		t.Fatal("the capture succeeded on a transport the installation does not know")
	}
	if !strings.Contains(err.Error(), provider) {
		t.Fatalf("Upsert → %v, want an error naming %q", err, provider)
	}
}

// aCensusShapedRecord is one inbound chat on a named transport — the shape both
// refusals above are asked about, so the difference between them is the registry
// and nothing else.
func aCensusShapedRecord(provider string) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: provider, SourceID: "9100:1"},
		Fields: capture.ActivityFields{
			Kind: "message", ChannelProvider: provider, Body: "whose message is this",
			Direction:  connector.DirectionInbound,
			OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		},
		Source:     provider + ":9100:1",
		CapturedBy: "connector:" + provider,
		Counterparty: connector.Counterparty{
			Direction:       connector.DirectionInbound,
			DisplayName:     "A Sender",
			ChannelIdentity: connector.ChannelIdentity{Provider: provider, ChannelUserID: "9100"},
		},
		ThreadKey: provider + ":9100",
	}
}

// registerTransport puts one transport in the registry under the credential
// model named, and takes it, and everything filed on it, away afterwards.
func registerTransport(t *testing.T, owner *pgx.Conn, provider, credential string) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
		 VALUES ($1, 'unit', $1, $2, true)`, provider, credential); err != nil {
		t.Fatalf("registering the %s transport %s: %v", credential, provider, err)
	}
	t.Cleanup(func() {
		// In dependency order: the registry row is the foreign-key parent of
		// every message filed on it.
		for _, statement := range []string{
			`DELETE FROM activity WHERE channel_provider = $1`,
			`DELETE FROM channel_provider WHERE provider = $1`,
		} {
			if _, err := owner.Exec(context.Background(), statement, provider); err != nil {
				t.Errorf("cleaning up after %s (%s): %v", provider, statement, err)
			}
		}
	})
}
