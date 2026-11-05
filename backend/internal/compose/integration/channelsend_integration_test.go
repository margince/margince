// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The channel reply over the real composition (telegram-oa design §8.1, §8.5):
// the 🟡 admission an agent caller meets, the human whose own click IS the
// approval, the default-deny consent gate, the unreachable contact who is refused
// before anything is staged, and the outbound activity that keeps a sent message
// on the timeline across a reload.
//
// It boots the api role WITH the connect registry, which is what makes the
// REQUEST-TIME PRE-FLIGHT live. That is load-bearing for this suite rather than
// incidental: the pre-flight asks whether a credential can transmit, and a
// workspace bot lives in channel_connection while a mailbox grant lives in
// capture_connection — so a pre-flight that read the mailbox table would refuse
// every reply here with a 422 about a mailbox nobody asked about, and every other
// test in the tree would still pass.

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

const channelSendBaseURL = "https://channel.example.test"

type channelSendEnv struct {
	*apptest.ChannelConversation
}

// setupChannelSend boots the api composition with the connect registry (so the
// send pre-flight is wired), then lays down what a captured Telegram
// conversation leaves behind: a contact, their channel identity, the inbound
// activity filed under the chat, and the workspace's bot binding.
func setupChannelSend(t *testing.T) *channelSendEnv {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: apptest.EarlyPool(t)})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(vault),
		// The Google app is configured only to make the connect registry — and
		// with it the pre-flight — exist. Nothing here sends mail.
		compose.WithGmailCapture(compose.GmailConfig{
			ClientID: "channel-id", ClientSecret: "channel-secret",
			StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: channelSendBaseURL,
		}, compose.CaptureConfig{}),
		compose.WithPublicBaseURL(channelSendBaseURL))
	apptest.BootstrapWorkspaceSession(t, e, "Channel Send E2E", "rep@fable.test", "Admin")

	c := &channelSendEnv{ChannelConversation: &apptest.ChannelConversation{AppEnv: e}}
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Telegram Buyer"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	c.ContactID = contact.ID
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM app_user WHERE email = $1`, "rep@fable.test").Scan(&c.User)
	}); err != nil {
		t.Fatalf("resolving the acting human: %v", err)
	}
	c.BindIdentity(t)
	c.SeedInboundMessage(t)
	c.GrantConsent(t, "transactional")
	c.ConnectBot(t)
	return c
}

// stripInboundAuthorship removes the customer's own "from" participant row
// seedInboundMessage plants, for a test that wants a purpose asked about with
// no reply evidence behind it — resolveCategory's thread arm would otherwise
// answer the question this test is about before any purpose is ever reached.
func (c *channelSendEnv) stripInboundAuthorship(t *testing.T) {
	t.Helper()
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`DELETE FROM activity_participant WHERE activity_id = $1 AND role = 'from'`, c.ActivityID)
		return err
	}); err != nil {
		t.Fatalf("stripping the inbound authorship: %v", err)
	}
}

// sendReply posts a reply with the given body and returns the status plus the
// problem's first validation code — the words the rep is shown, which is
// where the "what do I do about it" has to live.
func (c *channelSendEnv) sendReply(t *testing.T, purpose, body string, headers map[string]string) (status int, code, message string) {
	t.Helper()
	return c.sendReplyClaiming(t, purpose, body, "", headers)
}

// sendReplyClaiming is sendReply plus a claimed communication category, which
// is the only way to reach the channel door's own copy of the category
// refusal. Empty context omits the field entirely, so the ordinary caller
// above sends exactly the body it used to.
func (c *channelSendEnv) sendReplyClaiming(t *testing.T, purpose, body, context string, headers map[string]string) (status int, code, message string) {
	t.Helper()
	var answer struct {
		Code    string `json:"code"`
		Detail  string `json:"detail"`
		Details struct {
			Errors []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"details"`
	}
	payload := AnyMap{"body": body, "consent_purpose": purpose}
	if context != "" {
		payload["communication_context"] = context
	}
	status = c.Call(t, "POST", "/v1/activities/"+c.ActivityID+"/send-message", payload, headers, &answer)
	if errs := answer.Details.Errors; len(errs) > 0 {
		return status, errs[0].Code, errs[0].Message
	}
	return status, answer.Code, answer.Detail
}

