// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannelwire_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e channelEnv) staged(t *testing.T, name string) ids.UUID {
	t.Helper()
	up, err := e.store.StageUpload(e.member, name, bytes.NewReader([]byte("%PDF-1.4 "+name)))
	if err != nil {
		t.Fatalf("staging %s: %v", name, err)
	}
	return up.ID
}

func TestAStagedFileLandsWithOneMessageOnly(t *testing.T) {
	e := setupPersonalChannels(t)
	file := e.staged(t, "quote.pdf")
	first := whatsapp("wamid.f1", "8490@s.whatsapp.net")
	first.UploadIDs = []ids.UUID{file}
	second := whatsapp("wamid.f2", "8490@s.whatsapp.net")
	second.UploadIDs = []ids.UUID{file}
	res := e.push(t, first, second)
	if res[0].Status != personalchannels.StatusLanded {
		t.Fatalf("first = %+v, want landed", res[0])
	}
	if res[1].Status != personalchannels.StatusRefused || !strings.Contains(res[1].Reason, "not one of yours") {
		t.Fatalf("second = %+v, want refused: the file was already taken", res[1])
	}
}

func TestAMessageNamingMoreFilesThanItMayCarryIsRefusedAndTheFilesStay(t *testing.T) {
	e := setupPersonalChannels(t)
	m := whatsapp("wamid.many", "8490@s.whatsapp.net")
	for i := range 21 {
		m.UploadIDs = append(m.UploadIDs, e.staged(t, "f"+string(rune('a'+i))+".pdf"))
	}
	res := e.push(t, m)
	if res[0].Status != personalchannels.StatusRefused || !strings.Contains(res[0].Reason, "at most 20 files") {
		t.Fatalf("result = %+v, want refused for carrying 21 files", res[0])
	}
	if n := e.count(t, `SELECT count(*) FROM personal_channel_upload`); n != 21 {
		t.Fatalf("%d staged files left, want all 21 kept for a corrected message", n)
	}
}

func TestAReplyTheClientSentComesBackAsADuplicateNotASecondMessage(t *testing.T) {
	e := setupPersonalChannels(t)
	landed := e.push(t, whatsapp("wamid.in", "8490@s.whatsapp.net"))
	activity := *landed[0].ActivityID
	e.WsExec(t, `INSERT INTO personal_channel_reply (connection_id, activity_id, delivery_key, network, thread_id, state, provider_message_id, settled_at)
		VALUES ($1, $2, 'dk-sent', 'whatsapp', '', 'sent', 'wamid.out', now())`, e.connection(t), activity)
	echo := whatsapp("wamid.out", "me")
	echo.Direction = personalchannels.DirectionOutbound
	echo.From, echo.To = echo.To, echo.From
	res := e.push(t, echo)
	if res[0].Status != personalchannels.StatusDuplicate || res[0].ActivityID == nil || *res[0].ActivityID != activity {
		t.Fatalf("result = %+v, want a duplicate of the reply's activity %v", res[0], activity)
	}
}
