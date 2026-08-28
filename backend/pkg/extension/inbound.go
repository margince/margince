// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The inbound seam — a session-less HTTP edge a unit declares and the core
// mounts — is part of the published extension surface.
//
//margince:extension-surface

package extension

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The header names every inbound endpoint is signed with. They are fixed here
// rather than named per unit because the CORE reads two of the three: it checks
// the timestamp for freshness and carries the nonce as the replay key, and it
// cannot do either against a name only the unit knows.
//
// The signature covers all three parts, and the endpoint addressed as well —
// see SigningPayload — so none of them can be edited without invalidating it.
// The unit verifies it, because the secret lives in the unit's own namespace
// and the core cannot reach it.
const (
	// InboundHeaderTimestamp carries unix SECONDS, as decimal digits.
	InboundHeaderTimestamp = "X-Margince-Timestamp"

	// InboundHeaderNonce carries a caller-chosen unique value, hex-encoded.
	// It is what makes a correctly-signed request replayable exactly once:
	// the unit stores it against the endpoint and a second arrival with the
	// same one lands nothing.
	InboundHeaderNonce = "X-Margince-Nonce"

	// InboundHeaderSignature carries `sha256=<hex>`, the HMAC-SHA256 under
	// the endpoint's secret over exactly what SigningPayload builds. Read that
	// function before writing a sender: the material is not the three headers
	// concatenated, and a sender that assumes it is gets one opaque 401 with
	// nothing in it to say why.
	InboundHeaderSignature = "X-Margince-Signature"
)

// InboundEndpoint is one session-less HTTP edge a unit declares. A signed POST
// with no session reaches it, and nothing else about the request identifies a
// caller.
//
// It is a REQUEST an operator resolves, exactly as a Tool's tier and a
// SecretsRequest are: declaring an endpoint mounts nothing an operator has not
// enabled, and the bounds below are what the unit ASKS for — an installation
// ceiling may grant less, in which case the manifest records both numbers.
//
// The core owns admission and the unit owns meaning. The core resolves the
// installation's workspace, refuses an undeclared slug, caps the body, meters
// the two buckets, and refuses a timestamp outside Skew. The unit verifies the
// signature and decides what the payload means, because the secret is in the
// unit's namespace and the core has no way to read it.
type InboundEndpoint struct {
	// Slug names this endpoint within the unit. It is the last path segment of
	// `/webhooks/ext/<unit>/<slug>`, so it is public and appears in access
	// logs: the credential is the signing secret, never the path.
	//
	// It is stable. Changing it changes the URL every registered sender holds.
	Slug string

	// Secret is the bare key name of a user-scoped secret this unit DECLARED in
	// Secrets, and the one the signature is verified against. Boot refuses a
	// name the unit did not declare, so a typo is a refusal rather than an
	// endpoint that can never authenticate anything.
	Secret string

	// MaxBody bounds the request body in bytes.
	//
	// There is deliberately NO default. This is the one number that decides how
	// much an unauthenticated remote party can make the installation read per
	// request, and a defaultable cap is one a unit forgets to think about.
	MaxBody int64

	// Rate is what this endpoint asks to be metered at.
	Rate InboundRate

	// Skew is how far a request's timestamp may sit from the core's clock, in
	// either direction. Zero is invalid rather than meaning "no limit": an edge
	// with no freshness bound makes one captured request replayable forever,
	// and the nonce alone cannot fix that — the unit would have to keep every
	// nonce it has ever seen.
	Skew time.Duration

	// Handle is called once admission has passed. It runs on a Runtime minted
	// for this one request, whose Caller is nobody and whose permissions are
	// empty: an anonymous edge carries no authority, and anything the payload
	// eventually does on a member's behalf is minted later, from that member's
	// own live authority.
	Handle InboundHandler
}

// InboundHandler is what a unit does with an admitted request.
//
// It returns an InboundOutcome rather than a bare error because the STATUS a
// remote sender sees follows from the reason and not from a success bit — see
// the outcomes for which is which.
type InboundHandler func(ctx context.Context, rt Runtime, req InboundRequest) (InboundOutcome, error)

