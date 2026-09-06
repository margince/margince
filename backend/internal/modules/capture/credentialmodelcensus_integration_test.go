// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// No captured message is born readable by NOBODY — asked of every credential
// model the registry can hold, with the workspace floor down.
//
// The floor down is the state that makes a hold possible at all; with it up
// nothing narrows anything and the invariant is satisfied by a pipeline that
// ignores the axis entirely.
//
// The corpus is read from the CHECK constraint on
// `channel_provider.credential_model`, not listed here. A hand-listed one is a
// census that fails short: a third model added to the column would join the
// birth ladder with no case asserting what it produces, and this test would
// report PASS over a transport it never captured on. The coverage assertion is
// what makes that impossible — a value with no fixture fails HERE, where the
// message says what to write.
//
// Each model is captured under the principal its transports actually have, and
// that is the half a fixture is easiest to get wrong. A workspace bot acts for
// NO human, so its captured_by carries no member id and the audience gate's
// author arm cannot match it — which is precisely why a hold on such a row is
// unreadable rather than private. A census that gave the bot a member would
// prove nothing, because every row in it would have a reader by construction.

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension"
)

// censusTransport is one credential model's fixture: the transport declaring it
// and the member behind its captures, zero where the model has none.
type censusTransport struct {
	provider string
	member   ids.UUID
}

func TestEveryCredentialModelIsBornWithAReader(t *testing.T) {
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	member := ids.NewV7()
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Census Member')`,
		member, "census-"+member.String()+"@census.test"); err != nil {
		t.Fatalf("seeding the member: %v", err)
	}
	fixtures := map[string]censusTransport{
		string(extension.CredentialWorkspaceBot): {provider: "census_bot"},
		string(extension.CredentialPerMember):    {provider: "census_member", member: member},
	}
	for model, fixture := range fixtures {
		registerCensusTransport(t, owner, fixture.provider, model)
	}
	lowerTheFloor(ctx, t, pool, ws, member)

	models := credentialModelVocabulary(ctx, t, owner)
	if len(models) == 0 {
		t.Fatal("the registry admits no credential model at all — a census over an empty corpus proves nothing")
	}
	sink := capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)))
	for _, model := range models {
		fixture, ok := fixtures[model]
		if !ok {
			t.Fatalf("channel_provider admits credential_model %q and this census captures nothing on it — "+
				"add a transport declaring it above, or every message arriving on one is untested for the invariant", model)
		}
		id := captureCensusMessage(ctx, t, sink, ws, fixture)
		var reachable bool
		if err := database.WithWorkspaceTx(
			principal.WithWorkspaceID(ctx, ws), pool, func(tx pgx.Tx) error {
				var err error
				reachable, err = auth.ActivityHasAReaderTx(ctx, tx, id)
				return err
			}); err != nil {
			t.Fatalf("asking whether the %q message has a reader: %v", model, err)
		}
		if !reachable {
			var audience, reason string
			if err := owner.QueryRow(ctx,
				`SELECT audience, coalesce(audience_reason, '') FROM activity WHERE id = $1`,
				id).Scan(&audience, &reason); err != nil {
				t.Fatalf("reading back the %q message: %v", model, err)
			}
			t.Errorf("a message on a %q transport was born (%q, %q) and no human can open it",
				model, audience, reason)
		}
	}
}

// registerCensusTransport puts one transport in the registry and takes it, and
// everything filed on it, away afterwards.
func registerCensusTransport(t *testing.T, owner *pgx.Conn, provider, model string) {
	t.Helper()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
		 VALUES ($1, 'unit', $1, $2, true)`, provider, model); err != nil {
		t.Fatalf("registering the %s transport: %v", model, err)
	}
	t.Cleanup(func() {
		// In dependency order: the registry row is the foreign-key parent of
		// every message filed on it.
		for _, statement := range []string{
			`DELETE FROM capture_import WHERE activity_id IN (SELECT id FROM activity WHERE channel_provider = $1)`,
			`DELETE FROM activity WHERE channel_provider = $1`,
			`DELETE FROM channel_provider WHERE provider = $1`,
		} {
			if _, err := owner.Exec(context.Background(), statement, provider); err != nil {
				t.Errorf("cleaning up after %s (%s): %v", provider, statement, err)
			}
		}
	})
}

// lowerTheFloor turns workspace mail sharing off through the real settings
// store, so the census exercises the value the sink reads.
func lowerTheFloor(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ws, member ids.UUID) {
	t.Helper()
	off := false
	writer := principal.WithWorkspaceID(ctx, ws)
	writer = principal.WithCorrelationID(writer, ids.NewV7())
	writer = principal.WithActor(writer, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + member.String(), UserID: member,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	store := capture.NewSettings(settings.New(pool, settings.NewRegistry(capture.Definitions()...)))
	if _, err := store.Update(writer, capture.SettingsPatch{MailSharing: &off}); err != nil {
		t.Fatalf("lowering the workspace mail-sharing floor: %v", err)
	}
}

// credentialModelVocabulary reads the values the column admits, from the CHECK
// that admits them.
func credentialModelVocabulary(ctx context.Context, t *testing.T, owner *pgx.Conn) []string {
	t.Helper()
	var definition string
	if err := owner.QueryRow(ctx,
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		  WHERE conname = 'channel_provider_credential_model_check'`).Scan(&definition); err != nil {
		t.Fatalf("reading the credential-model vocabulary: %v", err)
	}
	var models []string
	for _, quoted := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(definition, -1) {
		models = append(models, quoted[1])
	}
	return models
}

// captureCensusMessage lands one inbound chat on the fixture's transport, under
// the principal that transport's captures actually run as.
func captureCensusMessage(
	ctx context.Context, t *testing.T, sink *capture.Sink, ws ids.UUID, fixture censusTransport,
) ids.UUID {
	t.Helper()
	rec := connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: fixture.provider, SourceID: "census:1"},
		Fields: capture.ActivityFields{
			Kind: "message", ChannelProvider: fixture.provider, Body: "a message the floor is down for",
			Direction:  connector.DirectionInbound,
			OccurredAt: time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
		},
		Source:     fixture.provider + ":census:1",
		CapturedBy: "connector:" + fixture.provider,
		Counterparty: connector.Counterparty{
			Direction:   connector.DirectionInbound,
			DisplayName: "A Sender",
			ChannelIdentity: connector.ChannelIdentity{
				Provider: fixture.provider, ChannelUserID: "census-account",
			},
		},
		ThreadKey: fixture.provider + ":census",
	}
	capturing := channelSinkContext(ctx, ws, "connector:"+fixture.provider)
	if fixture.member != ids.Nil {
		capturing = withCapturingMember(capturing, fixture.member)
	}
	ref, err := sink.Upsert(capturing, rec)
	if err != nil {
		t.Fatalf("capturing on %s: %v", fixture.provider, err)
	}
	return ref.ID
}

// withCapturingMember re-binds the acting connector to a member, which is what
// a per-member transport's ingest runs as: the connector identity wearing one
// human's authority.
func withCapturingMember(ctx context.Context, member ids.UUID) context.Context {
	actor, _ := principal.Actor(ctx)
	actor.UserID, actor.OnBehalfOf = member, member
	return principal.WithActor(ctx, actor)
}
