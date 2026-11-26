// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A colleague forwards messages that name no author to a company chat account.
// The account asks that colleague, in the same chat, who wrote them, and files
// the batch on the customer the colleague names. Every case runs through
// capture.FileChatMessage and the production sweep, against fake provider
// servers.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/chatclarify"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/tracesweep"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sentMessage is what a fake provider server was asked to send.
type sentMessage struct{ to, text, business string }

// fakeProvider answers a network's send call and records it. fail, when set,
// answers the next calls instead.
type fakeProvider struct {
	mu     sync.Mutex
	sent   []sentMessage
	calls  int
	fail   func(call int) (status int, body string)
	server *httptest.Server
	// decode reads a request into what it sent, and answer builds the success
	// body for a message id.
	decode func(path string, body []byte) (sentMessage, bool)
	answer func(id string) string
}

func newFakeProvider(t *testing.T, decode func(path string, body []byte) (sentMessage, bool), answer func(id string) string) *fakeProvider {
	t.Helper()
	p := &fakeProvider{decode: decode, answer: answer}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading a send: %v", err)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		p.calls++
		if p.fail != nil {
			if status, reply := p.fail(p.calls); status != 0 {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, reply)
				return
			}
		}
		msg, ok := p.decode(r.URL.Path, body)
		if !ok {
			t.Errorf("unexpected request %s", r.URL.Path)
			return
		}
		p.sent = append(p.sent, msg)
		_, _ = io.WriteString(w, p.answer(strconv.Itoa(len(p.sent))))
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakeProvider) messages() []sentMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]sentMessage(nil), p.sent...)
}

