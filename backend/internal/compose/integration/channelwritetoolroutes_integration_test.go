// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// send_message replies on a captured channel conversation. The route and the
// tool must accept a passport's reply alike: the same answer, and the same
// delivery waiting to go out.

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// A reply to a channel conversation, sent by a passport over either door, is
// accepted the same way and leaves one delivery of the same shape.
func TestSendMessageLeavesTheSameOnEitherDoor(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: apptest.EarlyPool(t)})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	// The slug is the acting seat's address: the channel fixtures look it up as
	// rep@fable.test.
	d := newTwoDoorsWith(t, "rep", []compose.Option{
		compose.WithKeyvault(vault),
		compose.WithGmailCapture(compose.GmailConfig{
			ClientID: "channel-id", ClientSecret: "channel-secret",
			StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: channelSendBaseURL,
		}, compose.CaptureConfig{}),
		compose.WithPublicBaseURL(channelSendBaseURL),
	}, "read", "write", "send")
	c := &channelSendEnv{AppEnv: d.e}
	c.contactID = createdID(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Telegram Buyer"})
	c.user = sessionUserID(t, d.e)
	c.bindIdentity(t)
	c.seedInboundMessage(t)
	c.grantConsent(t, "transactional")
	c.connectBot(t)

	rest := d.restStatus(t, "POST", "/v1/activities/"+c.activityID+"/send-message",
		AnyMap{"body": "Yes, shipping Monday.", "consent_purpose": "transactional"}, http.StatusAccepted)
	tool := d.tool(t, "send_message", map[string]any{
		"activity_id": c.activityID, "body": "Yes, shipping Monday.", "consent_purpose": "transactional",
	})
	// The route answers the reply it filed and the tool its id and a status, so
	// the two are compared on the stored reply and the delivery behind it.
	viaREST, viaTool := anIDIn(t, rest), anIDIn(t, tool)
	stored := func(id string) json.RawMessage {
		return d.restStatus(t, "GET", "/v1/activities/"+id, nil, http.StatusOK)
	}
	sent := sameRecords[struct {
		Kind            string `json:"kind"`
		Direction       string `json:"direction"`
		ChannelProvider string `json:"channel_provider"`
		Body            string `json:"body"`
		ThreadKey       string `json:"thread_key"`
		Audience        string `json:"audience"`
	}](t, "reply", stored(viaREST), stored(viaTool))
	if sent.Direction != "outbound" || sent.ChannelProvider != "telegram" || sent.Body != "Yes, shipping Monday." {
		t.Errorf("the reply is %+v, want an outbound telegram message with the text", sent)
	}
	type delivery struct{ Provider, Body, Purpose, ThreadKey string }
	deliveryOf := func(activity string) delivery {
		var out delivery
		if err := d.e.Owner.QueryRow(t.Context(),
			`SELECT provider, body, consent_purpose, coalesce(thread_key, '') FROM comms_outbound WHERE activity_id = $1`, activity).
			Scan(&out.Provider, &out.Body, &out.Purpose, &out.ThreadKey); err != nil {
			t.Fatalf("reading the delivery of %s: %v", activity, err)
		}
		return out
	}
	if rest, tool := deliveryOf(viaREST), deliveryOf(viaTool); rest != tool {
		t.Errorf("the route queued %+v and the tool %+v", rest, tool)
	}
	d.sameEffect(t, "reply", viaREST, viaTool)
}
