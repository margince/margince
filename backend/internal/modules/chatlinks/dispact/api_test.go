// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact/dispacttest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func serve(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.Client(), srv.URL+"/", "gwapp_test")
}

func TestAPostNamesItsChannelAndAnswersWithTheMessageTS(t *testing.T) {
	chat := dispacttest.New("team")
	chat.AddChannel("acme-deal")
	chat.Seed("acme-deal", dispacttest.Message{Seq: 41, Text: "earlier", PostedAt: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)})
	client := serve(t, chat)

	posted, err := client.Post(context.Background(), "acme-deal", "hello", "post-1", "")
	if err != nil {
		t.Fatal(err)
	}
	posts := chat.Posts()
	if len(posts) != 1 || posts[0].Channel != "acme-deal" || posts[0].ClientMsgID != "post-1" || posts[0].ThreadTS != "" {
		t.Fatalf("posts = %+v", posts)
	}
	if posted.ID != "0.000042" || posted.Seq != 42 {
		t.Fatalf("posted = %+v, want ts 0.000042 and seq 42", posted)
	}
	if err := client.Update(context.Background(), "acme-deal", posted.ID, "edited"); err != nil {
		t.Fatal(err)
	}
	if text, ok := chat.Updated(posted.ID); !ok || text != "edited" {
		t.Fatalf("update of %s = %q, %v", posted.ID, text, ok)
	}
}

func TestASecondCreateUnderOneKeyAnswersTheFirstChannel(t *testing.T) {
	client := serve(t, dispacttest.New("team"))
	first, err := client.CreateChannel(context.Background(), "margince:deal:1", "Acme")
	if err != nil || first.Slug == "" || !first.IsPrivate || !first.BotIsMember {
		t.Fatalf("first create = %+v, %v", first, err)
	}
	second, err := client.CreateChannel(context.Background(), "margince:deal:1", "Acme")
	if err != nil || second.Slug != first.Slug {
		t.Fatalf("second create = %+v, %v; want %s again", second, err, first.Slug)
	}
	byKey, err := client.ChannelByKey(context.Background(), "margince:deal:1")
	if err != nil || byKey.Slug != first.Slug {
		t.Fatalf("by key = %+v, %v", byKey, err)
	}
}

func TestAChannelIsReadFromUnderItsEnvelope(t *testing.T) {
	chat := dispacttest.New("team")
	chat.AddPublicChannel("open-room")
	client := serve(t, chat)
	ch, err := client.Channel(context.Background(), "open-room")
	if err != nil || ch.Slug != "open-room" || ch.IsPrivate || !ch.BotIsMember {
		t.Fatalf("channel = %+v, %v", ch, err)
	}
}

func TestARetriedPostAndAStrangeEditAreAnsweredAsDispactDoes(t *testing.T) {
	chat := dispacttest.New("team")
	chat.AddChannel("acme")
	client := serve(t, chat)
	first, err := client.Post(context.Background(), "acme", "card", "card-1", "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := client.Post(context.Background(), "acme", "card", "card-1", "")
	if err != nil || again.ID != first.ID || len(chat.Posts()) != 1 {
		t.Fatalf("a retried post = %+v, %v with %d posts; want the first post once", again, err, len(chat.Posts()))
	}
	if err := client.Update(context.Background(), "acme", "9.000009", "x"); !errors.Is(err, ErrChannelGone) {
		t.Errorf("editing a message the bot never posted = %v, want ErrChannelGone", err)
	}
	if _, err := client.Channel(context.Background(), "nowhere"); !errors.Is(err, ErrChannelGone) {
		t.Errorf("an unknown channel = %v, want ErrChannelGone", err)
	}
}

func TestMembersAreSortedByWhatDispactDidWithThem(t *testing.T) {
	client := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"added":["a@x"],"already_member":["b@x"],"failed":[` +
			`{"email":"c@x","reason":"user_not_found"},{"email":"d@x","reason":"user_inactive"},` +
			`{"email":"e@x","reason":"not_in_audience"},{"email":"f@x","reason":"add_failed"}]}`))
	}))
	got, err := client.AddMembers(context.Background(), "acme", []string{"a@x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Added) != 1 || len(got.AlreadyMember) != 1 || len(got.NotInChat) != 2 || len(got.NotAdded) != 2 {
		t.Fatalf("members = %+v; want 1 added, 1 already in, 2 without an account, 2 not added", got)
	}
}

func TestMeReadsTheTeam(t *testing.T) {
	bot, err := serve(t, dispacttest.New("team-7")).Me(context.Background())
	if err != nil || bot.TeamID != "team-7" || bot.ID == "" {
		t.Fatalf("me = %+v, %v", bot, err)
	}
}

func TestARefusalSentWithStatus200IsTyped(t *testing.T) {
	cases := []struct {
		code string
		want error
	}{
		{"invalid_auth", ErrTokenRejected},
		{"not_in_channel", ErrNotMember},
		{"channel_not_found", ErrChannelGone},
		{"message_not_found", ErrChannelGone},
		{"internal_error", ErrUnreachable},
		{"invalid_payload", ErrRequestRejected},
	}
	for _, c := range cases {
		client := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"ok":false,"error":"` + c.code + `","detail":"secret channel text"}`))
		}))
		_, err := client.Channel(context.Background(), "x")
		if !errors.Is(err, c.want) {
			t.Errorf("error %q = %v, want %v", c.code, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), "secret channel text") {
			t.Errorf("error %q copied the body into the error", c.code)
		}
	}
}

