// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dispactapp is the /mcp edge for the Margince app installed in a
// Dispact team. Dispact signs each call with a short-lived token for the person
// who asked its agent; this package checks that token against the connected
// Dispact bot and binds the call to that person's own passport.
package dispactapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/oidctoken"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The rules a call token is held to, as agreed with Dispact.
const (
	Audience = "app:margince:mcp"
	// maxLifetime bounds exp after iat. Dispact issues 60 s tokens.
	maxLifetime = 120 * time.Second
	// skew is how far exp and iat may be off this server's clock.
	skew = 30 * time.Second
	// jwksPath is where an install publishes the keys it signs with.
	jwksPath = "/.well-known/jwks.json"
	// passportLabel names the app on the passport a person sees in settings.
	passportLabel = "Dispact"
)

// ErrRejected is every refusal of the token itself. The transport answers 401.
var ErrRejected = errors.New("dispactapp: call token rejected")

// Bots answers the connected Dispact bot: its address is the token issuer,
// and its team id is the token's workspace_id.
type Bots interface {
	LiveBotIdentity(ctx context.Context, provider string) (capture.LiveBot, error)
}

// Seats resolves the installation and the person a verified token names.
type Seats interface {
	InstallationWorkspace(ctx context.Context) (ids.WorkspaceID, error)
	AuthenticateAppCaller(ctx context.Context, c identity.AppCaller) (identity.AgentIdentity, error)
}

