// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dispacttest is an in-memory Dispact bot API for tests. It answers in
// Dispact's own wire shape: HTTP 200 with an "ok" envelope, Slack-style "ts"
// strings and the cursor under "response_metadata". Every test that talks to
// a fake Dispact uses this one. A drift from the real shapes then fails in one
// place instead of passing in four.
package dispacttest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Message is one message in a channel's history as a test seeds it.
type Message struct {
	Seq        int64
	ThreadSeq  int64
	ReplyCount int
	UserName   string
	UserEmail  string
	Text       string
	PostedAt   time.Time
	Subtype    string
	Bot        bool
}

// Post is one message the bot sent.
type Post struct {
	Channel     string
	Text        string
	ClientMsgID string
	ThreadTS    string
	TS          string
}

// Server is the fake. Its zero value is not ready; use New.
type Server struct {
	mu       sync.Mutex
	teamID   string
	channels map[string]channel
	keys     map[string]string
	history  map[string][]Message
	common   []Message
	// everyChannel admits a channel no test added, as private with the bot in it.
	everyChannel bool
	posts        []Post
	invited      []string
	updates      map[string]string
	reads        map[string]int
	nextSeq      int64
}

type channel struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	IsPrivate   bool   `json:"is_private"`
	IsArchived  bool   `json:"is_archived"`
	BotIsMember bool   `json:"bot_is_member"`
}

// New makes a fake whose bot belongs to teamID. A channel the test never added
// or created is not found, as in Dispact.
func New(teamID string) *Server {
	return &Server{
		teamID: teamID, channels: map[string]channel{}, keys: map[string]string{},
		history: map[string][]Message{}, updates: map[string]string{}, reads: map[string]int{},
	}
}

// AddChannel adds a private channel with the bot in it.
func (s *Server) AddChannel(slug string) {
	s.put(channel{Slug: slug, Name: slug, IsPrivate: true, BotIsMember: true})
}

// AddChannelWithoutBot adds a private channel the bot is not in.
func (s *Server) AddChannelWithoutBot(slug string) {
	s.put(channel{Slug: slug, Name: slug, IsPrivate: true})
}

// AddPublicChannel adds a public channel with the bot in it.
func (s *Server) AddPublicChannel(slug string) {
	s.put(channel{Slug: slug, Name: slug, BotIsMember: true})
}

func (s *Server) put(ch channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[ch.Slug] = ch
}

// Seed adds messages to the end of a channel's history, in seq order.
func (s *Server) Seed(slug string, messages ...Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[slug] = append(s.history[slug], messages...)
	for _, m := range messages {
		s.nextSeq = max(s.nextSeq, m.Seq)
	}
}

// SeedEveryChannel admits every channel slug as private with the bot in it,
// each with this history unless Seed named it.
func (s *Server) SeedEveryChannel(messages ...Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.everyChannel = true
	s.common = append([]Message(nil), messages...)
	for _, m := range messages {
		s.nextSeq = max(s.nextSeq, m.Seq)
	}
}

// Invited are the emails the bot asked to add to a channel, in order.
func (s *Server) Invited() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.invited...)
}

// Posts are the messages the bot sent, oldest first.
func (s *Server) Posts() []Post {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Post(nil), s.posts...)
}

// Updated is the text a message was last rewritten to, by its ts.
func (s *Server) Updated(ts string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.updates[ts]
	return text, ok
}

// Reads counts the history reads of a channel.
func (s *Server) Reads(slug string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads[slug]
}

// bodyLimit bounds a request body, as Dispact's own server does.
const bodyLimit = 1 << 20

// The names the fake's answers repeat.
const (
	botID              = "bot-1"
	fieldBotID         = "bot_id"
	fieldText          = "text"
	fieldType          = "type"
	errChannelNotFound = "channel_not_found"
	fieldChannel       = "channel"
	fieldMessage       = "message"
	routeMessages      = "messages"
)

// tsShape is how Dispact writes a message ts.
var tsShape = regexp.MustCompile(`^[0-9]+\.[0-9]{6}$`)

// TS writes a seq the way Dispact writes a message ts.
func TS(seq int64) string { return fmt.Sprintf("%d.%06d", seq/1_000_000, seq%1_000_000) }

// ServeHTTP answers the bot API routes Margince calls.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("Authorization") == "" {
		reply(w, refuse("invalid_auth"))
		return
	}
	var body map[string]any
	if r.Method == http.MethodPost {
		raw, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit))
		if err != nil || json.Unmarshal(raw, &body) != nil {
			reply(w, refuse("invalid_payload"))
			return
		}
	}
	reply(w, s.route(r.Method, r.URL, body))
}