func TestAnAnswerWithoutOKIsAFault(t *testing.T) {
	client := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"slug":"acme","is_private":true}`))
	}))
	if _, err := client.Channel(context.Background(), "acme"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("an answer in another shape = %v, want ErrUnreachable", err)
	}
}

func TestEachHTTPRefusalIsTyped(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrTokenRejected},
		{http.StatusForbidden, ErrNotMember},
		{http.StatusNotFound, ErrChannelGone},
		{http.StatusBadGateway, ErrUnreachable},
		{http.StatusBadRequest, ErrRequestRejected},
	}
	for _, c := range cases {
		client := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
		}))
		if _, err := client.Channel(context.Background(), "x"); !errors.Is(err, c.want) {
			t.Errorf("status %d = %v, want %v", c.status, err, c.want)
		}
	}
}

func TestARateLimitNamesItsWait(t *testing.T) {
	client := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	_, err := client.Me(context.Background())
	var limited *connector.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter != 12*time.Second {
		t.Fatalf("rate limit = %v, want a RateLimitedError waiting 12s", err)
	}
}

func TestHistoryDecodesTheSeqThreadAndCursor(t *testing.T) {
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	chat := dispacttest.New("team")
	chat.AddChannel("acme")
	chat.Seed("acme",
		dispacttest.Message{Seq: 1_000_041, Text: "old", PostedAt: at},
		dispacttest.Message{Seq: 1_000_042, Text: "root", ReplyCount: 1, UserName: "Rita", PostedAt: at},
		dispacttest.Message{Seq: 1_000_043, Text: "reply", ThreadSeq: 1_000_042, UserName: "Tom", PostedAt: at},
		dispacttest.Message{Seq: 1_000_044, Text: "card", Bot: true, PostedAt: at},
	)
	client := serve(t, chat)

	page, err := client.History(context.Background(), "acme", 1_000_041, 500)
	if err != nil || len(page.Messages) != 3 {
		t.Fatalf("history = %+v, %v", page, err)
	}
	root, reply, card := page.Messages[0], page.Messages[1], page.Messages[2]
	if root.Seq != 1_000_042 || root.TS != "1.000042" || root.ThreadTS != root.TS || !root.PostedAt.Equal(at) {
		t.Errorf("root = %+v", root)
	}
	if reply.ThreadTS != root.TS || reply.IsBot {
		t.Errorf("reply = %+v, want it in the root's thread", reply)
	}
	if !card.IsBot {
		t.Errorf("card = %+v, want a bot message", card)
	}
	if page.NextCursor == nil || *page.NextCursor != 1_000_044 {
		t.Errorf("next cursor = %v, want 1000044", page.NextCursor)
	}
}

func TestATSThatIsNotDispactsIsRefused(t *testing.T) {
	for _, ts := range []string{"42", "0.42", "x.000042", "0.00004x", ""} {
		if _, err := seqOf(ts); err == nil {
			t.Errorf("seqOf(%q) took a malformed ts", ts)
		}
	}
	if seq, err := seqOf("12.000345"); err != nil || seq != 12_000_345 {
		t.Errorf("seqOf(12.000345) = %d, %v", seq, err)
	}
}
