// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// What a subscription round asks Microsoft for, and what it refuses to report.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The deadline is bounded by Microsoft's ceiling for a message resource — just
// under three days. Asking for more is refused outright rather than clamped, so
// a round that asked for a week would register nothing at all.
func TestASubscriptionAsksForADeadlineMicrosoftWillAccept(t *testing.T) {
	api := &fakeAPI{email: owner}
	c := pinnedConn(api)

	res, err := c.Watch(context.Background(), authBytes(t), "https://api.example/webhooks/graph?token=t")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if api.subCalls != 1 {
		t.Fatalf("%d subscription calls, want exactly one", api.subCalls)
	}
	want := pinned.Add(maxSubscriptionMinutes * time.Minute).UTC()
	if !api.subDeadline.Equal(want) {
		t.Errorf("asked for %v, want %v", api.subDeadline, want)
	}
	if maxSubscriptionMinutes >= 4230 {
		t.Errorf("the requested window is %d minutes; Microsoft's ceiling for a message resource is 4230 and it refuses rather than clamping", maxSubscriptionMinutes)
	}
	if !res.ExpiresAt.Equal(want) {
		t.Errorf("reported %v, want the deadline Microsoft honored", res.ExpiresAt)
	}
	// The registry stores this and the renewal scan keys on it; a history
	// anchor would suppress the first sync's backfill, and Graph has none to
	// give anyway.
	if res.HistoryID != "" {
		t.Errorf("HistoryID = %q, want none — a Graph subscription says nothing about the delta cursor", res.HistoryID)
	}
}

// The notification carries no mailbox of its own: its `resource` names a
// directory object id this system never stored. clientState is what the webhook
// routes on, so a round that failed to send it would register a subscription
// whose every notification is unroutable.
func TestASubscriptionCarriesTheOwnerSoTheWebhookCanRouteIt(t *testing.T) {
	const url = "https://api.example/webhooks/graph?token=t"
	api := &fakeAPI{email: owner}
	c := pinnedConn(api)

	if _, err := c.Watch(context.Background(), authBytes(t), url); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if api.subState != owner {
		t.Errorf("clientState = %q, want the mailbox owner %q", api.subState, owner)
	}
	if api.subURL != url {
		t.Errorf("notificationUrl = %q, want the URL it was given verbatim — the token rides in it", api.subURL)
	}
}

// A subscription Microsoft accepted without saying when it lapses is refused
// rather than defaulted: a guessed deadline is one the renewal sweep keys on,
// and guessing long is a mailbox that goes quiet with nothing to notice it.
func TestASubscriptionWithNoDeadlineIsRefusedRatherThanGuessed(t *testing.T) {
	api := &fakeAPI{email: owner, subResult: Subscription{ID: "sub-1"}}
	c := pinnedConn(api)

	if _, err := c.Watch(context.Background(), authBytes(t), "https://api.example/webhooks/graph"); !errors.Is(err, errNoSubscriptionDeadline) {
		t.Fatalf("Watch = %v, want the missing deadline refused", err)
	}
}

// A provider fault stops the round without a deadline, so the registry advances
// nothing and the next sweep retries from the same state.
func TestAFailedSubscriptionAdvancesNothing(t *testing.T) {
	api := &fakeAPI{email: owner, subErr: ErrUnreachable}
	c := pinnedConn(api)

	res, err := c.Watch(context.Background(), authBytes(t), "https://api.example/webhooks/graph")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("Watch = %v, want the provider fault", err)
	}
	if !res.ExpiresAt.IsZero() {
		t.Errorf("a failed round reported a deadline of %v", res.ExpiresAt)
	}
}

