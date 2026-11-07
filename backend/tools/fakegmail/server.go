// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const apiPrefix = "/gmail/v1/users/me"

type server struct {
	boxes     []mailbox
	byOwner   map[string]int
	mu        sync.Mutex
	connected map[string]bool
	calls     sync.Map
	rawBytes  atomic.Int64
	now       time.Time
	big       bool
}

func (s *server) routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /o/oauth2/v2/auth", s.consent)
	m.HandleFunc("POST /token", s.token)
	m.HandleFunc("GET "+apiPrefix+"/profile", s.profile)
	m.HandleFunc("GET "+apiPrefix+"/messages", s.list)
	m.HandleFunc("GET "+apiPrefix+"/messages/{id}", s.message)
	m.HandleFunc("GET "+apiPrefix+"/history", func(w http.ResponseWriter, _ *http.Request) {
		s.count("history")
		writeJSON(w, map[string]any{"historyId": "100"})
	})
	m.HandleFunc("GET "+apiPrefix+"/labels", s.labels)
	m.HandleFunc("POST "+apiPrefix+"/watch", func(w http.ResponseWriter, _ *http.Request) {
		s.count("watch")
		writeJSON(w, map[string]any{"historyId": "100", "expiration": strconv.FormatInt(time.Now().Add(7*24*time.Hour).UnixMilli(), 10)})
	})
	m.HandleFunc("GET /stats", s.stats)
	return m
}

func (s *server) count(op string) {
	counter, _ := s.calls.LoadOrStore(op, new(atomic.Int64))
	if c, ok := counter.(*atomic.Int64); ok {
		c.Add(1)
	}
}

// writeError answers in Google's error shape, which the connector reads.
func writeError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": status, "status": reason}}); err != nil {
		log.Printf("fakegmail: writing an error: %v", err)
	}
}

//craft:ignore naked-any each endpoint answers its own Google wire shape, built as a map
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("fakegmail: writing a response: %v", err)
	}
}

// consent stands in for Google's consent screen: it approves at once and sends
// the browser back with a code naming the mailbox. login_hint picks the
// mailbox; without it the first mailbox no code was exchanged for yet, so a
// reloaded consent page offers the same mailbox again.
func (s *server) consent(w http.ResponseWriter, r *http.Request) {
	s.count("auth")
	q := r.URL.Query()
	owner := q.Get("login_hint")
	if owner == "" {
		owner = s.nextUnconnected()
	}
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "redirect_uri is not a URL")
		return
	}
	v := back.Query()
	v.Set("code", "code-"+owner)
	v.Set("state", q.Get("state"))
	v.Set("scope", q.Get("scope"))
	back.RawQuery = v.Encode()
	//nolint:gosec // G710: sending the browser back to the caller's redirect_uri is what a consent screen does; this one serves local load tests only
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// token answers both the code exchange and a refresh. The tokens name the
// mailbox, and the granted scope is whatever was asked for.
func (s *server) token(w http.ResponseWriter, r *http.Request) {
	s.count("token")
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "the form could not be read")
		return
	}
	owner := strings.TrimPrefix(r.PostForm.Get("code"), "code-")
	if owner != "" {
		s.mu.Lock()
		s.connected[owner] = true
		s.mu.Unlock()
	}
	if refresh := r.PostForm.Get("refresh_token"); refresh != "" {
		owner = strings.TrimPrefix(refresh, "rt-")
	}
	scope := r.PostForm.Get("scope")
	if scope == "" {
		scope = "https://www.googleapis.com/auth/gmail.readonly"
	}
	writeJSON(w, map[string]any{"access_token": "at-" + owner, "refresh_token": "rt-" + owner, "expires_in": 3600, "token_type": "Bearer", "scope": scope})
}

func (s *server) nextUnconnected() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, box := range s.boxes {
		if !s.connected[box.owner] {
			return box.owner
		}
	}
	return s.boxes[0].owner
}

