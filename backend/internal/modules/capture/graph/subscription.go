// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The Microsoft Graph change-notification subscription: the registering half of
// Outlook push. Its consuming half is the webhook compose mounts.
//
// A subscription is the Microsoft twin of Gmail's users.watch and is maintained
// the same way — the registry's renewal sweep calls Watch before the deadline it
// stored — with two differences Microsoft imposes:
//
//   - It lapses in under THREE DAYS (4230 minutes for a message resource),
//     where Gmail's watch lasts seven. The renewal cadence is the operator's to
//     set and the deadline this returns is what the sweep keys on, so a
//     deployment that renews daily never notices; one that renews weekly would
//     go dark, which is why the sweep reads the stored deadline rather than
//     assuming a period.
//   - It is addressed by an id Microsoft mints, where Gmail's watch is
//     addressed by the mailbox. Rather than storing that id — a column, a
//     migration, and a value that can go stale against the provider — this
//     ASKS: Microsoft lists the subscriptions this app holds for this user, and
//     the one pointing at our notification URL is ours. The provider is the
//     source of truth for its own subscription, so a subscription created by a
//     previous deployment is adopted rather than duplicated.
//
// The subscription covers `/me/messages` — the whole mailbox rather than the
// folders Sync reads. A notification carries no content and is only a signal to
// sync now, and the sync decides for itself what to capture; narrowing the
// subscription would buy nothing and lose the sent copy the moment Sync's
// folder set changed under it.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/graphconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	// subscriptionsPath is Microsoft's subscription collection.
	subscriptionsPath = "/subscriptions"
	// messagesResource is what the subscription watches. Quoted the way Graph
	// spells a resource path, which is not the same as the request path.
	messagesResource = "/me/messages"
	// createdChange is the only change type worth a sync: an updated or deleted
	// message changes nothing this system has already captured, and asking for
	// them would wake a sync on every read receipt.
	createdChange = "created"
	// maxSubscriptionMinutes is Microsoft's ceiling for a message resource
	// (4230 minutes, just under three days). Requested slightly under it
	// because the ceiling is evaluated against Microsoft's clock, not ours, and
	// a request a few seconds over is refused outright rather than clamped.
	maxSubscriptionMinutes = 4200
	// maxSubscriptionPages bounds the listing walk. An app holds a handful of
	// subscriptions per user, so one page is the real answer and this exists so
	// a listing that never ends stops rather than spinning.
	maxSubscriptionPages = 20
	// maxClientStateLen is Microsoft's ceiling for the echoed clientState.
	maxClientStateLen = 128
)

// errNoSubscriptionDeadline marks a subscription Microsoft accepted without
// saying when it lapses. Refused rather than defaulted: a guessed deadline is
// one the renewal sweep would key on, and guessing long is a mailbox that goes
// quiet with nothing to notice it.
var errNoSubscriptionDeadline = errors.New("graph: the subscription carries no expiration")

// errNoSubscriptionOwner is a credential bundle with no mailbox address on it.
//
// Refused BEFORE the subscription is created, because clientState is what the
// webhook routes on: an empty one is omitted from the request, and Microsoft
// then echoes nothing — so every notification the subscription ever delivers is
// dropped as unroutable, and the mailbox is silently poll-only with a
// subscription that looks healthy.
var errNoSubscriptionOwner = fmt.Errorf(
	"graph: this connection's credential names no mailbox, so a subscription for it could not be routed: %w",
	connector.ErrAuthRejected,
)

// Subscription is what a registration round settled on: the subscription that
// now covers this mailbox, and when it lapses. Exported because it crosses the
// API seam a test substitutes.
type Subscription struct {
	ID         string
	Expiration time.Time
}