// Renew the one that is already there, then create only if there is none.
// Microsoft accepts several subscriptions on one resource, so a create-first
// round would leave a mailbox with one more every cycle, each delivering the
// same notification.
func TestARoundRenewsTheSubscriptionItAlreadyHasRatherThanAddingOne(t *testing.T) {
	var created, renewed int
	srv := subscriptionStub(t, subscriptionStubState{
		existing: []subscription{{
			ID: "sub-1", Resource: messagesResource,
			NotificationURL: "https://api.example/webhooks/graph?token=t",
		}},
		onCreate: func() { created++ },
		onRenew:  func() { renewed++ },
	})
	api := NewAPI(srv.Client(), srv.URL)

	sub, err := api.EnsureSubscription(context.Background(), "at",
		"https://api.example/webhooks/graph?token=t", owner, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("EnsureSubscription: %v", err)
	}
	if sub.ID != "sub-1" {
		t.Errorf("settled on %q, want the subscription that was already there", sub.ID)
	}
	if renewed != 1 || created != 0 {
		t.Errorf("renewed %d, created %d — a second subscription delivers the same notification twice", renewed, created)
	}
}

// A URL that is not ours is somebody else's subscription, including one left by
// a rotated operator token: it is replaced, not extended.
func TestASubscriptionForADifferentURLIsNotAdopted(t *testing.T) {
	var created, renewed int
	srv := subscriptionStub(t, subscriptionStubState{
		existing: []subscription{{
			ID: "sub-old", Resource: messagesResource,
			NotificationURL: "https://api.example/webhooks/graph?token=ROTATED",
		}},
		onCreate: func() { created++ },
		onRenew:  func() { renewed++ },
	})
	api := NewAPI(srv.Client(), srv.URL)

	if _, err := api.EnsureSubscription(context.Background(), "at",
		"https://api.example/webhooks/graph?token=t", owner, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("EnsureSubscription: %v", err)
	}
	if created != 1 || renewed != 0 {
		t.Errorf("renewed %d, created %d — a subscription pointing at a stale token was extended", renewed, created)
	}
}

// Microsoft drops a subscription whose endpoint failed too often, and a renewal
// then 404s. The recovery is a new subscription, not a failed round: otherwise
// a mailbox that went briefly unreachable never gets push back.
func TestARenewalOfAVanishedSubscriptionCreatesANewOne(t *testing.T) {
	var created int
	srv := subscriptionStub(t, subscriptionStubState{
		existing: []subscription{{
			ID: "sub-gone", Resource: messagesResource,
			NotificationURL: "https://api.example/webhooks/graph?token=t",
		}},
		renewStatus: http.StatusNotFound,
		onCreate:    func() { created++ },
	})
	api := NewAPI(srv.Client(), srv.URL)

	sub, err := api.EnsureSubscription(context.Background(), "at",
		"https://api.example/webhooks/graph?token=t", owner, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("EnsureSubscription: %v", err)
	}
	if created != 1 || sub.ID == "sub-gone" {
		t.Errorf("created %d and settled on %q — a vanished subscription left the mailbox with no push", created, sub.ID)
	}
}

// subscriptionStubState is what one stub round answers with.
type subscriptionStubState struct {
	existing    []subscription
	renewStatus int
	onCreate    func()
	onRenew     func()
}