//craft:ignore naked-any body and the answer are the JSON shapes of each route
func (s *Server) route(method string, u *url.URL, body map[string]any) map[string]any {
	path := strings.TrimPrefix(u.Path, "/api/bot/")
	switch {
	case method == http.MethodGet && path == "me":
		return map[string]any{"ok": true, "team": "Test team", "team_id": s.teamID, fieldBotID: botID, "bot_user": "margince"}
	case method == http.MethodPost && path == "channels":
		return s.create(body)
	case method == http.MethodGet && path == "channels":
		return s.list()
	case method == http.MethodGet && strings.HasPrefix(path, "channel-keys/"):
		slug, ok := s.keys[strings.TrimPrefix(path, "channel-keys/")]
		if !ok {
			return refuse(errChannelNotFound)
		}
		return map[string]any{"ok": true, fieldChannel: s.channelOf(slug)}
	case method == http.MethodPost && path == routeMessages:
		return s.post(body)
	case method == http.MethodPost && path == "messages/update":
		return s.update(body)
	}
	return s.routeChannel(method, strings.TrimPrefix(path, "channels/"), u.Query(), body)
}

// list answers the channels the bot can see: every public channel and each
// private one it is in, by slug, archived ones left out, as Dispact does.
func (s *Server) list() map[string]any {
	out := []map[string]any{}
	for slug, ch := range s.channels {
		if ch.IsArchived || (ch.IsPrivate && !ch.BotIsMember) {
			continue
		}
		out = append(out, map[string]any{"id": "id-" + slug, "name": slug, "is_private": ch.IsPrivate, "is_archived": false})
	}
	return map[string]any{"ok": true, "channels": out, "response_metadata": map[string]any{"next_cursor": ""}}
}

// routeChannel answers the routes under /api/bot/channels/{slug}.
//
//craft:ignore naked-any body and the answer are the JSON shapes of each route
func (s *Server) routeChannel(method, path string, query url.Values, body map[string]any) map[string]any {
	slug, rest, _ := strings.Cut(path, "/")
	if _, known := s.lookup(slug); !known {
		return refuse(errChannelNotFound)
	}
	switch {
	case method == http.MethodGet && rest == "":
		return map[string]any{"ok": true, fieldChannel: s.channelOf(slug)}
	case method == http.MethodGet && rest == routeMessages:
		return s.read(slug, query)
	case method == http.MethodPost && rest == "members":
		emails, _ := body["emails"].([]any) //craft:ignore unchecked-type-assertion no emails adds no one
		for _, email := range emails {
			if address, ok := email.(string); ok {
				s.invited = append(s.invited, address)
			}
		}
		return map[string]any{"ok": true, "added": emails, "already_member": []string{}, "failed": []any{}}
	}
	return refuse("unknown_method")
}

//craft:ignore naked-any the answer is Dispact's refusal envelope
func refuse(code string) map[string]any { return map[string]any{"ok": false, "error": code} }

func (s *Server) lookup(slug string) (channel, bool) {
	if ch, ok := s.channels[slug]; ok {
		return ch, true
	}
	return channel{Slug: slug, Name: slug, IsPrivate: true, BotIsMember: true}, s.everyChannel
}

func (s *Server) channelOf(slug string) channel {
	ch, _ := s.lookup(slug)
	return ch
}

//craft:ignore naked-any body is the create request's JSON
func (s *Server) create(body map[string]any) map[string]any {
	key, _ := body["external_key"].(string) //craft:ignore unchecked-type-assertion a create may name no key
	name, _ := body["name"].(string)        //craft:ignore unchecked-type-assertion Dispact refuses a missing name below
	if name == "" {
		return refuse("invalid_payload")
	}
	if slug, ok := s.keys[key]; ok && key != "" {
		return map[string]any{"ok": true, fieldChannel: slug, "channel_id": "id-" + slug, "created": false, "bot_is_member": true}
	}
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + strconv.Itoa(len(s.channels)+1)
	s.channels[slug] = channel{Slug: slug, Name: name, IsPrivate: true, BotIsMember: true}
	if key != "" {
		s.keys[key] = slug
	}
	return map[string]any{"ok": true, fieldChannel: slug, "channel_id": "id-" + slug, "created": true, "bot_is_member": true}
}