func (p *fakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// clarifyNetwork is everything one network contributes to the cases below.
type clarifyNetwork struct {
	name string
	// provider is the binding's provider, and colleague the member's account.
	provider, colleague string
	scope               string
	verify, verifiedAt  string
	// connection is the channel_connection the account's messages arrive through.
	connection ids.UUID
	provider2  *fakeProvider
	askID      func(n int) string
	// card is the contact card of the customer the colleague names, and
	// customerID the identity the contact is found by.
	card       *connector.ChatSharedContact
	customerID string
	// outsideWindow answers a send the provider refuses without delivering, and
	// retryable says whether a later attempt can succeed.
	refuse    func(int) (int, string)
	retryable bool
	// questionText is what the colleague is asked.
	questionText string
	business     string
	vault        keyvault.Vault
	sweep        func(e *integration.SearchEnv, vault keyvault.Vault) chatclarify.Sweep
}

func clarifyNetworks(t *testing.T, e *integration.SearchEnv) []clarifyNetwork {
	t.Helper()
	vault := keyvault.NewMemory()
	sealed := func(secret string) string {
		ref, err := vault.Put(context.Background(), ids.From[ids.WorkspaceKind](e.WS), []byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return string(ref)
	}
	whatsappServer := newFakeProvider(t, func(path string, body []byte) (sentMessage, bool) {
		var req struct {
			To   string `json:"to"`
			Text struct {
				Body string `json:"body"`
			} `json:"text"`
		}
		ok := strings.HasSuffix(path, "/109/messages") && json.Unmarshal(body, &req) == nil
		return sentMessage{to: req.To, text: req.Text.Body}, ok
	}, func(id string) string { return `{"messages":[{"id":"wamid.ASK` + id + `"}]}` })
	zaloServer := newFakeProvider(t, func(path string, body []byte) (sentMessage, bool) {
		var req struct {
			Recipient struct {
				UserID string `json:"user_id"`
			} `json:"recipient"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
		}
		ok := path == "/v3.0/oa/message/cs" && json.Unmarshal(body, &req) == nil
		return sentMessage{to: req.Recipient.UserID, text: req.Message.Text}, ok
	}, func(id string) string {
		return `{"error":0,"message":"Success","data":{"message_id":"zalo-ask-` + id + `"}}`
	})
	telegramServer := newFakeProvider(t, func(path string, body []byte) (sentMessage, bool) {
		var req struct {
			ChatID   int64  `json:"chat_id"`
			Text     string `json:"text"`
			Business string `json:"business_connection_id"`
		}
		ok := strings.HasSuffix(path, "/sendMessage") && json.Unmarshal(body, &req) == nil
		return sentMessage{to: strconv.FormatInt(req.ChatID, 10), text: req.Text, business: req.Business}, ok
	}, func(id string) string { return `{"ok":true,"result":{"message_id":50` + id + `}}` })

	bot := insertConnection(t, e, "telegram", "bot-1", "", sealed("123:bot-token"), nil)
	telegramLink := insertConnection(t, e, "telegram_business", "biz-conn-1", "8800", "", &bot)
	whatsappConn := insertConnection(t, e, "whatsapp", "waba-1", "109", sealed("EAAG-token"), nil)
	zaloConn := insertConnection(t, e, "zalo_oa", "app-1", "88", sealed(`{"access_token":"zalo-access","refresh_token":"r"}`), nil)
	now := func() time.Time { return clocktest.Now(t) }

	networks := []clarifyNetwork{
		{
			name: "whatsapp", provider: "whatsapp", colleague: "491701234567", verify: `chat_whatsapp_phone`, verifiedAt: `chat_whatsapp_verified_at`, connection: whatsappConn,
			provider2: whatsappServer, askID: func(n int) string { return "wamid.ASK" + strconv.Itoa(n) },
			card:       &connector.ChatSharedContact{DisplayName: "Anna Weber", Phones: []string{"4915112345678"}},
			customerID: "4915112345678",
			refuse: func(int) (int, string) {
				return http.StatusBadRequest, `{"error":{"code":131047,"message":"re-engagement"}}`
			},
			questionText: "Who wrote this? Reply with their contact card or phone number.",
			sweep: func(e *integration.SearchEnv, v keyvault.Vault) chatclarify.Sweep {
				client := whatsapp.NewClient(whatsappServer.server.Client(), whatsappServer.server.URL, "v27.0", whatsapp.WithoutDialGuardForTest())
				return sweepWith(e, v, now, func(cs *capturemod.ChatSenders) { cs.WhatsApp = whatsapp.NewSender(client) })
			},
		},
		{
			name: "telegram", provider: "telegram", colleague: "5001", verify: `chat_telegram_user_id`, verifiedAt: `chat_telegram_verified_at`, connection: telegramLink,
			provider2: telegramServer, askID: func(n int) string { return "50" + strconv.Itoa(n) },
			card:       &connector.ChatSharedContact{DisplayName: "Anna Weber", UserID: "6002"},
			customerID: "6002", business: "biz-conn-1", retryable: true,
			refuse: func(int) (int, string) {
				return http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":1}}`
			},
			questionText: "Who wrote this? Reply with their contact card.",
			sweep: func(e *integration.SearchEnv, v keyvault.Vault) chatclarify.Sweep {
				api := telegram.NewAPI(telegramServer.server.Client(), telegramServer.server.URL)
				return sweepWith(e, v, now, func(cs *capturemod.ChatSenders) { cs.TelegramBusiness = telegram.New(api) })
			},
		},
		{
			name: "zalo", provider: "zalo_oa", scope: "88", colleague: "zu-1", verify: `chat_zalo_user_id`, verifiedAt: `chat_zalo_verified_at`, connection: zaloConn,
			provider2: zaloServer, askID: func(n int) string { return "zalo-ask-" + strconv.Itoa(n) },
			card:       &connector.ChatSharedContact{DisplayName: "Anna Weber", UserID: "z-customer"},
			customerID: "z-customer",
			refuse: func(int) (int, string) {
				return http.StatusOK, `{"error":-213,"message":"user has not interacted"}`
			},
			questionText: "Who wrote this? Reply with their contact card.",
			sweep: func(e *integration.SearchEnv, v keyvault.Vault) chatclarify.Sweep {
				client := zalo.NewClient(zaloServer.server.Client(), "", zaloServer.server.URL)
				return sweepWith(e, v, now, func(cs *capturemod.ChatSenders) { cs.Zalo = zalo.NewSender(client) })
			},
		},
	}
	for i := range networks {
		networks[i].vault = vault
	}
	return networks
}

// senderWrap, when set, wraps every network's sender in the sweep. A test acts
// with it between a provider accepting a send and the sweep recording it.
var senderWrap func(connector.MessageSender) connector.MessageSender

// sweepWith is the production sweep with one network's sender pointed at a fake.
func sweepWith(e *integration.SearchEnv, vault keyvault.Vault, now func() time.Time, point func(*capturemod.ChatSenders)) chatclarify.Sweep {
	return chatclarify.Sweep{
		Pool: e.Pool, Now: now, Log: slog.Default(),
		Senders: func(db *database.DB) capturemod.ChatSenders {
			senders := capturemod.NewChatSenders(db, vault, nil)
			point(&senders)
			if senderWrap != nil {
				senders.WhatsApp, senders.Zalo, senders.TelegramBusiness = senderWrap(senders.WhatsApp), senderWrap(senders.Zalo), senderWrap(senders.TelegramBusiness)
			}
			return senders
		},
	}
}

func insertConnection(t *testing.T, e *integration.SearchEnv, provider, channel, account, credential string, parent *ids.UUID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			INSERT INTO channel_connection (provider, channel_id, channel_label, credential_ref, events_credential_ref, status,
			                                connected_by, account_id, parent_connection_id, approved_at)
			VALUES ($1, $2, $2, $3, NULLIF($3, ''), 'connected', $4, NULLIF($5, ''), $6, now()) RETURNING id`,
			provider, channel, credential, e.Rep1, account, parent).Scan(&id)
	}); err != nil {
		t.Fatalf("inserting a %s connection: %v", provider, err)
	}
	return id
}

// bindingOf is the company account's binding for one network.
func (n clarifyNetwork) bindingOf() connector.ChatBinding {
	return connector.ChatBinding{
		Provider: n.provider, CapturedBy: "connector:" + n.provider, KeyScope: "77", ThreadScope: "77",
		AccountScope: n.scope, Ownership: connector.ChatOwnedByWorkspace, Connection: n.connection,
	}
}

func verifyColleague(t *testing.T, e *integration.SearchEnv, n clarifyNetwork) {
	t.Helper()
	extra := ""
	if n.provider == "zalo_oa" {
		extra = ", chat_zalo_oa_id = '88'"
	}
	wsExec(t, e, `UPDATE app_user SET `+n.verify+` = $2, `+n.verifiedAt+` = now()`+extra+` WHERE id = $1`, e.Rep1, n.colleague)
}

// forwardKey makes the stored original every held forward needs.
func storeOriginal(t *testing.T, e *integration.SearchEnv, source string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		id, err = capturemod.InsertRawCaptureTx(context.Background(), tx, capturemod.RawRecord{
			SourceSystem: "test", SourceID: source, Payload: []byte(`{"n":"` + source + `"}`),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (n clarifyNetwork) message(id, body string, at time.Time) connector.ChatMessage {
	return connector.ChatMessage{
		ConversationID: n.colleague, IdempotencyKey: id, Direction: connector.DirectionInbound,
		Counterparty: connector.ChatParty{UserID: n.colleague, DisplayName: "Rep"}, Body: body, OccurredAt: at,
	}
}

type clarifyCase struct {
	t    *testing.T
	e    *integration.SearchEnv
	n    clarifyNetwork
	sink connector.Sink
	ctx  context.Context
}

func newClarifyCase(t *testing.T, n clarifyNetwork, e *integration.SearchEnv, sink connector.Sink) *clarifyCase {
	t.Helper()
	verifyColleague(t, e, n)
	return &clarifyCase{t: t, e: e, n: n, sink: sink, ctx: connectorCtx(e, n.bindingOf())}
}

// forward files one held forward with its stored original and returns the
// activity.
func (c *clarifyCase) forward(id, body string) ids.UUID {
	c.t.Helper()
	msg := c.n.message(id, body, clocktest.Now(c.t).Add(-30*time.Minute))
	msg.Forward = &connector.ChatForward{}
	msg.StoredOriginal = storeOriginal(c.t, c.e, c.n.name+"-"+id)
	ref, err := capturemod.FileChatMessage(c.ctx, c.sink, c.n.bindingOf(), msg)
	if err != nil {
		c.t.Fatalf("forwarding %s: %v", id, err)
	}
	return ref.ID
}

// settle ages the activities, as the two minutes of quiet that close a batch.
func (c *clarifyCase) settle(activities ...ids.UUID) {
	c.t.Helper()
	for _, a := range activities {
		wsExec(c.t, c.e, `UPDATE activity SET created_at = now() - interval '10 minutes' WHERE id = $1`, a)
	}
}

func (c *clarifyCase) sweep() {
	c.t.Helper()
	if err := c.n.sweep(c.e, c.n.vault).Workspace(context.Background(), c.e.WS); err != nil {
		c.t.Fatalf("sweep: %v", err)
	}
}

// reply files the colleague's answer.
func (c *clarifyCase) reply(id, quoting, body string, card *connector.ChatSharedContact) error {
	msg := c.n.message(id, body, clocktest.Now(c.t))
	msg.ReplyToID, msg.SharedContact = quoting, card
	msg.StoredOriginal = storeOriginal(c.t, c.e, c.n.name+"-"+id)
	_, err := capturemod.FileChatMessage(c.ctx, c.sink, c.n.bindingOf(), msg)
	return err
}

func (c *clarifyCase) held(a ids.UUID) bool {
	c.t.Helper()
	return wsCount(c.t, c.e, `SELECT count(*) FROM activity a WHERE a.id = $1 AND `+pendingforward.Predicate("a"), a) == 1
}

func (c *clarifyCase) filedOnCustomer(a ids.UUID) bool {
	c.t.Helper()
	return wsCount(c.t, c.e, `SELECT count(*) FROM activity_link l JOIN contact_channel_identity i ON i.contact_id = l.contact_id
		WHERE l.activity_id = $1 AND i.provider = $2 AND i.channel_user_id = $3`, a, c.n.provider, c.n.customerID) == 1
}

func (c *clarifyCase) requireNoColleagueContact() {
	c.t.Helper()
	if n := wsCount(c.t, c.e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1`, c.n.colleague); n != 0 {
		c.t.Fatalf("the colleague got %d channel identities, want none", n)
	}
}

func eachClarifyNetwork(t *testing.T, run func(t *testing.T, c *clarifyCase)) {
	t.Helper()
	for _, name := range []string{"whatsapp", "telegram", "zalo"} {
		t.Run(name, func(t *testing.T) {
			e := newCaptureEnv(t).e
			sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
			for _, n := range clarifyNetworks(t, e) {
				if n.name == name {
					run(t, newClarifyCase(t, n, e, sink))
				}
			}
		})
	}
}

func TestAColleaguesForwardsWithoutAnAuthorAreAskedAboutOnceAndFiledOnTheCustomerTheyName(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a, b, d := c.forward("f1", "price for 20 seats?"), c.forward("f2", "and a discount?"), c.forward("f3", "by Friday?")
		c.sweep()
		if got := c.n.provider2.messages(); len(got) != 0 {
			t.Fatalf("a batch still open was asked about: %v", got)
		}
		c.settle(a, b, d)
		c.sweep()
		c.sweep()
		sent := c.n.provider2.messages()
		if len(sent) != 1 {
			t.Fatalf("questions sent = %d, want exactly one for the batch", len(sent))
		}
		if sent[0].to != c.n.colleague || sent[0].text != c.n.questionText || sent[0].business != c.n.business {
			t.Fatalf("question = %+v, want %q to %s through %q", sent[0], c.n.questionText, c.n.colleague, c.n.business)
		}

		err := c.reply("r1", c.n.askID(1), "[contacts]", c.n.card)
		if !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing the answer = %v, want it consumed", err)
		}
		for _, id := range []ids.UUID{a, b, d} {
			if !c.filedOnCustomer(id) {
				t.Fatalf("activity %s is not filed on the customer", id)
			}
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE kind = 'message' AND captured_by LIKE '%' || $1::text`, c.e.Rep1); n != 3 {
			t.Fatalf("messages submitted by the colleague = %d, want the 3 forwards", n)
		}
		c.requireNoColleagueContact()
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE kind = 'message'`); n != 3 {
			t.Fatalf("History holds %d messages, want only the 3 forwards", n)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM raw_capture WHERE source_id LIKE $1`, c.n.name+"-r%"); n != 0 {
			t.Fatalf("the answer's raw row survived")
		}
	})
}

func TestTheProvidersEchoOfAQuestionIsNeverFiled(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		echo := c.n.message(c.n.askID(1), c.n.questionText, clocktest.Now(t))
		echo.Direction, echo.StoredOriginal = connector.DirectionOutbound, storeOriginal(t, c.e, c.n.name+"-echo")
		_, err := capturemod.FileChatMessage(c.ctx, c.sink, c.n.bindingOf(), echo)
		if !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing the echo = %v, want it consumed", err)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE kind = 'message'`); n != 1 {
			t.Fatalf("History holds %d messages, want the one forward", n)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM raw_capture WHERE source_id = $1`, c.n.name+"-echo"); n != 0 {
			t.Fatal("the echo's raw row survived")
		}
	})
}

func TestAQuestionIDInAnotherChatNeverDeletesItsStoredOriginal(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		for _, legacy := range []bool{false, true} {
			if legacy {
				wsExec(t, c.e, `UPDATE raw_capture SET ask_message_ids = ARRAY[$1::text]
					WHERE id = (SELECT raw_capture_id FROM activity WHERE id = $2)`, c.n.askID(1), a)
			}
			msg := c.n.message(c.n.askID(1), "A colleague's own message", clocktest.Now(t))
			msg.ConversationID, msg.Direction = "another-chat", connector.DirectionOutbound
			msg.StoredOriginal = storeOriginal(t, c.e, fmt.Sprintf("%s-other-chat-%t", c.n.name, legacy))
			sink := compose.NewCaptureSink(c.e.Pool, compose.CaptureConfig{})
			consumed, err := sink.HandleClarification(c.ctx, c.n.bindingOf(), msg)
			if err != nil || consumed {
				t.Fatalf("other chat, legacy=%v: consumed=%v, err=%v", legacy, consumed, err)
			}
			if wsCount(t, c.e, `SELECT count(*) FROM raw_capture WHERE id = $1`, msg.StoredOriginal) != 1 {
				t.Fatal("a message in another chat lost its stored original")
			}
		}
	})
}

func TestAReplyQuotingAFollowUpDoesNotFileEitherBatch(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		first := c.forward("first", "first customer's price?")
		c.settle(first)
		c.sweep()
		second := c.forward("second", "second customer's delivery?")
		c.settle(second)
		c.sweep()
		if err := c.reply("unquoted", "", "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatal(err)
		}
		c.sweep()
		if len(c.n.provider2.messages()) != 3 {
			t.Fatal("the two questions and their follow-up were not sent")
		}
		if err := c.reply("quoted-follow-up", c.n.askID(3), "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatal(err)
		}
		if !c.held(first) || !c.held(second) {
			t.Fatal("an ambiguous follow-up filed a batch on a guess")
		}
	})
}

func TestAPhoneNumberInTextAnswersAWhatsAppQuestion(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	n := clarifyNetworks(t, e)[0]
	c := newClarifyCase(t, n, e, sink)
	a := c.forward("f1", "price?")
	c.settle(a)
	c.sweep()
	if err := c.reply("r1", n.askID(1), "+49 151 12345678", nil); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
		t.Fatalf("filing the answer = %v, want it consumed", err)
	}
	if !c.filedOnCustomer(a) {
		t.Fatal("the forward is not filed on the phone number's contact")
	}
	c.requireNoColleagueContact()
}

func TestTwoWaitingBatchesAndAnUnquotedAnswerGetOneFollowUpAndFileNothing(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		first1, first2 := c.forward("a1", "first batch one"), c.forward("a2", "first batch two")
		c.settle(first1, first2)
		c.sweep()
		second := c.forward("b1", "second batch")
		c.settle(second)
		c.sweep()
		if len(c.n.provider2.messages()) != 2 {
			t.Fatalf("questions sent = %d, want one for each batch", len(c.n.provider2.messages()))
		}
		if err := c.reply("r1", "", "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing the unquoted answer = %v, want it consumed", err)
		}
		for _, id := range []ids.UUID{first1, first2, second} {
			if !c.held(id) {
				t.Fatalf("activity %s was filed on a guess", id)
			}
		}
		c.sweep()
		c.sweep()
		sent := c.n.provider2.messages()
		if len(sent) != 3 {
			t.Fatalf("messages sent = %d, want the two questions and one follow-up", len(sent))
		}
		for _, want := range []string{"1.", "2.", "first batch one", "second batch"} {
			if !strings.Contains(sent[2].text, want) {
				t.Fatalf("the follow-up %q does not list %q", sent[2].text, want)
			}
		}

		if err := c.reply("r2", c.n.askID(1), "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing the quoted answer = %v, want it consumed", err)
		}
		if !c.filedOnCustomer(first1) || !c.filedOnCustomer(first2) {
			t.Fatal("the older batch is not filed on the customer")
		}
		if !c.held(second) {
			t.Fatal("the newer batch was filed by an answer to the older one")
		}
	})
}

func TestAnAnswerAfterTheBatchWasFiledIsUsedUpAndFilesNothingMore(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		if err := c.reply("r1", c.n.askID(1), "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatal(err)
		}
		before := wsCount(t, c.e, `SELECT count(*) FROM activity`)
		other := &connector.ChatSharedContact{DisplayName: "Someone Else", UserID: "9999", Phones: []string{"4917099999999"}}
		if err := c.reply("r2", c.n.askID(1), "[contacts]", other); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("a late answer = %v, want it used up", err)
		}
		if after := wsCount(t, c.e, `SELECT count(*) FROM activity`); after != before {
			t.Fatalf("a late answer added %d activities", after-before)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id IN ('9999', '4917099999999')`); n != 0 {
			t.Fatal("a late answer made a contact")
		}
		if len(c.n.provider2.messages()) != 1 {
			t.Fatal("a late answer was answered")
		}
	})
}