// subscriptionStub answers the three calls a round can make.
func subscriptionStub(t *testing.T, st subscriptionStubState) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if st.onCreate != nil {
				st.onCreate()
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]any{
				"id": "sub-new", "expirationDateTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
			return
		}
		writeJSON(w, map[string]any{"value": st.existing})
	})
	mux.HandleFunc("/subscriptions/", func(w http.ResponseWriter, _ *http.Request) {
		if st.onRenew != nil {
			st.onRenew()
		}
		if st.renewStatus != 0 {
			w.WriteHeader(st.renewStatus)
			return
		}
		writeJSON(w, map[string]any{
			"id": "sub-1", "expirationDateTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// The seam is asserted at compile time in graph.go; this pins what the registry
// reads back off it.
var _ connector.Watcher = (*Connector)(nil)

// The registry stores this deadline and the renewal scan selects on it, so a
// value further out than Microsoft can legitimately grant is a connection the
// scan never picks up again — while the real subscription lapses within three
// days and the mailbox quietly falls back to the poll.
func TestADeadlineBeyondWhatWasAskedForIsClamped(t *testing.T) {
	api := &fakeAPI{email: owner, subResult: Subscription{
		ID: "sub-1", Expiration: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
	}}
	c := pinnedConn(api)

	res, err := c.Watch(context.Background(), authBytes(t), "https://api.example/webhooks/graph")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	want := pinned.Add(maxSubscriptionMinutes * time.Minute).UTC()
	if !res.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want it clamped to the %v that was asked for", res.ExpiresAt, want)
	}
}

// Our subscription may not be on the first page. Falling through to create is
// what would add one more every renewal cycle, each delivering the same
// notification — the accumulation renew-then-create exists to prevent.
func TestASubscriptionIsFoundOnAPageAfterTheFirst(t *testing.T) {
	var created, renewed int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			created++
			writeJSON(w, map[string]any{"id": "sub-new"})
			return
		}
		if r.URL.Query().Get("$skiptoken") == "p2" {
			writeJSON(w, map[string]any{"value": []subscription{{
				ID: "sub-ours", Resource: messagesResource,
				NotificationURL: "https://api.example/webhooks/graph?token=t",
			}}})
			return
		}
		writeJSON(w, map[string]any{
			"value":           []subscription{{ID: "someone-else", Resource: "/me/events"}},
			"@odata.nextLink": srv.URL + "/subscriptions?%24skiptoken=p2",
		})
	})
	mux.HandleFunc("/subscriptions/", func(w http.ResponseWriter, _ *http.Request) {
		renewed++
		writeJSON(w, map[string]any{
			"id": "sub-ours", "expirationDateTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})

	sub, err := NewAPI(srv.Client(), srv.URL).EnsureSubscription(context.Background(), "at",
		"https://api.example/webhooks/graph?token=t", owner, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("EnsureSubscription: %v", err)
	}
	if created != 0 || renewed != 1 || sub.ID != "sub-ours" {
		t.Errorf("created %d, renewed %d, settled on %q — a second page was not walked", created, renewed, sub.ID)
	}
}

// A renewal aims an authenticated PATCH — carrying the person's own delegated
// token — at a path built from a provider-supplied id. An id carrying a path
// segment must not redirect it at another resource.
func TestARenewalCannotBeAimedByAProviderSuppliedID(t *testing.T) {
	var patched string
	mux := http.NewServeMux()
	mux.HandleFunc("/subscriptions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"value": []subscription{{
			ID: "../../me/messages", Resource: messagesResource,
			NotificationURL: "https://api.example/webhooks/graph?token=t",
		}}})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		patched = r.URL.Path
		writeJSON(w, map[string]any{
			"id": "sub-1", "expirationDateTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if _, err := NewAPI(srv.Client(), srv.URL).EnsureSubscription(context.Background(), "at",
		"https://api.example/webhooks/graph?token=t", owner, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("EnsureSubscription: %v", err)
	}
	if !strings.HasPrefix(patched, subscriptionsPath+"/") {
		t.Errorf("the renewal reached %q — a provider-supplied id steered it out of the subscription collection", patched)
	}
}

// clientState is what the webhook routes on. A bundle with no owner would
// create a subscription Microsoft echoes nothing for, so every notification it
// ever delivers is dropped as unroutable — a mailbox silently back on the poll
// behind a subscription that looks healthy.
func TestASubscriptionIsNotCreatedForACredentialNamingNoMailbox(t *testing.T) {
	api := &fakeAPI{email: owner}
	c := pinnedConn(api)

	auth, err := graphconn.Seal(connectorName, graphconn.AuthState{RefreshToken: "rt"})
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := c.Watch(context.Background(), auth, "https://api.example/webhooks/graph"); !errors.Is(err, errNoSubscriptionOwner) {
		t.Fatalf("Watch = %v, want the ownerless credential refused", err)
	}
	if api.subCalls != 0 {
		t.Errorf("%d subscription(s) created for a credential that could not be routed", api.subCalls)
	}
}
