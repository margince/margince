// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// An admin enrols a shared mailbox as a source of internal operational mail.
// Colleague mail it receives is then read by its reader team and by no other
// seat, admins included.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	opsDeskAddress = "ops@myco.example"
	// opsDeskProvider is a mail provider, the only kind internal mail admits.
	opsDeskProvider = "graph"
)

// opsDesk connects ops@ read by team, and returns its connection, the fake it
// pulls through and a sync of it.
func opsDesk(t *testing.T, env captureEnv, team ids.UUID) (ids.UUID, *mailBatchConnector, func(t *testing.T, raws ...[]byte)) {
	t.Helper()
	conn := &mailBatchConnector{name: opsDeskProvider, accountLabel: opsDeskAddress}
	registry := compose.NewCaptureRegistry(env.e.Pool, newTestKeyvault(t, env.e), compose.CaptureConfig{
		Now: func() time.Time { return captureNow },
	})
	registry.Register(conn)
	id := integration.ConnectSharedMailbox(t, registry, env.e.WS, integration.SharedMailbox{
		Provider: opsDeskProvider, Address: opsDeskAddress, ReaderTeam: team, ConnectedBy: env.e.Rep1, Posture: capturemod.PostureHeld,
	})
	wsCtx := principal.WithWorkspaceID(context.Background(), env.e.WS)
	return id, conn, func(t *testing.T, raws ...[]byte) {
		t.Helper()
		conn.raws = raws
		if err := registry.SyncOnce(wsCtx, id); err != nil {
			t.Fatalf("shared mailbox SyncOnce: %v", err)
		}
	}
}

// colleague seeds a live seat at the workspace's own domain.
func colleague(t *testing.T, e *integration.SearchEnv, address string) ids.UUID {
	t.Helper()
	return e.SeedID(t, `INSERT INTO app_user (id, email, display_name, email_domain_folded) VALUES ($1, $2, $2, $3)`,
		address, owndomain.StoredOfAddress(address))
}

// internalMailAdmin is an admin seat that may change the internal-mail policy
// and read every activity a row scope allows.
func internalMailAdmin(e *integration.SearchEnv, user ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				"capture_settings": {Read: true, Update: true}, "activity": {Read: true}, "contact": {Read: true}, "company": {Read: true},
			},
		},
	})
}

func internalMailSettings(t *testing.T, e *integration.SearchEnv) *capturemod.SettingsStore {
	t.Helper()
	raw, cfg := configtest.StoredOverWith(t, e.Pool, capturemod.Definitions(), capturemod.ConfigSettings())
	return capturemod.NewSettings(raw, cfg)
}

func storedAuthorization(t *testing.T, e *integration.SearchEnv, id ids.UUID) json.RawMessage {
	t.Helper()
	var raw json.RawMessage
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT internal_mail_authorization FROM activity WHERE id = $1`, id).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the internal-mail authorization: %v", err)
	}
	return raw
}

func TestColleagueMailToAnEnrolledSharedMailboxIsReadByItsReaderTeamOnly(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	adminSeat := colleague(t, e, "admin@myco.example")
	admin := internalMailAdmin(e, adminSeat)
	box, conn, syncDesk := opsDesk(t, env, e.Team1)
	mail := func(id string) []byte {
		return emailCC("lena@myco.example", "Lena", "omar@myco.example", opsDeskAddress, id)
	}
	conn.receivedAt = clocktest.Now(t).Add(time.Hour)
	syncDesk(t, mail("ops-before@myco.example"))
	if got := len(emailIDs(t, e)); got != 0 {
		t.Fatalf("colleague mail to a shared mailbox nobody enrolled stored %d emails, want none", got)
	}

	store := internalMailSettings(t, e)
	sources, err := store.ListInternalMailSources(admin)
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, source := range sources {
		offered = offered || (source.ConnectionID == box && source.OwnerUserID == nil && source.AccountLabel == opsDeskAddress)
	}
	if !offered {
		t.Fatalf("the source picker offered %+v, want the shared mailbox by its address and with no owner", sources)
	}
	if _, err := store.SetInternalMailPolicy(admin, true, []capturemod.InternalMailSelection{{ConnectionID: box, Containers: []string{}}}); err != nil {
		t.Fatalf("enrolling the shared mailbox: %v", err)
	}
	syncDesk(t, mail("ops-admitted@myco.example"))
	id := oneActivityID(t, e)
	var source struct {
		ConnectionID ids.UUID        `json:"connection_id"`
		Owner        json.RawMessage `json:"owner_user_id"`
	}
	if err := json.Unmarshal(storedAuthorization(t, e, id), &source); err != nil || source.ConnectionID != box || string(source.Owner) != "null" {
		t.Fatalf("the stored authorization names %+v (%v), want the shared connection and a null owner", source, err)
	}

	for who, reader := range map[string]struct {
		ctx  context.Context
		want bool
	}{
		"a reader-team member not on the mail": {readerAs(e, e.Rep1, e.Team1), true},
		"a seat outside the reader team":       {readerAs(e, e.Rep3, e.Team2), false},
		"an admin":                             {admin, false},
	} {
		if readable, _ := readsMail(reader.ctx, t, e, id); readable != reader.want {
			t.Errorf("%s reads the internal mail: %v, want %v", who, readable, reader.want)
		}
	}

	snapshot := storedAuthorization(t, e, id)
	if _, err := store.SetInternalMailPolicy(admin, true, []capturemod.InternalMailSelection{}); err != nil {
		t.Fatalf("removing the shared mailbox: %v", err)
	}
	syncDesk(t, mail("ops-after@myco.example"))
	if got := len(emailIDs(t, e)); got != 1 {
		t.Fatalf("after the source was removed the mailbox holds %d emails, want only the admitted one", got)
	}
	if after := storedAuthorization(t, e, id); string(after) != string(snapshot) {
		t.Fatalf("removing the source changed the admitted mail's authorization to %s", after)
	}
}

func TestASharedMailboxRefusesAHistoryImportItHasNoOwnerToStart(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	admin := internalMailAdmin(e, colleague(t, e, "admin@myco.example"))
	box, _, _ := opsDesk(t, env, e.Team1)
	since := clocktest.Now(t).UTC().Truncate(24*time.Hour).AddDate(0, 0, -7)
	_, err := internalMailSettings(t, e).SetInternalMailPolicy(admin, true, []capturemod.InternalMailSelection{{ConnectionID: box, Containers: []string{}, HistorySince: &since}})
	if _, ok := errors.AsType[settings.InvalidValue](err); !ok {
		t.Fatalf("enrolling a shared mailbox with a history import answered %v, want the setting refused as invalid", err)
	}
}