func TestAQuestionTheProviderRefusedWithoutDeliveringKeepsTheBatchHeldAndIsNeverAskedTwice(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		failing := true
		c.n.provider2.fail = func(int) (int, string) {
			if failing {
				return c.n.refuse(0)
			}
			return 0, ""
		}
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		c.sweep()
		if n := len(c.n.provider2.messages()); n != 0 {
			t.Fatalf("questions delivered = %d, want none while the provider refuses", n)
		}
		if !c.held(a) {
			t.Fatal("the forward left the held state")
		}
		failing = false
		if c.n.retryable {
			wsExec(t, c.e, `UPDATE raw_capture SET ask_next_at = now() - interval '1 second'`)
		}
		c.sweep()
		c.sweep()
		want := 0
		if c.n.retryable {
			want = 1
		}
		if n := len(c.n.provider2.messages()); n != want {
			t.Fatalf("questions delivered after the provider recovered = %d, want %d", n, want)
		}
	})
}

func TestAQuestionWhoseOutcomeIsUnknownIsNeverAskedAgain(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		c.n.provider2.fail = func(int) (int, string) { return http.StatusInternalServerError, `{}` }
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		calls := c.n.provider2.callCount()
		c.n.provider2.fail = nil
		c.sweep()
		c.sweep()
		if got := c.n.provider2.callCount(); got != calls || calls != 1 {
			t.Fatalf("provider calls = %d then %d, want one and no second try", calls, got)
		}
		if !c.held(a) {
			t.Fatal("the forward left the held state")
		}
	})
}