// mintPassport issues an agent passport under the given scopes and returns its bearer token.
func (c *channelSendEnv) mintPassport(t *testing.T, scopes []string) string {
	t.Helper()
	var minted struct {
		Token string `json:"token"`
	}
	if status := c.Call(t, "POST", "/v1/passports", AnyMap{"label": "reply agent", "scopes": scopes}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	return minted.Token
}

// sendReplyAs mints a passport under the given scopes and sends the reply as
// that agent — the two-step pattern both scope tests below share.
func (c *channelSendEnv) sendReplyAs(t *testing.T, scopes []string, purpose string) (status int, code, message string) {
	t.Helper()
	token := c.mintPassport(t, scopes)
	return c.sendReply(t, purpose, "Yes — shipping Monday.", map[string]string{"Authorization": "Bearer " + token})
}

// stagedChannelDeliveries counts channel-shaped comms_outbound rows — the fact a
// refusal must not leave behind, and the fact an acceptance must.
func (c *channelSendEnv) stagedChannelDeliveries(t *testing.T) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM comms_outbound WHERE channel_user_id IS NOT NULL`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting staged channel deliveries: %v", err)
	}
	return n
}

// outboundActivities counts what the timeline gained. A refusal that logged one
// would show the rep a message that never left.
func (c *channelSendEnv) outboundActivities(t *testing.T) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity WHERE direction = 'outbound'`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting outbound activities: %v", err)
	}
	return n
}

// assertNoOutboundEffect fails if a refusal left behind a staged delivery or a
// logged outbound activity — the two facts every refusal path in this suite
// must NOT produce, named by the caller's description of the refusal.
func (c *channelSendEnv) assertNoOutboundEffect(t *testing.T, refusal string) {
	t.Helper()
	if n := c.stagedChannelDeliveries(t); n != 0 {
		t.Fatalf("%d deliveries staged behind %s, want 0", n, refusal)
	}
	if n := c.outboundActivities(t); n != 0 {
		t.Fatalf("%d outbound activities logged behind %s, want 0", n, refusal)
	}
}

// A write-only passport may not reach an outbound verb at all: the scope cap
// the granting human set is checked before the tier question is asked.
func TestSendMessageRefusesAWriteOnlyPassport(t *testing.T) {
	c := setupChannelSend(t)

	status, code, _ := c.sendReplyAs(t, []string{"read", "write"}, "transactional")

	if status != http.StatusForbidden || code != "scope_exceeds_grantor" {
		t.Fatalf("write-only passport reply → %d %q, want 403 scope_exceeds_grantor", status, code)
	}
	c.assertNoOutboundEffect(t, "a scope-refused send")
}

// A send-scoped passport replies WITHOUT an approval token, and the reply goes.
//
// It used to be refused with approval_required. What changed is the tier, not
// the gate: the passport carries the granting human's own seat and row scope,
// and `send` is a cap that human chose to lend, so asking them to confirm again
// made the agent surface weaker than the contact behind it. What still refuses a
// caller is the CAP — a passport never granted `send` cannot reach this at all,
// which TestOutboundVerbsRequireAnOutboundCap holds.
func TestSendMessageAcceptsASendScopedPassportWithoutAToken(t *testing.T) {
	c := setupChannelSend(t)

	status, code, detail := c.sendReplyAs(t, []string{"read", "send"}, "transactional")

	if status != http.StatusAccepted {
		t.Fatalf("agent reply on a send-scoped passport → %d %q (%s), want 202", status, code, detail)
	}
	if n := c.stagedChannelDeliveries(t); n != 1 {
		t.Fatalf("%d channel deliveries staged behind the accepted reply, want 1", n)
	}
}

// A passport its granting human never lent `send` is refused, and that is the
// boundary that matters: the tier decides whether a contact is asked twice, the
// scope decides whether the act was delegable at all.
func TestSendMessageRefusesAPassportWithoutTheSendCap(t *testing.T) {
	c := setupChannelSend(t)

	status, code, _ := c.sendReplyAs(t, []string{"read"}, "transactional")

	if status == http.StatusAccepted {
		t.Fatalf("a read-only passport sent a channel reply → %d %q", status, code)
	}
	c.assertNoOutboundEffect(t, "a send refused for want of the cap")
}