// InboundRequest is one admitted request as the unit sees it: the parts the
// signature covers, and nothing else.
//
// It carries no *http.Request. A unit that could read the URL, the remote
// address or arbitrary headers would be deciding on inputs the signature does
// not cover, which is how an edge that looks signed stops being one.
type InboundRequest struct {
	// Slug is the endpoint this arrived on — the unit's own declared value,
	// resolved by the core, never the raw path segment.
	Slug string

	// Ref is the trailing path segment, opaque to the core and owned by the
	// unit. Empty when the caller addressed the endpoint without one.
	//
	// It exists because Slug is DECLARED and therefore the same for every
	// caller, while a connector usually needs one URL per member. The unit
	// mints a Ref when a member opens an endpoint, stores it, and resolves it
	// back to that member's row and secret here.
	//
	// The core never interprets it, never keys a rate limiter on it, and never
	// resolves anything from it. That is the whole reason it is safe to accept
	// from the path: a value a caller chose becomes a limiter key only after it
	// has been resolved against something declared, and this one is resolved
	// against the unit's own table instead.
	//
	// It is bounded (MaxInboundRef) and drawn from a closed alphabet, so it is
	// loggable and cannot be used to smuggle a path. It is NOT a credential —
	// it appears in access logs exactly as the slug does, and the signature is
	// what authenticates the request. A unit that treats a Ref as a secret has
	// published its secret in every proxy log between here and the sender.
	Ref string

	// Timestamp is the value the caller signed, already checked against Skew.
	Timestamp time.Time

	// Nonce is the value the caller signed. It is the unit's replay key: store
	// it unique per endpoint, and a second arrival with the same one lands
	// nothing. Bounded (MaxInboundNonce) before it reaches a unit: a
	// caller-chosen value a unit is asked to KEEP is a caller-chosen write.
	Nonce string

	// Signature is the presented `sha256=<hex>` MAC, verbatim. The unit
	// recomputes the expected value over SignedPayload and compares with
	// hmac.Equal — never with ==, and never after decoding only one side.
	Signature string

	// Body is the request body, at most MaxBody bytes.
	Body []byte
}

// SigningScope names WHICH exchange a MAC was minted for. It is the first line
// of every signed payload, and it is what stops one exchange's signature from
// being valid material in another.
//
// The case it exists for is concrete and was otherwise live: a unit that both
// sends signed messages and receives them, under one secret, over one payload
// spelling, produces outbound requests that are byte-for-byte valid INBOUND
// requests. Any party trusted only to receive could relay what it was sent
// straight back to the installation's own edge and be authenticated by it,
// without ever holding the secret. Two scopes make those two messages
// different messages.
type SigningScope string

const (
	// ScopeInbound is a request arriving at a declared inbound endpoint.
	ScopeInbound SigningScope = "margince-inbound-v1"
	// ScopeOutbound is a request a unit sends to a destination someone
	// registered with it.
	ScopeOutbound SigningScope = "margince-outbound-v1"
)

// SigningPayload is the exact material a signature covers, for either
// direction. Compute the expected MAC over this and over nothing else: a
// verifier that re-spells the concatenation is a verifier that will one day
// spell it differently from the sender, and the failure looks like a wrong
// secret rather than a rule that moved.
//
// The encoding is UNAMBIGUOUS, which a plain join is not. Every field before
// the body is drawn from an alphabet with no newline in it — a scope is a
// constant here, a slug and a ref are held to their published grammars, the
// timestamp is decimal and the nonce is hex — so the first five newlines are
// the five separators and nothing else can be read as one. The earlier
// `<ts>.<nonce>.<body>` spelling had no such property: moving the boundary
// rightwards produced a DIFFERENT nonce over IDENTICAL signing material, so
// one captured request could be replayed once per separator in its own body,
// each replay landing a fresh row past a uniqueness index that never saw a
// collision. Nothing about that attack needed the secret.
func SigningPayload(scope SigningScope, slug, ref string, at time.Time, nonce string, body []byte) []byte {
	head := fmt.Sprintf("%s\n%s\n%s\n%d\n%s\n", scope, slug, ref, at.Unix(), nonce)
	payload := make([]byte, 0, len(head)+len(body))
	payload = append(payload, head...)
	return append(payload, body...)
}

// SignedPayload is the material an arriving request's signature covers.
//
// Slug and Ref are IN it: without them a request captured on one endpoint is
// valid on every other endpoint the installation declares, which is a
// distinction the URL makes and the signature would otherwise not.
func (r InboundRequest) SignedPayload() []byte {
	return SigningPayload(ScopeInbound, r.Slug, r.Ref, r.Timestamp, r.Nonce, r.Body)
}