func TestAnUnansweredQuestionExpiresWithItsBatch(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		wsExec(t, c.e, `UPDATE activity SET created_at = now() - $2::interval WHERE id = $1`, a, (pendingforward.Window + time.Hour).String())
		purger := compose.NewCapturePurger(c.e.Pool, compose.NewRetentionServiceFor(compose.InstallationDB(c.e.Pool), nil, slog.Default()))
		if err := tracesweep.ExpireForwards(principal.SystemActing(workspaceConnectorCtx(c.e), "system:test-forward-expiry"), c.e.Pool, purger); err != nil {
			t.Fatal(err)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, a); n != 0 {
			t.Fatal("the expired forward kept its text")
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM raw_capture WHERE held_by IS NOT NULL`); n != 0 {
			t.Fatal("the expired forward kept its raw row and its question")
		}
	})
}

func TestForwardsApartInTimeAreAskedAboutAsSeparateBatches(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a, b, d := c.forward("f1", "one"), c.forward("f2", "two"), c.forward("f3", "three")
		wsExec(t, c.e, `UPDATE activity SET created_at = now() - interval '30 minutes' WHERE id = $1`, a)
		wsExec(t, c.e, `UPDATE activity SET created_at = now() - interval '29 minutes' WHERE id = $1`, b)
		wsExec(t, c.e, `UPDATE activity SET created_at = now() - interval '20 minutes' WHERE id = $1`, d)
		c.sweep()
		if n := len(c.n.provider2.messages()); n != 2 {
			t.Fatalf("questions sent = %d, want one for the pair and one for the single forward", n)
		}
	})
}

func TestACustomerConversationIsStillRefusedAndOnlyAVerifiedColleagueIsAsked(t *testing.T) {
	const thread = "whatsapp:biz:109:4915112345678"
	if err := connector.RefuseCaptureOnlyThread("whatsapp", thread); err == nil {
		t.Fatal("a reply into a customer's business conversation was allowed")
	}
	if err := connector.AllowChannelSend(connector.PurposeClarification, "whatsapp", thread, false); err == nil {
		t.Fatal("a question to someone who is no verified colleague was allowed")
	}
	if err := connector.AllowChannelSend(connector.PurposeReply, "whatsapp", thread, true); err == nil {
		t.Fatal("a reply was allowed because its recipient is a colleague")
	}
	if err := connector.AllowChannelSend(connector.PurposeClarification, "whatsapp", thread, true); err != nil {
		t.Fatalf("a question to a verified colleague was refused: %v", err)
	}
}

func TestAColleaguesOwnMessageToTheAccountIsNeverCustomerCorrespondence(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		if err := c.reply("t1", "", "thanks!", nil); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing a colleague's plain text = %v, want it consumed", err)
		}
		c.sweep()
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE kind = 'message'`); n != 0 {
			t.Fatalf("History holds %d messages, want none", n)
		}
		c.requireNoColleagueContact()
		if n := wsCount(t, c.e, `SELECT count(*) FROM raw_capture WHERE source_id = $1`, c.n.name+"-t1"); n != 0 {
			t.Fatal("the colleague's message kept its raw row")
		}
		if n := len(c.n.provider2.messages()); n != 0 {
			t.Fatalf("a colleague with no batch waiting got %d messages", n)
		}
	})
}