// The human's own action IS the approval (ADR-0055), so their reply carries no
// token and no idempotency key and is accepted — and it is accepted THROUGH the
// live pre-flight, which is the half that would silently refuse every channel
// reply if it asked about the wrong credential.
func TestSendMessageAcceptsAHumanCallerWithoutAToken(t *testing.T) {
	c := setupChannelSend(t)

	status, code, detail := c.sendReply(t, "transactional", "Yes — shipping Monday.", nil)

	if status != http.StatusAccepted {
		t.Fatalf("human reply → %d %q (%s), want 202", status, code, detail)
	}
	if n := c.stagedChannelDeliveries(t); n != 1 {
		t.Fatalf("%d channel deliveries staged behind an accepted reply, want 1", n)
	}
	// The staged row is channel-shaped and addresses the account the
	// conversation was held with — resolved by the server, never named by the
	// caller.
	var recipient, body, provider string
	var subject, messageID *string
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT channel_user_id, body, provider, subject, message_id
			   FROM comms_outbound WHERE channel_user_id IS NOT NULL`).
			Scan(&recipient, &body, &provider, &subject, &messageID)
	}); err != nil {
		t.Fatalf("reading the staged delivery: %v", err)
	}
	if recipient != apptest.ChannelAccountID {
		t.Fatalf("staged recipient = %q, want the conversation's account %q", recipient, apptest.ChannelAccountID)
	}
	if provider != "telegram" || body != "Yes — shipping Monday." {
		t.Fatalf("staged provider/body = %q/%q, want telegram and the rep's text", provider, body)
	}
	if subject != nil || messageID != nil {
		t.Fatalf("staged row carries mail columns (subject=%v message_id=%v); it is not channel-shaped", subject, messageID)
	}
}

// The pre-flight's other half, and the reason the acceptance above proves
// something: with no bot bound, the same reply is refused at request time with a
// sentence naming who fixes it — instead of a 202 and a delivery that can only
// park where the rep never looks.
func TestSendMessageRefusesWhenNoBotIsBoundForTheChannel(t *testing.T) {
	c := setupChannelSend(t)
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE channel_connection SET status = 'disconnected'`)
		return err
	}); err != nil {
		t.Fatalf("disconnecting the bot: %v", err)
	}

	status, code, detail := c.sendReply(t, "transactional", "Yes — shipping Monday.", nil)

	if status != http.StatusUnprocessableEntity || code != "channel_not_send_capable" {
		t.Fatalf("reply with no bot bound → %d %q, want 422 channel_not_send_capable", status, code)
	}
	if !strings.Contains(detail, "admin") {
		t.Fatalf("refusal detail %q does not say who has to fix it", detail)
	}
	if n := c.stagedChannelDeliveries(t); n != 0 {
		t.Fatalf("%d deliveries staged behind a refused reply, want 0", n)
	}
}

// An empty or whitespace-only body has nothing to transmit: Telegram refuses it
// outright, so accepting one buys a timeline entry claiming the rep answered and
// a delivery that can only walk the whole retry ladder before parking under a
// reason naming nothing the operator can act on. The contract's minLength is
// documentation — httperr.Decode performs no schema validation — so the refusal
// has to be a Go guard, and it has to land before anything is written.
func TestSendMessageRefusesAnEmptyBody(t *testing.T) {
	c := setupChannelSend(t)

	for _, body := range []string{"", "   \n\t "} {
		status, code, message := c.sendReply(t, "transactional", body, nil)

		if status != http.StatusUnprocessableEntity {
			t.Fatalf("reply with body %q → %d, want 422", body, status)
		}
		if code != "empty_message_body" {
			t.Fatalf("reply with body %q answered code %q, want an empty_message_body validation error", body, code)
		}
		if !strings.Contains(message, "type") {
			t.Fatalf("refusal detail %q does not tell the rep what to do", message)
		}
		c.assertNoOutboundEffect(t, fmt.Sprintf("an empty body %q", body))
	}
}