// subscription is Microsoft's wire shape for one, in the fields this connector
// uses.
type subscription struct {
	ID              string    `json:"id"`
	Resource        string    `json:"resource"`
	NotificationURL string    `json:"notificationUrl"`    //nolint:tagliatelle // Microsoft's wire format; must match to decode
	Expiration      time.Time `json:"expirationDateTime"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
}

// subscriptionRequest is what creating one asks for. Renewal sends the
// expiration alone, which is why every other field is omitempty.
type subscriptionRequest struct {
	ChangeType      string    `json:"changeType,omitempty"`      //nolint:tagliatelle // Microsoft's wire format
	NotificationURL string    `json:"notificationUrl,omitempty"` //nolint:tagliatelle // Microsoft's wire format
	Resource        string    `json:"resource,omitempty"`
	ClientState     string    `json:"clientState,omitempty"` //nolint:tagliatelle // Microsoft's wire format
	Expiration      time.Time `json:"expirationDateTime"`    //nolint:tagliatelle // Microsoft's wire format
}

// Watch registers or renews this mailbox's Graph subscription against
// notificationURL and reports when it lapses.
//
// It performs provider I/O and touches nothing else; the registry persists the
// deadline and schedules the next call, exactly as it does for Gmail.
//
// HistoryID is deliberately empty. Gmail's watch returns a history anchor the
// registry also declines to store — writing it would suppress the first sync's
// backfill — and Graph has no equivalent to return at all: the delta cursor is
// Sync's own and a subscription says nothing about it.
func (c *Connector) Watch(ctx context.Context, auth connector.Auth, notificationURL string) (connector.WatchResult, error) {
	st, err := graphconn.Read(connectorName, auth)
	if err != nil {
		return connector.WatchResult{}, err
	}
	if st.Owner == "" {
		return connector.WatchResult{}, errNoSubscriptionOwner
	}
	// Microsoft caps clientState at 128 characters and refuses the whole
	// creation above it. Refused here instead, by name: an address that long is
	// legal (RFC 5321 allows 254) and the vendor's error for it says nothing
	// about which field was too long.
	if len(st.Owner) > maxClientStateLen {
		return connector.WatchResult{}, fmt.Errorf(
			"graph: this mailbox's address is %d characters and Microsoft's clientState holds %d, so a subscription for it could not carry the address the webhook routes on: %w",
			len(st.Owner), maxClientStateLen, connector.ErrSkip)
	}
	access, err := c.oauth.AccessToken(ctx, st.RefreshToken)
	if err != nil {
		return connector.WatchResult{}, err
	}
	deadline := c.now().Add(maxSubscriptionMinutes * time.Minute).UTC()

	// The mailbox OWNER rides in clientState, which Microsoft echoes verbatim on
	// every notification. Microsoft's own guidance is to use it as a secret; this
	// deployment authenticates a notification with the operator token on the
	// notification URL instead — the same shared secret Gmail's push carries —
	// and spends clientState on the one thing a Graph notification otherwise
	// cannot say: WHICH mailbox it is about. Its `resource` names a directory
	// object id this system never stored.
	sub, err := c.api.EnsureSubscription(ctx, access, notificationURL, st.Owner, deadline)
	if err != nil {
		return connector.WatchResult{}, err
	}
	if sub.Expiration.IsZero() {
		return connector.WatchResult{}, errNoSubscriptionDeadline
	}
	// Never later than what was asked for. The registry stores this and the
	// renewal scan selects on it, so a deadline further out than Microsoft can
	// legitimately grant is a connection the scan never picks up again — while
	// the real subscription lapses within three days and the mailbox falls back
	// to the poll with nothing failing to say so. What was requested is the
	// ceiling, and it is already computed one line above.
	if sub.Expiration.After(deadline) {
		sub.Expiration = deadline
	}
	return connector.WatchResult{ExpiresAt: sub.Expiration}, nil
}

// EnsureSubscription renews the subscription already pointing at
// notificationURL, or creates one when there is none.
//
// Renew-then-create rather than create-then-dedupe: Microsoft accepts several
// subscriptions on one resource, so a create-first loop would leave a mailbox
// with one more every renewal cycle, each of them delivering the same
// notification.
func (a *httpAPI) EnsureSubscription(
	ctx context.Context, accessToken, notificationURL, clientState string, deadline time.Time,
) (Subscription, error) {
	existing, err := a.findSubscription(ctx, accessToken, notificationURL)
	if err != nil {
		return Subscription{}, err
	}
	if existing != "" {
		renewed, err := a.renewSubscription(ctx, accessToken, existing, deadline)
		if err == nil {
			return renewed, nil
		}
		// Gone since the list named it — Microsoft drops a subscription whose
		// endpoint failed too often, and the recovery is a new one rather than a
		// failed round. Any other fault stops here.
		if !errors.Is(err, errSubscriptionGone) {
			return Subscription{}, err
		}
	}
	return a.createSubscription(ctx, accessToken, notificationURL, clientState, deadline)
}

// errSubscriptionGone is a renewal Microsoft refused because the subscription is
// no longer there. Internal to this file: the caller's answer is to create one,
// never to report it.
var errSubscriptionGone = errors.New("graph: the subscription no longer exists")

// findSubscription returns the id of this app's subscription for this user
// pointing at notificationURL, or empty.
//
// Matched on the notification URL ALONE, and never additionally on the
// resource. The URL is a value this deployment chose and Microsoft stores
// verbatim, so comparing it is comparing our own bytes; `resource` is a value
// Microsoft may echo in a normalized form of its own, and a comparison that
// missed would create a second subscription every renewal round — each one
// delivering the same notification, forever.
//
// The URL carries the operator token, so a subscription left behind by a
// rotated token is correctly seen as somebody else's and replaced rather than
// extended.
func (a *httpAPI) findSubscription(ctx context.Context, accessToken, notificationURL string) (string, error) {
	next := a.base + subscriptionsPath
	for page := 0; page < maxSubscriptionPages; page++ {
		var body struct {
			Value    []subscription `json:"value"`
			NextLink string         `json:"@odata.nextLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
		}
		if _, err := a.get(ctx, accessToken, next, nil, &body); err != nil {
			return "", err
		}
		for _, s := range body.Value {
			if s.NotificationURL == notificationURL {
				return s.ID, nil
			}
		}
		if body.NextLink == "" {
			return "", nil
		}
		if err := a.sameAPIOrigin(body.NextLink); err != nil {
			return "", err
		}
		next = body.NextLink
	}
	// A listing that did not end is a listing that proves nothing: falling
	// through to create here is what would add one more subscription every
	// renewal cycle, each delivering the same notification — the accumulation
	// renew-then-create exists to prevent.
	return "", errSubscriptionListUnbounded
}