func TestAColleagueWhoAnswersWithNothingUsableIsHintedOncePerBatchAndNothingIsFiled(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		unusable := &connector.ChatSharedContact{DisplayName: "No Account"}
		for i, answer := range []struct {
			body string
			card *connector.ChatSharedContact
		}{{"thanks", nil}, {"[contacts]", unusable}, {"ok", nil}} {
			if err := c.reply(fmt.Sprintf("u%d", i), c.n.askID(1), answer.body, answer.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
				t.Fatalf("answer %d = %v, want it consumed", i, err)
			}
			c.sweep()
		}
		sent := c.n.provider2.messages()
		if len(sent) != 2 || !strings.HasPrefix(sent[1].text, "Please reply with the customer's contact card") {
			t.Fatalf("messages sent = %+v, want the question and one hint", sent)
		}
		if !c.held(a) {
			t.Fatal("the forward was filed on a guess")
		}
		c.requireNoColleagueContact()
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity WHERE kind = 'message'`); n != 1 {
			t.Fatalf("History holds %d messages, want the one forward", n)
		}
	})
}

// cancelAfterSend cancels the sweep once the provider accepted the send, so the
// write that records it fails.
type cancelAfterSend struct {
	inner  connector.MessageSender
	cancel func()
}

func (c cancelAfterSend) SendMessage(ctx context.Context, auth connector.Auth, msg connector.ChannelMessage) (connector.SendReceipt, error) {
	receipt, err := c.inner.SendMessage(ctx, auth, msg)
	c.cancel()
	return receipt, err
}

func TestAnAnswerIsStillRecognisedWhenTheRecordOfASuccessfulSendWasLost(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a := c.forward("f1", "price?")
		c.settle(a)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		senderWrap = func(inner connector.MessageSender) connector.MessageSender { return cancelAfterSend{inner, cancel} }
		t.Cleanup(func() { senderWrap = nil })
		if err := c.n.sweep(c.e, c.n.vault).Workspace(ctx, c.e.WS); err == nil {
			t.Fatal("the sweep recorded a send after its context ended")
		}
		senderWrap = nil
		c.sweep()
		c.sweep()
		if n := len(c.n.provider2.messages()); n != 1 {
			t.Fatalf("questions sent = %d, want the one whose record was lost and no second", n)
		}
		if err := c.reply("r1", "", "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing the answer = %v, want it consumed", err)
		}
		if !c.filedOnCustomer(a) {
			t.Fatal("a valid answer to a question that was sent was not used")
		}
	})
}

// archivingEnsurer is the production ensure with an expiry in front of it. The
// expiry runs between the answer's check and the ensure. On the first call it
// archives the forward being filed, as a purge would.
type archivingEnsurer struct {
	t     *testing.T
	e     *integration.SearchEnv
	store *contacts.Store
	calls []ids.UUID
	gone  []ids.UUID
}

func (r *archivingEnsurer) EnsureChannelCounterparty(ctx context.Context, req capturemod.EnsureChannelRequest) (capturemod.EnsureOutcome, error) {
	r.calls = append(r.calls, req.ActivityID)
	if len(r.calls) == 1 {
		wsExec(r.t, r.e, `UPDATE activity SET archived_at = now() WHERE id = $1`, req.ActivityID)
	}
	_, err := r.store.EnsureHeldChannelCounterparty(ctx, contacts.EnsureChannelCounterpartyInput{
		Identity: req.Identity, DisplayName: req.DisplayName, ActivityID: ids.From[ids.ActivityKind](req.ActivityID),
		Source: req.Source, CapturedBy: req.CapturedBy,
	})
	if errors.Is(err, contacts.ErrActivityNotHeld) {
		r.gone = append(r.gone, req.ActivityID)
		return capturemod.EnsureOutcome{}, capturemod.ErrHeldForwardGone
	}
	return capturemod.EnsureOutcome{}, err
}

func TestAHeldForwardThatWentAwayWhileTheAnswerWasFilingIsLeftAlone(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		a, b := c.forward("f1", "one"), c.forward("f2", "two")
		c.settle(a, b)
		c.sweep()
		ensurer := &archivingEnsurer{t: t, e: c.e, store: contacts.NewStore(compose.InstallationDB(c.e.Pool)).WithConsumerMail(capturemod.MatcherTx).WithFloorCarry(compose.EntityFloorCarry(c.e.Pool)).WithSenderStamp(activities.StampProvenSendersTx)}
		sink, ok := c.sink.(*capturemod.Sink)
		if !ok {
			t.Fatalf("the sink is a %T", c.sink)
		}
		c.sink = sink.WithChannelEnsurer(ensurer)
		participantsBefore := wsCount(t, c.e, `SELECT count(*) FROM activity_participant WHERE activity_id = ANY($1)`, []ids.UUID{a, b})
		if err := c.reply("r1", c.n.askID(1), "[contacts]", c.n.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
			t.Fatalf("filing the answer = %v, want it consumed", err)
		}
		if len(ensurer.calls) != 2 || len(ensurer.gone) != 1 {
			t.Fatalf("ensure calls = %d, found gone = %d, want 2 and 1", len(ensurer.calls), len(ensurer.gone))
		}
		gone := ensurer.gone[0]
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, gone); n != 0 {
			t.Fatalf("the archived forward was linked %d times", n)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM activity_participant WHERE activity_id = ANY($1)`, []ids.UUID{a, b}); n != participantsBefore {
			t.Fatalf("participants went from %d to %d, want none written", participantsBefore, n)
		}
		if n := wsCount(t, c.e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1`, c.n.customerID); n != 1 {
			t.Fatalf("customer identities = %d, want the one the still-held forward made", n)
		}
	})
}

func TestAnAnswerThatNamesAColleagueIsRefusedWithOneHintAndNothingIsFiled(t *testing.T) {
	eachClarifyNetwork(t, func(t *testing.T, c *clarifyCase) {
		other := newClarifyOther(t, c)
		a := c.forward("f1", "price?")
		c.settle(a)
		c.sweep()
		own := &connector.ChatSharedContact{DisplayName: "Rep", UserID: c.n.colleague, Phones: []string{c.n.colleague}}
		answers := []struct {
			name string
			body string
			card *connector.ChatSharedContact
		}{{"own card", "[contacts]", own}, {"another colleague's card", "[contacts]", c.n.card}}
		if c.n.provider == "whatsapp" {
			answers = append(answers, struct {
				name string
				body string
				card *connector.ChatSharedContact
			}{"a colleague's number", "+" + c.n.colleague, nil})
		}
		for i, answer := range answers {
			if err := c.reply(fmt.Sprintf("c%d", i), c.n.askID(1), answer.body, answer.card); !errors.Is(err, capturemod.ErrChatMessageConsumed) {
				t.Fatalf("%s: filing the answer = %v, want it consumed", answer.name, err)
			}
			c.sweep()
		}
		sent := c.n.provider2.messages()
		if len(sent) != 2 || !strings.Contains(sent[1].text, "not a colleague's") {
			t.Fatalf("messages sent = %+v, want the question and one hint that names colleagues", sent)
		}
		if !c.held(a) {
			t.Fatal("the forward was filed on a colleague")
		}
		c.requireNoColleagueContact()
		if n := wsCount(t, c.e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = $1`, other); n != 0 {
			t.Fatalf("another colleague got %d channel identities, want none", n)
		}
	})
}