// InboundOutcome is why a handler stopped. It mirrors the response discipline
// the core's webhook chassis applies to provider callbacks, plus the one an
// anonymous signed edge needs that a provider callback does not.
type InboundOutcome int

const (
	// InboundAccepted means the request was verified and durably recorded.
	// The core answers 202: recorded is not the same as acted on, and this
	// edge deliberately does not do the acting.
	InboundAccepted InboundOutcome = iota

	// InboundUnauthenticated means the request did not verify. The core
	// answers ONE opaque 401 with an empty body, identical for every reason a
	// unit might return this — an unknown endpoint, a disabled one, a replayed
	// nonce, a wrong signature — because an error that distinguishes them
	// enumerates the installation's endpoints for whoever asks.
	InboundUnauthenticated

	// InboundOverCapacity means the request verified but the unit will not
	// hold it: a bounded queue that is full. The core answers 429. It is
	// separate from Transient because it is not a fault and retrying sooner
	// will not help.
	InboundOverCapacity

	// InboundPoison means the request verified and could not be acted on, and
	// redelivering the same bytes cannot change that. The core answers 202, so
	// a sender does not retry forever against a payload we will never accept.
	InboundPoison

	// InboundTransient means the unit failed for a reason redelivery can fix.
	// The core answers 500.
	InboundTransient
)

// InboundRate is the two buckets an inbound endpoint is metered on. Both are
// required: the endpoint bucket bounds what one sender can cost this
// installation, and the client-IP bucket is what still brakes a flood spread
// across many endpoints.
type InboundRate struct {
	// PerIP meters by the client address the core trusts, not by a header a
	// caller can write.
	PerIP Rate

	// PerEndpoint meters every request that resolves to this DECLARED
	// endpoint, whatever its source.
	//
	// SIZE IT FOR THE WHOLE INSTALLATION, not for one sender. A slug comes
	// from the declaration and is the same for every caller, so where a unit gives
	// each member their own URL by minting a trailing Ref, all of those URLs
	// resolve to this one declaration and spend this one budget. The Ref
	// cannot be the meter's key — a caller-chosen key is a caller-choosing
	// its own budget, and ratelimit leaves an over-long key unmetered and
	// admitted — so members share fate here by construction.
	//
	// The consequence to size against: this bucket is spent BEFORE a
	// signature is checked, deliberately, so that an unverified request
	// cannot cost a secret read. An anonymous party sending junk therefore
	// spends the same budget a member's real sender needs, and a limit set
	// for one sender's traffic is a limit an anonymous party can exhaust on
	// everyone's behalf. Set it against the installation's total expected
	// arrival rate with headroom, and rely on PerIP to brake a single source.
	PerEndpoint Rate
}

// Rate is a fixed-window allowance: Limit requests per Window.
type Rate struct {
	Limit  int
	Window time.Duration
}

// inboundSlugGrammar bounds a slug to what sits in a URL path segment and an
// access log without quoting — the same shape a unit name and an ingress
// system key take.
var inboundSlugGrammar = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxInboundSlugLength bounds the slug's share of the mounted path. It is short
// deliberately: the path is a rate-limiter key, and ratelimit refuses an
// over-long key by leaving it UNMETERED, so a slug that could grow without
// bound would be a self-serve way off the meter.
const maxInboundSlugLength = 32

// MaxInboundRef bounds the trailing path segment a caller may address an
// endpoint with. Short, because it is a handle the unit minted and not a
// payload: anything longer is a caller trying to use the URL as a channel.
const MaxInboundRef = 64

// inboundRefGrammar bounds a Ref to what sits in one URL path segment and an
// access log without escaping. It admits upper case, unlike a slug, because a
// unit mints these and base64url or hex is the obvious way to.
var inboundRefGrammar = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidInboundRef reports whether a trailing path segment is one the core will
// pass to a unit. It is published so a unit minting a Ref can hold itself to the
// same rule the edge applies, rather than discovering the mismatch as a 404 on a
// URL it has already given to somebody.
func ValidInboundRef(ref string) bool {
	return len(ref) <= MaxInboundRef && inboundRefGrammar.MatchString(ref)
}

