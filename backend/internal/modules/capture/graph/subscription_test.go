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
	"sync"
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
// a rotated operator token: a new one is created beside it rather than the old
// one being re-pointed.
//
// The old one is NOT deleted — nothing here can prove it is ours to delete —
// so a rotation does leave a duplicate delivering to an endpoint that now
// refuses it. That is bounded and self-healing: exactly one, because the next
// round matches the new URL exactly, and Microsoft drops a subscription whose
// endpoint keeps failing.
//
// The alternative is matching on the resource and re-pointing whatever watches
// `/me/messages`. It is rejected deliberately. Graph lists subscriptions per
// APP, so two deployments sharing one Entra registration — a staging and a
// production, the ordinary case — see each other's, and re-pointing would take
// the other's notifications and give back nothing. The token in the URL is what
// tells those two apart, which is the whole reason it is there. A bounded
// duplicate beats an unbounded hijack.
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
	// Guarded for the reason oneSubscriptionStub gives below: the write happens
	// on the server's goroutine and the read on this one.
	var (
		mu      sync.Mutex
		patched string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/subscriptions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"value": []subscription{{
			ID: "../../me/messages", Resource: messagesResource,
			NotificationURL: "https://api.example/webhooks/graph?token=t",
		}}})
	})
	// The RAW path, as it went over the wire. r.URL.Path is the decoded view,
	// which is the wrong thing to assert on twice over: it would read
	// `/subscriptions/../../me/messages` as though the escaping had failed, and
	// it hides that the bytes sent never left the collection.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		patched = r.URL.EscapedPath()
		mu.Unlock()
		writeJSON(w, map[string]any{
			"id": "sub-1", "expirationDateTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// The round SUCCEEDS: refusing the id is not refusing the mailbox. It falls
	// through to creating a subscription, which is the safe direction — a
	// duplicate notification costs one redundant sync, where declining to renew
	// costs the mailbox its push.
	if _, err := NewAPI(srv.Client(), srv.URL).EnsureSubscription(context.Background(), "at",
		"https://api.example/webhooks/graph?token=t", owner, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("EnsureSubscription: %v", err)
	}
	// Nothing was PATCHed at all: the id is refused before a request is built,
	// so this does not rest on how the far end normalizes what it received.
	mu.Lock()
	sent := patched
	mu.Unlock()
	if sent != "" {
		t.Errorf("a renewal was sent to %q for a provider-supplied id that is not a subscription id", sent)
	}
}

// The refusal itself, at the seam, so the case above cannot pass by accident of
// routing. Every spelling here is one a server that decodes before resolving
// would walk out of the subscription collection on, whatever url.PathEscape did
// to the separators.
func TestOnlyASubscriptionIDCanAddressARenewal(t *testing.T) {
	for _, id := range []string{
		"", "../../me/messages", "..", "a/b", `a\b`, "sub 1", "sub?x=1", "sub#f", "%2e%2e",
	} {
		if isSubscriptionID(id) {
			t.Errorf("%q was accepted as a subscription id", id)
		}
	}
	// And a real one still is, or the refusal above would be refusing every
	// renewal this client ever makes.
	if !isSubscriptionID("0fc1b2a3-3d4e-5f60-8a1b-2c3d4e5f6071") {
		t.Error("a GUID Microsoft would mint was refused as a subscription id")
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

// A RENEWAL ADDRESSES THE SUBSCRIPTION IT STORED, and does not go looking.
//
// Watch has to make sure a subscription exists, and without a handle that means
// walking GET /subscriptions — paged, once per mailbox, every cycle — to find
// the one this installation made. The handle turns that into one call.
// testNotifyURL is the endpoint every subscription test registers against, so a
// fake reading a subscription back can report "nothing has moved" by default.
const testNotifyURL = "https://api.example/webhooks/graph?token=t"

func TestARenewalExtendsTheStoredSubscriptionWithoutListingAnything(t *testing.T) {
	api := &fakeAPI{email: owner}
	c := pinnedConn(api)

	res, err := c.RenewWatch(context.Background(), authBytes(t),
		testNotifyURL, "sub-stored")
	if err != nil {
		t.Fatalf("RenewWatch: %v", err)
	}
	if api.renewCalls != 1 || api.renewID != "sub-stored" {
		t.Fatalf("renewed %d time(s) naming %q, want one naming the stored id", api.renewCalls, api.renewID)
	}
	if api.subCalls != 0 {
		t.Errorf("%d EnsureSubscription call(s) — a renewal that knows the id paid for the listing "+
			"anyway, which is the round trip per mailbox per cycle this exists to save", api.subCalls)
	}
	want := pinned.Add(maxSubscriptionMinutes * time.Minute).UTC()
	if !res.ExpiresAt.Equal(want) {
		t.Errorf("reported %v, want the deadline Microsoft honored", res.ExpiresAt)
	}
	if res.Ref != "sub-stored" {
		t.Errorf("Ref = %q, want the id that was renewed — the registry stores this and the next "+
			"renewal addresses it", res.Ref)
	}
}

// AND A HANDLE MICROSOFT NO LONGER KNOWS falls back to the listing, which is
// where a recovery path belongs.
//
// A subscription dropped for repeated delivery failures, or one made by an
// installation since restored from a backup, is exactly the case Ensure's
// renew-then-create answers. Reporting the error instead would leave the mailbox
// on the poll for as long as the stored id kept failing.
func TestARenewalWhoseSubscriptionIsGoneRegistersAFreshOne(t *testing.T) {
	api := &fakeAPI{email: owner, renewErr: ErrSubscriptionGone}
	c := pinnedConn(api)

	res, err := c.RenewWatch(context.Background(), authBytes(t),
		testNotifyURL, "sub-vanished")
	if err != nil {
		t.Fatalf("RenewWatch on a subscription Microsoft has dropped: %v — the mailbox would stay "+
			"on the poll for as long as the stored id kept failing", err)
	}
	if api.subCalls != 1 {
		t.Errorf("%d EnsureSubscription call(s), want one — the fallback IS the listing", api.subCalls)
	}
	if res.Ref == "" || res.Ref == "sub-vanished" {
		t.Errorf("Ref = %q, want the id of the subscription that now exists", res.Ref)
	}
}

// AND A HANDLE NAMING A SUBSCRIPTION THAT POINTS ELSEWHERE is not renewed.
//
// A renewal extends a deadline; it never re-states where the subscription
// points. Before a handle existed, every renewal went through
// EnsureSubscription, which looks its subscription up BY notificationURL — so a
// deployment whose webhook URL had moved got a fresh subscription on the new
// endpoint without anyone arranging it. Extending the stored one instead would
// keep Microsoft delivering to an endpoint nobody serves, and leave the new
// webhook poll-only with nothing failing to say so.
func TestARenewalWhoseEndpointMovedRegistersAtTheNewOne(t *testing.T) {
	api := &fakeAPI{email: owner, getSubURL: "https://api.example/webhooks/graph?token=OLD"}
	c := pinnedConn(api)

	res, err := c.RenewWatch(context.Background(), authBytes(t), testNotifyURL, "sub-at-old-endpoint")
	if err != nil {
		t.Fatalf("RenewWatch: %v", err)
	}
	if api.renewCalls != 0 {
		t.Errorf("renewed %d time(s) naming %q — extending a subscription registered against the "+
			"previous endpoint keeps notifications going where nobody is listening", api.renewCalls, api.renewID)
	}
	if api.subCalls != 1 {
		t.Errorf("%d EnsureSubscription call(s), want one: it is the call that looks a subscription "+
			"up BY the endpoint, and registers when none points there", api.subCalls)
	}
	if res.Ref == "" {
		t.Error("Ref is empty — the freshly registered subscription's id is what the next renewal " +
			"addresses, and without it every cycle repeats the listing")
	}
}

// A RENEWAL READS THE SUBSCRIPTION BEFORE IT EXTENDS ONE, and reads it from
// MICROSOFT rather than from anything stored beside the handle: the endpoint
// carries the token that admits a notification, and watch_ref is an ordinary
// column.
func TestARenewalReadsTheSubscriptionBeforeExtendingIt(t *testing.T) {
	api := &fakeAPI{email: owner}
	c := pinnedConn(api)

	if _, err := c.RenewWatch(context.Background(), authBytes(t), testNotifyURL, "sub-stored"); err != nil {
		t.Fatalf("RenewWatch: %v", err)
	}
	if api.getSubCalls != 1 || api.getSubID != "sub-stored" {
		t.Errorf("read the subscription %d time(s) naming %q, want one naming the stored id — "+
			"without it a renewal cannot tell that the endpoint has not moved", api.getSubCalls, api.getSubID)
	}
}

// AND A REF THAT CANNOT NAME A SUBSCRIPTION falls back rather than failing,
// which is what makes a first registration — and a connection stored before
// handles existed — cost one listing and no incident.
func TestARefThatNamesNoSubscriptionFallsBackRatherThanFailing(t *testing.T) {
	for name, stored := range map[string]string{
		"empty":        "",
		"a path step":  "..",
		"a path":       "../../me/messages",
		"needs escape": "sub 1/../x",
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{email: owner}
			c := pinnedConn(api)
			if _, err := c.RenewWatch(context.Background(), authBytes(t), testNotifyURL, stored); err != nil {
				t.Fatalf("RenewWatch: %v", err)
			}
			if api.getSubCalls != 0 {
				t.Errorf("read a subscription %d time(s) for a ref that names none", api.getSubCalls)
			}
			if api.renewCalls != 0 || api.subCalls != 1 {
				t.Errorf("renewed %d and ensured %d, want 0 and 1 — a ref that names no "+
					"subscription answers nothing a renewal is asking",
					api.renewCalls, api.subCalls)
			}
		})
	}
}

// THE STORED HANDLE CARRIES NOTHING BUT THE SUBSCRIPTION ID.
//
// Microsoft signs nothing on a change notification, so the operator token in the
// notification URL is the only thing admitting one. watch_ref reaches every
// database reader and every backup, and a renewal has no need to remember the
// endpoint there: it asks Microsoft.
func TestAStoredHandleCarriesNothingButTheID(t *testing.T) {
	const url = "https://api.example/webhooks/graph?token=OPERATOR-SECRET"
	api := &fakeAPI{email: owner, subResult: Subscription{ID: "sub-1", Expiration: pinned.Add(time.Hour)}}
	c := pinnedConn(api)

	res, err := c.Watch(context.Background(), authBytes(t), url)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if res.Ref != "sub-1" {
		t.Fatalf("Ref = %q, want the subscription id", res.Ref)
	}
	for _, leak := range []string{"OPERATOR-SECRET", "api.example", "token=", "https"} {
		if strings.Contains(res.Ref, leak) {
			t.Errorf("the stored handle %q carries %q — that column is not a place for the "+
				"factor that admits a notification, nor for anything an offline guess could "+
				"be checked against", res.Ref, leak)
		}
	}
}

// A FALLBACK REFRESHES THE TOKEN ONCE, not twice: Watch refreshes one of its
// own, and the fallback is the ordinary path for a first registration rather
// than a rare one.
func TestAFallbackDoesNotRefreshTheTokenTwice(t *testing.T) {
	api := &fakeAPI{email: owner}
	oauth := &countingOAuth{OAuth: &fakeOAuth{access: "access-1"}}
	c := New(oauth, api)
	c.now = func() time.Time { return pinned }

	if _, err := c.RenewWatch(context.Background(), authBytes(t),
		testNotifyURL, ""); err != nil {
		t.Fatalf("RenewWatch: %v", err)
	}
	if oauth.calls != 1 {
		t.Errorf("refreshed the access token %d time(s), want one — a first registration is the "+
			"ordinary case here, so a doubled refresh is doubled token traffic on every mailbox",
			oauth.calls)
	}
}

// countingOAuth counts access-token refreshes and does nothing else.
type countingOAuth struct {
	OAuth
	calls int
}

func (o *countingOAuth) AccessToken(ctx context.Context, refresh string) (string, error) {
	o.calls++
	return o.OAuth.AccessToken(ctx, refresh)
}

// oneSubscriptionStub answers GET /subscriptions/{id} with the given status and
// body, and returns a reader for the raw path the request took.
//
// Through a mutex rather than a bare pointer: the handler runs on the server's
// own goroutine, and nothing in net/http promises the test goroutine an edge to
// read across afterwards, so -race is entitled to call the plain version a data
// race whatever it does in practice.
func oneSubscriptionStub(t *testing.T, status int, body map[string]any) (*httptest.Server, func() string) {
	t.Helper()
	var (
		mu   sync.Mutex
		path string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path = r.URL.EscapedPath()
		mu.Unlock()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		writeJSON(w, body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() string {
		mu.Lock()
		defer mu.Unlock()
		return path
	}
}

func TestReadingASubscriptionReportsWhereItDelivers(t *testing.T) {
	deadline := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	srv, path := oneSubscriptionStub(t, http.StatusOK, map[string]any{
		"id":                 "sub-1",
		"notificationUrl":    testNotifyURL,
		"expirationDateTime": deadline.Format(time.RFC3339),
	})

	sub, err := NewAPI(srv.Client(), srv.URL).GetSubscription(context.Background(), "at", "sub-1")
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub.ID != "sub-1" || !sub.Expiration.Equal(deadline) {
		t.Errorf("sub = %+v, want the id and deadline Microsoft reported", sub)
	}
	if sub.NotificationURL != testNotifyURL {
		t.Errorf("NotificationURL = %q, want the endpoint — it is the whole reason for this read",
			sub.NotificationURL)
	}
	if got := path(); got != "/subscriptions/sub-1" {
		t.Errorf("read %q, want the subscription addressed inside its own collection", got)
	}
}

// A SUBSCRIPTION MICROSOFT NO LONGER KNOWS reads as gone, so the renewal above
// it registers afresh rather than reporting a fault the caller cannot act on.
func TestReadingAVanishedSubscriptionIsGoneRatherThanAnError(t *testing.T) {
	srv, _ := oneSubscriptionStub(t, http.StatusNotFound, nil)
	if _, err := NewAPI(srv.Client(), srv.URL).
		GetSubscription(context.Background(), "at", "sub-gone"); !errors.Is(err, ErrSubscriptionGone) {
		t.Fatalf("err = %v, want ErrSubscriptionGone", err)
	}
}

// AND A READ CANNOT BE AIMED BY A PROVIDER-SUPPLIED ID. The id arrives as a
// decoded field of a provider response, and one carrying a path segment would
// aim this authenticated GET — the user's own delegated token on it — at
// another resource. Refused rather than escaped: url.PathEscape leaves `..`
// intact, so a server that decodes %2F before resolving still walks out of the
// collection.
func TestAReadCannotBeAimedByAProviderSuppliedID(t *testing.T) {
	srv, path := oneSubscriptionStub(t, http.StatusOK, map[string]any{"id": "x"})
	if _, err := NewAPI(srv.Client(), srv.URL).
		GetSubscription(context.Background(), "at", "../../me/messages"); !errors.Is(err, ErrSubscriptionGone) {
		t.Fatalf("err = %v, want ErrSubscriptionGone for an id that cannot be addressed", err)
	}
	if got := path(); got != "" {
		t.Errorf("the request went out to %q — an id like this must not reach the wire at all", got)
	}
}

// A FAULT READING THE SUBSCRIPTION IS REPORTED, not turned into a fresh
// registration. Microsoft being unreachable for a moment is not evidence that
// the subscription is gone, and registering on it would leave the mailbox with
// a second subscription delivering the same notifications.
func TestAFaultReadingTheSubscriptionDoesNotRegisterASecondOne(t *testing.T) {
	api := &fakeAPI{email: owner, getSubErr: ErrUnreachable}
	c := pinnedConn(api)

	if _, err := c.RenewWatch(context.Background(), authBytes(t), testNotifyURL, "sub-stored"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want the fault reported", err)
	}
	if api.subCalls != 0 || api.renewCalls != 0 {
		t.Errorf("ensured %d and renewed %d after a failed read, want neither — a moment's "+
			"unreachability is not evidence the subscription is gone", api.subCalls, api.renewCalls)
	}
}