// newClarifyOther confirms the customer card's account as a second colleague's.
func newClarifyOther(t *testing.T, c *clarifyCase) string {
	t.Helper()
	extra := ""
	if c.n.provider == "zalo_oa" {
		extra = ", chat_zalo_oa_id = '88'"
	}
	wsExec(t, c.e, `UPDATE app_user SET `+c.n.verify+` = $2, `+c.n.verifiedAt+` = now()`+extra+` WHERE id = $1`, c.e.Rep3, c.n.customerID)
	return c.n.customerID
}

func TestARateLimitedQuestionWaitsAsLongAsTheProviderSaid(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	n := clarifyNetworks(t, e)[1]
	c := newClarifyCase(t, n, e, sink)
	n.provider2.fail = func(int) (int, string) {
		return http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":300}}`
	}
	a := c.forward("f1", "price?")
	c.settle(a)
	c.sweep()
	c.sweep()
	if calls := n.provider2.callCount(); calls != 1 {
		t.Fatalf("provider calls = %d, want one while the provider says to wait", calls)
	}
	if got := wsScalar(t, e, `SELECT (ask_next_at > now() + interval '290 seconds')::text || '/' || ask_attempts FROM raw_capture WHERE ask_batch_id IS NOT NULL`); got != "true/1" {
		t.Fatalf("next try later than the provider said and attempts = %s, want true/1", got)
	}
}