// MaxInboundNonce bounds the replay key a caller may present. The nonce is
// caller-chosen and this edge asks a unit to STORE it, under a uniqueness
// index that by construction never collapses two of them — so an unbounded one
// is an unbounded write, and the ref beside it is bounded for the same reason.
// 128 characters is four times the 32 a 128-bit hex value needs, which is past
// any sender's idea of unique and nowhere near a payload.
const MaxInboundNonce = 128

// inboundNonceGrammar is the alphabet the nonce header has always documented.
// It is ENFORCED rather than merely documented because SigningPayload's
// unambiguity rests on it: a nonce free to contain the separator could absorb
// the head of the body and present itself as a different replay key over the
// same signed bytes.
var inboundNonceGrammar = regexp.MustCompile(`^[0-9a-fA-F]+$`)

// ValidInboundNonce reports whether a presented replay key is one the core will
// pass to a unit. Published beside ValidInboundRef so a sender can hold itself
// to the rule rather than meeting it as an opaque refusal.
func ValidInboundNonce(nonce string) bool {
	return len(nonce) <= MaxInboundNonce && inboundNonceGrammar.MatchString(nonce)
}

// MaxInboundBody is the largest body any declared endpoint may ask for. It
// bounds what one unauthenticated request costs before its signature is even
// checked, which is the only point at which the installation still has no
// reason to trust the sender.
const MaxInboundBody int64 = 1 << 20

// MaxInboundSkew is the largest freshness window a declared endpoint may ask
// for. Beyond it, a captured request stays replayable for longer than any
// bounded nonce store can cover.
const MaxInboundSkew = 15 * time.Minute

// Validate enforces what an endpoint must state to be mountable. Both the
// manifest generator and the boot preflight run this same check, so a
// declaration that reached the composed set outside the generator path is
// judged the same way.
func (e InboundEndpoint) Validate() error {
	if err := e.validateSlug(); err != nil {
		return err
	}
	if strings.TrimSpace(e.Secret) == "" {
		return fmt.Errorf("extension: inbound endpoint %q names no secret — an edge with nothing to verify against admits whatever arrives", e.Slug)
	}
	switch {
	case e.MaxBody <= 0:
		return fmt.Errorf("extension: inbound endpoint %q sets no body cap — it decides how much an unauthenticated sender can make this installation read, so it has no default", e.Slug)
	case e.MaxBody > MaxInboundBody:
		return fmt.Errorf("extension: inbound endpoint %q asks for a %d-byte body cap, over the %d-byte ceiling", e.Slug, e.MaxBody, MaxInboundBody)
	case e.Skew <= 0:
		return fmt.Errorf("extension: inbound endpoint %q sets no clock skew — an edge with no freshness bound leaves one captured request replayable indefinitely", e.Slug)
	case e.Skew > MaxInboundSkew:
		return fmt.Errorf("extension: inbound endpoint %q asks for a %s skew, over the %s ceiling", e.Slug, e.Skew, MaxInboundSkew)
	case e.Handle == nil:
		return fmt.Errorf("extension: inbound endpoint %q declares no handler", e.Slug)
	}
	return e.Rate.validate(e.Slug)
}

func (e InboundEndpoint) validateSlug() error {
	switch {
	case strings.TrimSpace(e.Slug) == "":
		return errors.New("extension: a declared inbound endpoint has an empty slug")
	case len(e.Slug) > maxInboundSlugLength:
		return fmt.Errorf("extension: inbound slug %q is %d characters — it keys a rate limiter, which leaves an over-long key unmetered, so it is capped at %d",
			e.Slug, len(e.Slug), maxInboundSlugLength)
	case !inboundSlugGrammar.MatchString(e.Slug):
		return fmt.Errorf("extension: inbound slug %q is not a slug (lower-case [a-z0-9] segments joined by single hyphens)", e.Slug)
	}
	return nil
}

func (r InboundRate) validate(slug string) error {
	if err := r.PerIP.validate(slug, "per-IP"); err != nil {
		return err
	}
	return r.PerEndpoint.validate(slug, "per-endpoint")
}

func (r Rate) validate(slug, bucket string) error {
	switch {
	case r.Limit <= 0:
		return fmt.Errorf("extension: inbound endpoint %q declares no %s allowance — an unmetered anonymous edge is one a single sender decides the cost of", slug, bucket)
	case r.Window <= 0:
		return fmt.Errorf("extension: inbound endpoint %q declares a %s allowance over no window", slug, bucket)
	}
	return nil
}
