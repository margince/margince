// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The customer's Telegram account and the chat the conversation was held in.
// A private chat's id is the account id, so one value serves both.
const (
	ChannelAccountID = "770011"
	ChannelThreadKey = "telegram:8100:" + ChannelAccountID
)

// ChannelConversation is what a captured Telegram conversation leaves behind
// for the contact ContactID, with the bot User connected.
type ChannelConversation struct {
	*AppEnv
	ActivityID string
	ContactID  string
	User       string
}

// BindIdentity writes the contact_channel_identity row an inbound message binds.
// Written as the table owner because the subject here is what the SEND path
// reads out of it, not how ingress puts it there.
func (c *ChannelConversation) BindIdentity(t *testing.T) {
	t.Helper()
	if err := InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
			VALUES ($1, 'telegram', $2, 'buyer', 'telegram', 'connector:telegram')`,
			c.ContactID, ChannelAccountID)
		return err
	}); err != nil {
		t.Fatalf("binding the channel identity: %v", err)
	}
}

// SeedInboundMessage writes the conversation being answered: an inbound telegram
// activity under the chat's thread key, linked to the contact. Capture leaves
// this shape, transport included. The reply path reads the provider from that
// column, and refuses every reply on an anchor seeded without it.
func (c *ChannelConversation) SeedInboundMessage(t *testing.T) {
	t.Helper()
	if err := InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity (kind, channel_provider, body, occurred_at, direction, source_system, source_id, source, captured_by, thread_key)
			VALUES ('message', 'telegram', 'Is this still available?', now(), 'inbound',
			        'telegram', '8100:770011:5', 'telegram:8100:770011:5', 'connector:telegram', $1)
			RETURNING id`, ChannelThreadKey).Scan(&c.ActivityID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, c.ActivityID, c.ContactID); err != nil {
			return err
		}
		// The customer's own authorship. The send path's evidence check
		// (authorIsTheSubject) reads it by account to tell a reply from an
		// unprompted first contact. A channel counterparty has no address.
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, channel_user_id, role)
			VALUES ($1, $2, 'from')`, c.ActivityID, ChannelAccountID)
		return err
	}); err != nil {
		t.Fatalf("seeding the inbound conversation: %v", err)
	}
}

// GrantConsent records an active grant for one purpose. The gate is per PURPOSE,
// so this is what a send under that purpose needs and a send under any other
// still lacks.
func (c *ChannelConversation) GrantConsent(t *testing.T, key string) {
	t.Helper()
	var purposes struct {
		Data []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if status := c.Call(t, "GET", "/v1/consent-purposes", nil, nil, &purposes); status != http.StatusOK {
		t.Fatalf("list purposes → %d", status)
	}
	var purposeID string
	for _, p := range purposes.Data {
		if p.Key == key {
			purposeID = p.ID
		}
	}
	if purposeID == "" {
		t.Fatalf("bootstrap seeded no %q purpose: %+v", key, purposes.Data)
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+c.ContactID+"/consent", map[string]any{
		"purpose_id": purposeID, "new_state": "granted", "lawful_basis": "consent",
		"wording": "Yes, you may contact me about this.",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("record consent → %d", status)
	}
}

// ConnectBot writes the workspace's live bot binding. The pre-flight reads its
// liveness and never resolves the credential ref, so the ref is an opaque value.
func (c *ChannelConversation) ConnectBot(t *testing.T) {
	t.Helper()
	if err := InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO channel_connection
				(provider, channel_id, channel_label, credential_ref, status, connected_by)
			VALUES ('telegram', '8100', '@fablebot', 'vault:token', 'connected', $1)`,
			c.User)
		return err
	}); err != nil {
		t.Fatalf("connecting the workspace bot: %v", err)
	}
}