func (s *server) mailboxOf(w http.ResponseWriter, r *http.Request) (int, bool) {
	box, ok := s.byOwner[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer at-")]
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED")
	}
	return box, ok
}

func (s *server) profile(w http.ResponseWriter, r *http.Request) {
	s.count("profile")
	if box, ok := s.mailboxOf(w, r); ok {
		writeJSON(w, map[string]any{"emailAddress": s.boxes[box].owner, "historyId": "100", "messagesTotal": s.boxes[box].count})
	}
}

// list pages through the mailbox newest first. The search query is ignored:
// every message is inside any window an import asks for.
func (s *server) list(w http.ResponseWriter, r *http.Request) {
	s.count("list")
	box, ok := s.mailboxOf(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	size, err := strconv.Atoi(q.Get("maxResults"))
	if err != nil || size <= 0 || size > 500 {
		size = 100
	}
	start, err := strconv.Atoi(q.Get("pageToken"))
	if err != nil {
		start = 0
	}
	end := min(start+size, s.boxes[box].count)
	ids := make([]map[string]string, 0, max(0, end-start))
	for n := start; n < end; n++ {
		ids = append(ids, map[string]string{"id": messageID(box, n)})
	}
	out := map[string]any{"messages": ids, "resultSizeEstimate": s.boxes[box].count}
	if end < s.boxes[box].count {
		out["nextPageToken"] = strconv.Itoa(end)
	}
	writeJSON(w, out)
}

func (s *server) message(w http.ResponseWriter, r *http.Request) {
	holder, authorized := s.mailboxOf(w, r)
	if !authorized {
		return
	}
	box, n, ok := s.parseID(r.PathValue("id"))
	if !ok || box != holder {
		http.NotFound(w, r)
		return
	}
	msg := s.build(box, n)
	out := map[string]any{"id": msg.id, "threadId": msg.id, "labelIds": msg.labels, "internalDate": strconv.FormatInt(msg.date.UnixMilli(), 10)}
	if strings.EqualFold(r.URL.Query().Get("format"), "metadata") {
		s.count("get_metadata")
		headers := make([]map[string]string, 0, len(msg.headers))
		for _, h := range msg.headers {
			headers = append(headers, map[string]string{"name": h[0], "value": h[1]})
		}
		out["payload"] = map[string]any{"headers": headers}
	} else {
		s.count("get_raw")
		out["raw"] = base64.URLEncoding.EncodeToString([]byte(msg.raw))
		s.rawBytes.Add(int64(len(msg.raw)))
	}
	writeJSON(w, out)
}

func (s *server) parseID(id string) (box, n int, ok bool) {
	if len(id) != 7 {
		return 0, 0, false
	}
	b, errBox := strconv.ParseInt(id[:1], 16, 32)
	m, errN := strconv.ParseInt(id[1:], 16, 32)
	if errBox != nil || errN != nil || b < 1 || int(b) > len(s.boxes) || int(m) >= s.boxes[b-1].count {
		return 0, 0, false
	}
	return int(b - 1), int(m), true
}

func (s *server) labels(w http.ResponseWriter, _ *http.Request) {
	s.count("labels")
	var labels []map[string]string
	for _, id := range []string{"INBOX", "SENT", "DRAFT", "SPAM", "TRASH", "CATEGORY_PROMOTIONS"} {
		labels = append(labels, map[string]string{"id": id, "name": id, "type": "system"})
	}
	writeJSON(w, map[string]any{"labels": labels})
}

// stats reports the calls served by kind and the raw bytes sent, which is how
// a load run reads its own progress.
func (s *server) stats(w http.ResponseWriter, _ *http.Request) {
	out := map[string]int64{"raw_bytes": s.rawBytes.Load()}
	s.calls.Range(func(key, value any) bool {
		name, okName := key.(string)
		counter, okCounter := value.(*atomic.Int64)
		if okName && okCounter {
			out[name] = counter.Load()
		}
		return true
	})
	writeJSON(w, out)
}
