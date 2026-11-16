// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannelwire_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/personalchannelwire"
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var channelRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	RowScope: principal.RowScopeOwn,
	Objects: map[string]principal.ObjectGrant{
		"personal_channel": {Create: true, Read: true, Update: true},
		"activity":         {Create: true, Read: true, Update: true},
		"contact":          {Create: true, Read: true, Update: true},
	},
}

type channelEnv struct {
	*integration.Env
	store    *personalchannels.Store
	passport ids.UUID
	member   context.Context
	now      time.Time
}

func setupPersonalChannels(t *testing.T) channelEnv {
	t.Helper()
	e := integration.Setup(t)
	// A landing runs under the member's live grants, so Rep1 holds the seeded rep role.
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = 'rep'
		ON CONFLICT DO NOTHING`, e.Rep1)
	db := compose.InstallationDB(e.Pool)
	passport := e.SeedPassportFor(t, integration.OwnerConn(t), "Chat Bridge", e.Rep1)
	ce := channelEnv{
		Env:      e,
		store:    personalchannelwire.NewStore(e.Pool, db, compose.NewCaptureSink(e.Pool, compose.CaptureConfig{Blob: blobstore.NewMemory()})),
		passport: passport,
		member:   e.AgentOnPassport(t, e.Rep1, nil, channelRepPerms, ids.From[ids.PassportKind](passport)),
		now:      clocktest.Now(t),
	}
	ce.store = ce.store.WithClock(func() time.Time { return ce.now })
	if _, err := ce.store.SetEnabled(e.As(e.Rep1, nil, channelRepPerms), true); err != nil {
		t.Fatalf("turning personal channels on: %v", err)
	}
	return ce
}

func (e channelEnv) push(t *testing.T, msgs ...personalchannels.PushMessage) []personalchannels.PushResult {
	t.Helper()
	out, err := e.store.Push(e.member, msgs, e.now.Add(time.Minute))
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	return out
}

func (e channelEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func whatsapp(id, account string) personalchannels.PushMessage {
	return personalchannels.PushMessage{
		MessageID: id, Network: "whatsapp", Body: "Here is the signed quote",
		OccurredAt: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC),
		From:       personalchannels.Party{Account: account, Name: "An Tran"},
		To:         personalchannels.Party{Account: "me", Name: "Rep"},
	}
}

func TestAPushedMessageLandsWithItsFileAndSaysWhoSentItIn(t *testing.T) {
	e := setupPersonalChannels(t)
	up, err := e.store.StageUpload(e.member, "quote.pdf", bytes.NewReader([]byte("%PDF-1.4 quote")))
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	m := whatsapp("wamid.1", "8490@s.whatsapp.net")
	m.UploadIDs = []ids.UUID{up.ID}
	res := e.push(t, m)
	if res[0].Status != personalchannels.StatusLanded || res[0].ActivityID == nil {
		t.Fatalf("result = %+v, want landed with an activity", res[0])
	}
	activity := *res[0].ActivityID
	if n := e.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND kind = 'message' AND channel_provider = 'personal_channels'`, activity); n != 1 {
		t.Fatal("the message did not land as a personal_channels message")
	}
	if n := e.count(t, `SELECT count(*) FROM attachment WHERE entity_type = 'activity' AND entity_id = $1`, activity); n != 1 {
		t.Fatalf("the file did not land on the activity: %d attachments", n)
	}
	if n := e.count(t, `SELECT count(*) FROM personal_channel_message WHERE activity_id = $1 AND network = 'whatsapp' AND origin_kind = 'passport' AND origin_id = $2`, activity, e.passport); n != 1 {
		t.Fatal("the landing does not name the passport that sent it in")
	}
	if n := e.count(t, `SELECT count(*) FROM personal_channel_upload WHERE id = $1`, up.ID); n != 0 {
		t.Fatal("the staged file outlived the message that carried it")
	}
	if n := e.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND captured_by = $2`, activity, personalchannelwire.ConnectorID+":"+e.Rep1.String()); n != 1 {
		t.Fatal("the message was not captured by the minted connector")
	}
}

func TestAResentMessageIsADuplicate(t *testing.T) {
	e := setupPersonalChannels(t)
	first := e.push(t, whatsapp("wamid.2", "8491@s.whatsapp.net"))
	again := e.push(t, whatsapp("wamid.2", "8491@s.whatsapp.net"))
	if again[0].Status != personalchannels.StatusDuplicate || *again[0].ActivityID != *first[0].ActivityID {
		t.Fatalf("resend = %+v, want duplicate of %v", again[0], *first[0].ActivityID)
	}
}

func TestOneAccountOnTwoNetworksIsTwoIdentities(t *testing.T) {
	e := setupPersonalChannels(t)
	zalo := whatsapp("z.1", "8492")
	zalo.Network = "zalo"
	res := e.push(t, whatsapp("w.1", "8492"), zalo)
	for _, r := range res {
		if r.Status != personalchannels.StatusLanded {
			t.Fatalf("result = %+v", r)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM contact_channel_identity WHERE provider = 'personal_channels' AND channel_user_id IN ('whatsapp/8492', 'zalo/8492')`); n != 2 {
		t.Fatalf("bindings = %d, want one per network", n)
	}
}

func TestARefusedMessageDoesNotRefuseTheBatch(t *testing.T) {
	e := setupPersonalChannels(t)
	bad := whatsapp("bad.1", "has:colon")
	res := e.push(t, bad, whatsapp("ok.1", "8493"))
	if res[0].Status != personalchannels.StatusRefused || res[1].Status != personalchannels.StatusLanded {
		t.Fatalf("results = %+v", res)
	}
}

func TestAPushRunsOutOfTimeAndDefersTheRest(t *testing.T) {
	e := setupPersonalChannels(t)
	res, err := e.store.Push(e.member, []personalchannels.PushMessage{whatsapp("late.1", "8494")}, e.now.Add(-time.Second))
	if err != nil || res[0].Status != personalchannels.StatusDeferred {
		t.Fatalf("results = %+v, %v; want deferred", res, err)
	}
}