// Consent is default-deny PER PURPOSE: the contact granted `transactional` and
// nothing else, so a reply sent under another purpose is suppressed with the
// same 409 the mail path answers — the shape the composer relies on to keep the
// rep's drafted text on screen.
func TestSendMessageRefusesWithoutConsentForThePurpose(t *testing.T) {
	c := setupChannelSend(t)
	// Without the customer's own authorship the thread arm cannot answer for
	// this claim (resolveCategory runs it unconditionally, before any purpose
	// is asked about, and a reply is a reply whatever anyone claimed) — this
	// test is about a purpose with no consent behind it, so it must not also
	// double as a reply the customer never sent.
	c.stripInboundAuthorship(t)

	status, code, _ := c.sendReply(t, "marketing_email", "Yes — shipping Monday.", nil)

	if status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("reply under an ungranted purpose → %d %q, want 409 consent_not_granted", status, code)
	}
	c.assertNoOutboundEffect(t, "a suppressed reply")

	// AND IT LEAVES A REVIEW, like a refused mail does. A message the engine
	// stopped is the same piece of work whichever transport it was going out
	// on, and a rep reading a code with no record is the same dead end. The
	// channel path refused without recording anything until this landed.
	var reviews int
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_review WHERE resolved_at IS NULL`).Scan(&reviews); err != nil {
		t.Fatalf("counting the reviews: %v", err)
	}
	if reviews != 1 {
		t.Errorf("%d review(s) after a refused channel reply, want 1 — the rep read a code and "+
			"the installation kept no record of what it refused", reviews)
	}
}

// A contact who blocked the bot cannot be reached, and blocking does NOT archive
// the identity (D9) — so the conversation is still there, still readable, and the
// reply must be refused on reachability rather than accepted and parked.
func TestSendMessageRefusesAnUnreachableContact(t *testing.T) {
	c := setupChannelSend(t)
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact_channel_identity SET blocked_at = now() WHERE channel_user_id = $1`, apptest.ChannelAccountID)
		return err
	}); err != nil {
		t.Fatalf("blocking the identity: %v", err)
	}

	status, code, detail := c.sendReply(t, "transactional", "Yes — shipping Monday.", nil)

	if status != http.StatusUnprocessableEntity || code != "contact_unreachable" {
		t.Fatalf("reply to a blocked contact → %d %q, want 422 contact_unreachable", status, code)
	}
	if !strings.Contains(detail, "blocked") {
		t.Fatalf("refusal detail %q does not say why the contact cannot be reached", detail)
	}
	c.assertNoOutboundEffect(t, "an unreachable recipient")
}

// Without the outbound activity the UI's optimistic append vanishes the moment
// the rep reloads, which reads as "my message was lost". So the row is the
// feature, not bookkeeping: same conversation, same contact, the text that was
// sent.
func TestSentMessageLandsAsAnOutboundActivity(t *testing.T) {
	c := setupChannelSend(t)
	var sent struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Direction string `json:"direction"`
		Body      string `json:"body"`
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.ActivityID+"/send-message", AnyMap{
		"body": "On its way.", "consent_purpose": "transactional",
	}, nil, &sent); status != http.StatusAccepted {
		t.Fatalf("human reply → %d", status)
	}
	if sent.Kind != "message" || sent.Direction != "outbound" || sent.Body != "On its way." {
		t.Fatalf("logged activity = %+v, want an outbound message carrying the sent text", sent)
	}

	var threadKey, linkedContact string
	var deliveryActivity string
	if err := apptest.InWorkspace(c.AppEnv, t, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx,
			`SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, sent.ID).Scan(&threadKey); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`SELECT contact_id::text FROM activity_link WHERE activity_id = $1 AND entity_type = 'contact'`,
			sent.ID).Scan(&linkedContact); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT activity_id::text FROM comms_outbound WHERE channel_user_id IS NOT NULL`).Scan(&deliveryActivity)
	}); err != nil {
		t.Fatalf("reading what the reply wrote: %v", err)
	}
	// Capture joins an inbound reply against outbound activities on thread_key,
	// so a reply filed under any other key is a reply this conversation never
	// had — and the timeline would show it as a message out of nowhere.
	if threadKey != apptest.ChannelThreadKey {
		t.Fatalf("outbound activity thread_key = %q, want the conversation's %q", threadKey, apptest.ChannelThreadKey)
	}
	if linkedContact != c.ContactID {
		t.Fatalf("outbound activity links contact %s, want the conversation's %s", linkedContact, c.ContactID)
	}
	// The delivery names THIS activity: one transaction, one fact, so a receipt
	// recorded later lands on the row the rep is looking at.
	if deliveryActivity != sent.ID {
		t.Fatalf("the staged delivery anchors activity %s, want the one just logged (%s)", deliveryActivity, sent.ID)
	}
}
