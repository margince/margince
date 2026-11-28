// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Deal Room buyer edge: /v1/public/rooms/* carries no seat session. This
// middleware, composed beside the booking and preference edges, throttles the
// surface and — for the operations that need one — resolves the Bearer to a
// room session and binds it together with a BUYER principal.
//
// What it does NOT do is bind a system principal. The other two public edges
// do, because their handlers go on to call RBAC-gated stores that a system
// principal passes. A buyer must not pass those gates: platform/auth refuses
// the buyer kind outright, so the only store methods that answer a buyer are
// the Deal Room's session-scoped ones. The three anonymous operations carry no
// actor at all; the store binds the one the credential names once it is known.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A literal, not dealrooms.PublicPathPrefix: the credential-path gate reads
// it from here. The kept-cookie integration tests fail if the two drift.
const publicDealRoomPrefix = "/v1/public/rooms/"

// The three operations that carry no session. Everything else under the prefix
// presents a Bearer or the session cookie.
var publicDealRoomAnonymous = map[string]bool{
	"peek":         true,
	"exchange":     true,
	"link-request": true,
}

// publicDealRoomLimiters: per-IP is the brake on everything; the link request
// gets a second, tighter one because each accepted call can put a mail on the
// wire, and a shared ceiling so a distributed flood cannot turn the relay into
// a spam source whatever the per-IP figure admits.
type publicDealRoomLimiters struct {
	perIP         *ratelimit.Limiter
	linkPerIP     *ratelimit.Limiter
	linkPerEmail  *ratelimit.Limiter
	linkShared    *ratelimit.Limiter
	perSessionMut *ratelimit.Limiter
}

func newPublicDealRoomLimiters() publicDealRoomLimiters {
	return publicDealRoomLimiters{
		perIP:     ratelimit.New("public-deal-room/per-ip", ratelimit.FailClosed, 60, time.Minute),
		linkPerIP: ratelimit.New("public-deal-room/link-per-ip", ratelimit.FailClosed, 3, time.Minute),
		// Per ADDRESS, whatever the source: a reissue retires the buyer's
		// standing credential, so one address may be reissued a few times an
		// hour and no more, or a distributed caller could keep a buyer's link
		// perpetually retired.
		linkPerEmail:  ratelimit.New("public-deal-room/link-per-address", ratelimit.FailClosed, 3, time.Hour),
		linkShared:    ratelimit.New("public-deal-room/link-shared", ratelimit.FailClosed, 10, time.Minute),
		perSessionMut: ratelimit.New("public-deal-room/per-session-write", ratelimit.FailOpen, 20, time.Minute),
	}
}

// linkSharedKey is the one bucket every link request shares.
const linkSharedKey = "deal_room_link_request"

func publicDealRoom(store *dealrooms.Store, limits publicDealRoomLimiters, origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, publicDealRoomPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			if !dealrooms.GuardPublicRequest(w, r, origin) {
				return
			}
			operation, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, publicDealRoomPrefix), "/")
			if !limits.perIP.Allow(httpserver.ClientIP(r)) {
				httperr.Write(w, r, ratelimit.Refused(limits.perIP))
				return
			}
			if publicDealRoomAnonymous[operation] {
				if operation == "link-request" {
					if bucket := linkRequestRefusedBy(limits, r); bucket != nil {
						httperr.Write(w, r, ratelimit.WaitOnly(max(limits.linkPerIP.RetryAfterSeconds(), limits.linkShared.RetryAfterSeconds(), limits.linkPerEmail.RetryAfterSeconds())))
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}
			sess, ok := resolveBuyerSession(store, w, r)
			if !ok {
				return
			}
			if r.Method != http.MethodGet && !limits.perSessionMut.Allow(sess.ID.String()) {
				httperr.Write(w, r, ratelimit.Refused(limits.perSessionMut))
				return
			}
			ctx := dealrooms.WithSession(r.Context(), sess)
			ctx = principal.WithActor(ctx, dealrooms.BuyerPrincipal(sess.ParticipantID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Read the cheapest buckets first. The per-address check needs the request body.
func linkRequestRefusedBy(limits publicDealRoomLimiters, r *http.Request) *ratelimit.Limiter {
	if !limits.linkPerIP.Allow(httpserver.ClientIP(r)) {
		return limits.linkPerIP
	}
	if !limits.linkShared.Allow(linkSharedKey) {
		return limits.linkShared
	}
	email, ok := peekLinkRequestEmail(r)
	if !ok {
		// Unparseable: let the handler answer the 422. Nothing is reissued.
		return nil
	}
	if !limits.linkPerEmail.Allow(email) {
		return limits.linkPerEmail
	}
	return nil
}

// peekLinkRequestEmail reads the address out of the body without consuming it
// for the handler, lowercased so the bucket matches the stored spelling.
func peekLinkRequestEmail(r *http.Request) (string, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, linkRequestBodyLimit))
	if err != nil {
		return "", false
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var body struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Email == "" {
		return "", false
	}
	return correspondence.Fold(body.Email), true
}

// linkRequestBodyLimit bounds what the edge will read to find the address: an
// address and a little JSON around it.
const linkRequestBodyLimit = 4 << 10

// resolveBuyerSession reads the presented token and resolves it, writing the
// one 401 every failure shares. A missing token, a malformed one, an unknown
// one and a revoked one are indistinguishable from outside.
func resolveBuyerSession(store *dealrooms.Store, w http.ResponseWriter, r *http.Request) (dealrooms.Session, bool) {
	token := dealrooms.PresentedSessionToken(r)
	if token == "" {
		httperr.Unauthorized(w, r, dealrooms.NoSessionDetail)
		return dealrooms.Session{}, false
	}
	sess, err := store.ResolveSession(r.Context(), token)
	if errors.Is(err, dealrooms.ErrSessionRefused) {
		httperr.Unauthorized(w, r, dealrooms.NoSessionDetail)
		return dealrooms.Session{}, false
	}
	if err != nil {
		// A database fault is not a refusal, and must not read as one: a buyer
		// told their link is dead will ask for a new one that is not needed.
		httperr.Write(w, r, err)
		return dealrooms.Session{}, false
	}
	return sess, true
}