// isSubscriptionID reports whether id can be spliced into a URL path as ONE
// segment naming a subscription.
//
// The test is that the id is ALREADY its own escaping, plus the two dot
// segments that survive escaping untouched. Anything needing an escape — a
// separator, a query or fragment marker, a space, a stray percent — is not an
// identifier Microsoft minted, and `.` and `..` are path instructions wearing an
// id's clothes.
//
// Stated as what an id IS rather than as a list of traversal spellings: a
// blacklist is a list somebody has to keep complete, against an input whose
// whole point is that it was chosen by the far end.
func isSubscriptionID(id string) bool {
	return id != "" && id != "." && id != ".." && url.PathEscape(id) == id
}

// errSubscriptionListUnbounded is a subscription listing that would not end.
var errSubscriptionListUnbounded = fmt.Errorf(
	"graph: the subscription listing did not end within %d pages: %w", maxSubscriptionPages, ErrUnreachable,
)

func (a *httpAPI) renewSubscription(ctx context.Context, accessToken, id string, deadline time.Time) (Subscription, error) {
	// REFUSED, not merely escaped. The id arrives as a decoded field of a
	// provider response, and one carrying a path segment would aim this
	// authenticated PATCH — the user's own delegated token on it — at another
	// resource. url.PathEscape alone does not settle that: it escapes the
	// separators and leaves `..` intact, so a server that decodes %2F before
	// resolving the path still walks out of the collection. Rejecting the id
	// does not depend on how the far end normalizes.
	if !isSubscriptionID(id) {
		// GONE, not unreachable: the subscription this names cannot be
		// addressed, so from the round's seat there is nothing to renew — and
		// the round answers that by creating one, which is the safe direction.
		// Failing instead would leave the mailbox on the poll for as long as
		// the provider kept answering with an id like this.
		return Subscription{}, errSubscriptionGone
	}
	var out subscription
	status, err := a.writeJSON(ctx, http.MethodPatch,
		a.base+subscriptionsPath+"/"+url.PathEscape(id), accessToken,
		subscriptionRequest{Expiration: deadline}, &out)
	if err != nil {
		if status == http.StatusNotFound {
			return Subscription{}, errSubscriptionGone
		}
		return Subscription{}, err
	}
	return Subscription{ID: out.ID, Expiration: out.Expiration}, nil
}

func (a *httpAPI) createSubscription(
	ctx context.Context, accessToken, notificationURL, clientState string, deadline time.Time,
) (Subscription, error) {
	var out subscription
	if _, err := a.writeJSON(ctx, http.MethodPost, a.base+subscriptionsPath, accessToken, subscriptionRequest{
		ChangeType:      createdChange,
		NotificationURL: notificationURL,
		Resource:        messagesResource,
		ClientState:     clientState,
		Expiration:      deadline,
	}, &out); err != nil {
		return Subscription{}, fmt.Errorf("graph: creating the change-notification subscription: %w", err)
	}
	return Subscription{ID: out.ID, Expiration: out.Expiration}, nil
}
