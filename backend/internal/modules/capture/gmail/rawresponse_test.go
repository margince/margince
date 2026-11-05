// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestDeclaredRawLengthReleasesRoomForTheNextFetch(t *testing.T) {
	secondStarted := make(chan struct{})
	body := fmt.Sprintf(`{"raw":%q}`, base64.RawURLEncoding.EncodeToString([]byte("mail")))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flushing headers: %v", err)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/first") {
			select {
			case <-secondStarted:
			case <-r.Context().Done():
				return
			}
		} else {
			close(secondStarted)
		}
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("writing message: %v", err)
		}
	}))
	defer server.Close()
	api := NewAPI(server.Client(), server.URL)
	budget := semaphore.NewWeighted(rawFetchReservation + 4096)
	ids := []string{"first", "second"}
	err := inOrder(t.Context(), len(ids), func(ctx context.Context, i int) (Message, error) {
		return api.GetRaw(ctx, "test", ids[i])
	}, func(i int, msg Message, err error) error {
		if err != nil {
			return err
		}
		if string(msg.RFC822) != "mail" {
			return fmt.Errorf("message %d changed", i)
		}
		return nil
	}, budget, rawFetchReservation)
	if err != nil {
		t.Fatal(err)
	}
	if !budget.TryAcquire(rawFetchReservation + 4096) {
		t.Fatal("completed HTTP walk leaked bytes")
	}
	budget.Release(rawFetchReservation + 4096)
}

func TestJSONResponseCannotHideOversizeBehindAValidPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, `{}`+strings.Repeat(" ", 64)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	var out json.RawMessage
	if _, err := api.getOnce(t.Context(), "test", "/messages/test", nil, &out, 2); err == nil {
		t.Fatal("response beyond the byte cap was accepted")
	}
}

func TestRawLabelDecodingBoundsArrayMemoryBeforeAllocation(t *testing.T) {
	for _, count := range []int{maxMessageLabels, maxMessageLabels + 1} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			var labels messageLabels
			body := `[` + strings.Repeat(`"",`, count-1) + `""]`
			err := json.Unmarshal([]byte(body), &labels)
			if count == maxMessageLabels {
				if err != nil || len(labels) != count {
					t.Fatalf("labels=%d err=%v want=%d", len(labels), err, count)
				}
			} else if err == nil || len(labels) != 0 {
				t.Fatalf("oversize array allocated %d labels: %v", len(labels), err)
			}
		})
	}
}

func TestConcurrentWalkersRespectTheProductionRawBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	started, release := make(chan struct{}, 2), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flushing response: %v", err)
			return
		}
		started <- struct{}{}
		select {
		case <-release:
			if _, err := fmt.Fprint(w, `{"raw":"bWFpbA"}`); err != nil {
				t.Errorf("writing raw mail: %v", err)
			}
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	api := NewAPI(server.Client(), server.URL)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- inOrder(ctx, 1, func(ctx context.Context, _ int) (Message, error) {
				return api.GetRaw(ctx, "test", "mail")
			}, func(_ int, msg Message, err error) error {
				if err != nil {
					return err
				}
				if string(msg.RFC822) != "mail" {
					return errors.New("changed message")
				}
				return nil
			}, processRawMemory, rawFetchReservation)
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("production budget serialized two chunked downloads")
		}
	}
	if processRawMemory.TryAcquire(rawFetchReservation) {
		processRawMemory.Release(rawFetchReservation)
		t.Error("two walkers failed to charge the shared budget")
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if !processRawMemory.TryAcquire(rawProcessBudget) {
		t.Fatal("walkers leaked the shared budget")
	}
	processRawMemory.Release(rawProcessBudget)
}

type rawResponseTransport func(*http.Request) (*http.Response, error)

func (transport rawResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestRawResponsesEnforceDeclaredLengthAndEncoding(t *testing.T) {
	body := `{"raw":"bWFpbA"}`
	for _, tc := range []struct {
		name    string
		length  int64
		body    string
		invalid bool
	}{
		{"unknown", -1, body, false},
		{"exact", int64(len(body)), body, false},
		{"longer body", int64(len(body) - 1), body, true},
		{"shorter body", int64(len(body) + 1), body, true},
		{"invalid UTF8 label", -1, "{\"raw\":\"bWFpbA\",\"labelIds\":[\"\xff\"]}", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: rawResponseTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: tc.length, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			msg, err := NewAPI(client, "https://gmail.example").GetRaw(t.Context(), "test", "mail")
			if tc.invalid {
				if !errors.Is(err, ErrUnreachable) {
					t.Fatalf("invalid response error=%v", err)
				}
			} else if err != nil || string(msg.RFC822) != "mail" {
				t.Fatalf("raw=%q err=%v", msg.RFC822, err)
			}
		})
	}
}

func TestGzipRawResponsesUseTheBoundedReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		writer := gzip.NewWriter(w)
		if _, err := io.WriteString(writer, `{"raw":"bWFpbA"}`); err != nil {
			t.Errorf("compressing message: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Errorf("closing compressed response: %v", err)
		}
	}))
	defer server.Close()
	msg, err := NewAPI(server.Client(), server.URL).GetRaw(t.Context(), "test", "mail")
	if err != nil || string(msg.RFC822) != "mail" {
		t.Fatalf("gzip raw=%q err=%v", msg.RFC822, err)
	}
}

func TestExcessiveMetadataFallsBackToTheFullMessage(t *testing.T) {
	headers := `[` + strings.Repeat(`{"name":"X","value":"v"},`, maxMessageLabels) + `{"name":"X","value":"v"}]`
	raw := base64.RawURLEncoding.EncodeToString(rawMsg("fallback@example.com", "alice@acme.com"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.URL.Query().Get("format") {
		case "metadata":
			body = `{"payload":{"headers":` + headers + `}}`
		case "RAW":
			body = fmt.Sprintf(`{"raw":%q}`, raw)
		default:
			body = `{"messages":[{"id":"mail"}]}`
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()
	api := NewAPI(server.Client(), server.URL)
	c := New(fakeOAuth{access: "test"}, api)
	sink := &recordingSink{}
	result, err := c.BackfillPage(t.Context(), authBytes(t), time.Time{}, "", sink)
	if err != nil || result.Captured != 1 || len(sink.recs) != 1 {
		t.Fatalf("fallback=%+v captured=%d err=%v", result, len(sink.recs), err)
	}
	if !processRawMemory.TryAcquire(rawProcessBudget) {
		t.Fatal("metadata fallback leaked reservations")
	}
	processRawMemory.Release(rawProcessBudget)
	headersAPI, ok := api.(HeaderFetcher)
	if !ok {
		t.Fatal("HTTP API cannot fetch headers")
	}
	if _, err := headersAPI.GetHeaders(t.Context(), "test", "mail"); !errors.Is(err, errMetadataNeedsRaw) || errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("metadata fallback classified as a page fault: %v", err)
	}
}