// Replays holds each token id once until the token expires. FirstUse is false
// for an id already seen.
type Replays interface {
	FirstUse(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// Edge verifies Dispact call tokens. One verifier serves the connected
// install's address, so its key cache survives across calls.
type Edge struct {
	bots    Bots
	replays Replays
	client  *http.Client
	now     func() time.Time

	mu       sync.Mutex
	address  string
	verifier *oidctoken.Verifier
}

// New builds the edge. client fetches the install's keys, so it must refuse
// private addresses in production as the bot's own client does.
func New(bots Bots, replays Replays, client *http.Client) *Edge {
	return &Edge{bots: bots, replays: replays, client: client, now: time.Now}
}

// Wire builds the edge over the stored bot, Redis and the bot's own HTTP
// client, which refuses a private address unless private is set.
func Wire(db *database.DB, rdb *redis.Client, private bool) *Edge {
	return New(capture.NewRegistry(db, nil, nil, nil), RedisReplays{RDB: rdb}, chatsync.HTTPClient(private))
}

// SetClock replaces the clock the token's times are judged by.
func (e *Edge) SetClock(now func() time.Time) *Edge {
	e.now = now
	return e
}

// Or routes a call token through the edge and any other bearer to next, the
// passport path. A nil edge routes everything to next.
func (e *Edge) Or(seats Seats, next func(*http.Request) (context.Context, error)) func(*http.Request) (context.Context, error) {
	return func(r *http.Request) (context.Context, error) {
		bearer := httpserver.BearerToken(r.Header.Get("Authorization"))
		if e == nil || !IsCallToken(bearer) {
			return next(r)
		}
		wsID, err := seats.InstallationWorkspace(r.Context())
		if err != nil {
			return nil, fmt.Errorf("dispactapp: resolving the installation: %w: %w", err, agents.ErrAuthUnavailable)
		}
		return e.Authenticate(principal.WithWorkspaceID(r.Context(), wsID.UUID), seats, bearer)
	}
}

// IsCallToken reports a bearer shaped as a signed token: three segments. A
// passport token never has that shape.
func IsCallToken(bearer string) bool {
	return strings.Count(bearer, ".") == 2
}

// Authenticate checks one call token and binds the call to the person it
// names. A person with no member gets the app's list and nothing more.
func (e *Edge) Authenticate(ctx context.Context, seats Seats, bearer string) (context.Context, error) {
	bot, err := e.bots.LiveBotIdentity(ctx, chatlinks.ProviderDispact)
	if errors.Is(err, capture.ErrNoConnection) {
		return nil, fmt.Errorf("%w: no Dispact bot is connected", ErrRejected)
	}
	if err != nil {
		return nil, fmt.Errorf("dispactapp: reading the Dispact bot: %w: %w", err, agents.ErrAuthUnavailable)
	}
	claims, err := e.verifierFor(bot.Address).VerifyAs(ctx, bearer, e.matchCall(bot.TeamID))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	if err := e.spend(ctx, claims); err != nil {
		return nil, err
	}
	surface := agents.AppSurface{Tools: Tools}
	agent, err := seats.AuthenticateAppCaller(ctx, identity.AppCaller{
		Issuer: claims.Iss, Subject: claims.Sub, Email: claims.Email, Label: passportLabel,
	})
	switch {
	case errors.Is(err, identity.ErrNoSeat):
		surface.Seatless = noSeat{}
		return agents.WithAppSurface(ctx, surface), nil
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrConflict):
		return nil, fmt.Errorf("%w: the caller's passport is revoked or cannot be minted: %w", ErrRejected, err)
	case err != nil:
		return nil, fmt.Errorf("dispactapp: binding the caller's passport: %w: %w", err, agents.ErrAuthUnavailable)
	}
	ctx = principal.WithActor(agents.WithAppSurface(ctx, surface), agent.Principal())
	return principal.WithCorrelationID(ctx, ids.NewV7()), nil
}

// verifierFor returns the verifier for the install at address, building a new
// one when the connected address changed.
func (e *Edge) verifierFor(address string) *oidctoken.Verifier {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.verifier == nil || e.address != address {
		issuer := func(c oidctoken.Claims) error {
			if c.Iss != address {
				return fmt.Errorf("%w: iss %q is not the connected install", oidctoken.ErrRejected, c.Iss)
			}
			return nil
		}
		e.address = address
		e.verifier = oidctoken.New(address+jwksPath, issuer, oidctoken.IdentityResolvedPerRequest).
			WithHTTPClient(e.client).SetClock(e.now).SetSkew(skew)
	}
	return e.verifier
}

// matchCall holds the claims oidctoken does not: the audience, the team, the
// token's short life, its id, and the confirmed address.
func (e *Edge) matchCall(teamID string) func(oidctoken.Claims) error {
	return func(c oidctoken.Claims) error {
		switch {
		case c.Aud != Audience:
			return fmt.Errorf("aud %q is not %s", c.Aud, Audience)
		case teamID == "" || c.WorkspaceID != teamID:
			return fmt.Errorf("workspace_id %q is not the connected team", c.WorkspaceID)
		case c.Exp-c.Iat > int64(maxLifetime/time.Second) || c.Exp <= c.Iat:
			return fmt.Errorf("lifetime %ds is outside 1 to %d seconds", c.Exp-c.Iat, int64(maxLifetime/time.Second))
		case c.Jti == "" || c.Sub == "":
			return errors.New("jti and sub are required")
		case !c.EmailVerified || c.Email == "":
			return errors.New("the caller's email is not confirmed")
		}
		return nil
	}
}

// spend records the token's id until the token expires, so a second use of
// the same token is refused. A store that cannot answer refuses the call.
func (e *Edge) spend(ctx context.Context, c oidctoken.Claims) error {
	// Never under a second: Redis keeps a key with no expiry forever.
	ttl := max(time.Unix(c.Exp, 0).Add(skew).Sub(e.now()), time.Second)
	first, err := e.replays.FirstUse(ctx, c.Iss+"|"+c.Jti, ttl)
	if err != nil {
		return fmt.Errorf("dispactapp: recording the token id: %w: %w", err, agents.ErrAuthUnavailable)
	}
	if !first {
		return fmt.Errorf("%w: the token id was already used", ErrRejected)
	}
	return nil
}