//craft:ignore naked-any body is the post request's JSON
func (s *Server) post(body map[string]any) map[string]any {
	slug, _ := body["channel"].(string) //craft:ignore unchecked-type-assertion a missing channel is refused below
	text, _ := body[fieldText].(string) //craft:ignore unchecked-type-assertion a missing text is refused below
	ch, known := s.lookup(slug)
	if !known {
		return refuse(errChannelNotFound)
	}
	if !ch.BotIsMember {
		return refuse("not_in_channel")
	}
	if text == "" {
		return refuse("no_text")
	}
	clientMsgID, _ := body["client_msg_id"].(string) //craft:ignore unchecked-type-assertion a post may carry no client id
	thread, _ := body["thread_ts"].(string)          //craft:ignore unchecked-type-assertion a post outside a thread has none
	// Dispact answers a repeated client_msg_id with the first post.
	for _, p := range s.posts {
		if clientMsgID != "" && p.Channel == slug && p.ClientMsgID == clientMsgID && p.ThreadTS == thread {
			return map[string]any{
				"ok": true, fieldChannel: slug, "ts": p.TS,
				fieldMessage: map[string]any{fieldType: fieldMessage, fieldText: p.Text, "ts": p.TS, fieldBotID: botID},
			}
		}
	}
	s.nextSeq++
	ts := TS(s.nextSeq)
	s.posts = append(s.posts, Post{Channel: slug, Text: text, ClientMsgID: clientMsgID, ThreadTS: thread, TS: ts})
	return map[string]any{
		"ok": true, fieldChannel: slug, "ts": ts,
		fieldMessage: map[string]any{fieldType: fieldMessage, fieldText: text, "ts": ts, fieldBotID: botID},
	}
}

// update rewrites a message the bot posted; Dispact refuses any other ts.
//
//craft:ignore naked-any body is the update request's JSON
func (s *Server) update(body map[string]any) map[string]any {
	slug, _ := body["channel"].(string) //craft:ignore unchecked-type-assertion a missing channel matches no post
	ts, _ := body["ts"].(string)        //craft:ignore unchecked-type-assertion a missing ts matches no post
	text, _ := body[fieldText].(string) //craft:ignore unchecked-type-assertion a missing text is refused below
	if !tsShape.MatchString(ts) {
		return refuse("invalid_ts")
	}
	if text == "" {
		return refuse("no_text")
	}
	for _, p := range s.posts {
		if p.Channel == slug && p.TS == ts {
			s.updates[ts] = text
			return map[string]any{"ok": true, fieldChannel: slug, "ts": ts, fieldText: text}
		}
	}
	return refuse("message_not_found")
}

// read is the forward pull: messages after the seq, oldest first, with the
// next after in response_metadata.next_cursor even on the last page.
func (s *Server) read(slug string, query url.Values) map[string]any {
	s.reads[slug]++
	after, err := strconv.ParseInt(query.Get("after"), 10, 64)
	if err != nil {
		return refuse("invalid_arguments")
	}
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit <= 0 {
		limit = 99
	}
	out := []map[string]any{}
	next, more := after, false
	history, ok := s.history[slug]
	if !ok {
		history = s.common
	}
	for _, m := range history {
		if m.Seq <= after {
			continue
		}
		if len(out) == limit {
			more = true
			break
		}
		out = append(out, wire(m))
		next = m.Seq
	}
	return map[string]any{
		"ok": true, "messages": out, "has_more": more,
		"response_metadata": map[string]any{"next_cursor": strconv.FormatInt(next, 10)},
	}
}

//craft:ignore naked-any the answer is one history message's JSON
func wire(m Message) map[string]any {
	out := map[string]any{
		fieldType: fieldMessage, "ts": TS(m.Seq), fieldText: m.Text,
		"posted_at": m.PostedAt.UTC().Format(time.RFC3339),
	}
	if m.Bot {
		out[fieldBotID] = botID
	} else {
		out["user"], out["user_name"] = "user-"+m.UserName, m.UserName
		if m.UserEmail != "" {
			out["user_email"] = m.UserEmail
		}
	}
	if m.Subtype != "" {
		out["subtype"] = m.Subtype
	}
	switch {
	case m.ThreadSeq != 0:
		out["thread_ts"] = TS(m.ThreadSeq)
	case m.ReplyCount > 0:
		out["thread_ts"], out["reply_count"] = TS(m.Seq), m.ReplyCount
	}
	return out
}

// reply answers 200, as Dispact does for a refusal too.
//
//craft:ignore naked-any answer is a route's JSON
func reply(w http.ResponseWriter, answer map[string]any) {
	httperr.WriteJSON(w, http.StatusOK, answer)
}
