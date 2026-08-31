// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A hand-rolled verifier for an RS256 OIDC ID token — the one Google Pub/Sub
// attaches to a push request (Authorization: Bearer <jwt>), and the ones the
// Google and Microsoft sign-in flows exchange a code for. Keys are fetched from
// the issuer's JWKS endpoint and cached per its Cache-Control max-age. It checks
// the signature, the exp/iat window, and hands the claims to two injected
// predicates: WHOSE token this is (checkIssuer) and WHICH identity it may speak
// for (matchIdentity). No new module dependency — crypto/rsa + net/http,
// mirroring gmail/client.go's hand-rolled provider I/O. Every rejection
// collapses to one opaque error; the caller answers 401 and logs the detail
// server-side (never echoed to the client).

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
)

const googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

// oidcSkew tolerates small clock differences on exp/iat.
const oidcSkew = 2 * time.Minute

// jwksRefreshCooldown bounds JWKS refreshes across calls, not just within
// one: the header's alg/kid are read before any signature check, so an
// unauthenticated caller can force a cache miss on every request just by
// sending a never-seen kid. Without this cooldown, a burst of such tokens
// would drive one outbound HTTPS fetch (and one hold of v.mu) per request.
const jwksRefreshCooldown = time.Minute

// errOIDCRejected is the single opaque failure the verifier returns; the
// wrapped cause is for server-side logs only.
var errOIDCRejected = errors.New("oidc: token rejected")

type oidcTokenVerifier struct {
	jwksURL string
	// checkIssuer settles WHOSE token this is, and is separate from
	// matchIdentity because the two answer different questions and only one of
	// them may ever be omitted. Every constructor supplies an issuer check as a
	// positional argument, so a new provider cannot reach the signature check
	// having forgotten it — the failure that would otherwise let any IdP with a
	// reachable JWKS mint a token this verifier accepts.
	checkIssuer   func(oidcClaims) error
	matchIdentity func(oidcClaims) error
	client        *http.Client
	now           func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	expires     time.Time
	nextRefresh time.Time
	inflight    *jwksRefreshFlight
}

// jwksRefreshFlight coalesces concurrent JWKS refreshes: the first caller
// fetches, everyone arriving while the fetch is in flight waits on done and
// shares its outcome instead of being rejected by the cooldown.
type jwksRefreshFlight struct {
	done chan struct{}
	err  error
}

// newOIDCVerifier builds a verifier for one issuer. checkIssuer and
// matchIdentity are both callbacks so a shared verifier does not need to
// hardcode which provider — or which caller — it is: Google Pub/Sub and Google
// sign-in read the same issuer and different identities, while Microsoft reads
// a different issuer entirely.
func newOIDCVerifier(jwksURL string, checkIssuer, matchIdentity func(oidcClaims) error) *oidcTokenVerifier {
	return &oidcTokenVerifier{
		jwksURL:       jwksURL,
		checkIssuer:   checkIssuer,
		matchIdentity: matchIdentity,
		client:        &http.Client{Timeout: 30 * time.Second},
		now:           time.Now,
	}
}

// newGoogleOIDCVerifier is the Google-issuer verifier: an empty jwksURL falls
// back to Google's own endpoint, and the issuer check is supplied here rather
// than by each caller so no Google caller can be composed without one.
func newGoogleOIDCVerifier(jwksURL string, matchIdentity func(oidcClaims) error) *oidcTokenVerifier {
	if jwksURL == "" {
		jwksURL = googleJWKSURL
	}
	return newOIDCVerifier(jwksURL, googleIssuer, matchIdentity)
}

// googleIssuer accepts the two spellings Google issues ID tokens under.
func googleIssuer(c oidcClaims) error {
	if c.Iss != "accounts.google.com" && c.Iss != "https://accounts.google.com" {
		return fmt.Errorf("%w: iss %q", errOIDCRejected, c.Iss)
	}
	return nil
}

func (v *oidcTokenVerifier) withHTTPClient(c *http.Client) *oidcTokenVerifier {
	v.client = c
	return v
}

func (v *oidcTokenVerifier) withClock(now func() time.Time) *oidcTokenVerifier {
	v.now = now
	return v
}

type oidcHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type oidcClaims struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Exp           int64  `json:"exp"`
	Iat           int64  `json:"iat"`
	// Tid is the Microsoft directory the account belongs to. Google issues no
	// such claim; for Microsoft it is what says WHICH tenant vouched for the
	// email, and the issuer is derived from it, so the two are read together.
	Tid string `json:"tid"`
	// PreferredUsername is Microsoft's sign-in name for the account. Read only
	// as a fallback where a work/school account carries no `email` claim, and
	// only under a tenant this installation trusts.
	PreferredUsername string `json:"preferred_username"`
}

// Verify returns the decoded claims only for a well-formed, correctly-signed
// token whose issuer and identity both pass the injected checks.
func (v *oidcTokenVerifier) Verify(ctx context.Context, bearer string) (oidcClaims, error) {
	if bearer == "" {
		return oidcClaims{}, fmt.Errorf("%w: empty bearer", errOIDCRejected)
	}
	parts := strings.Split(bearer, ".")
	if len(parts) != 3 {
		return oidcClaims{}, fmt.Errorf("%w: not a JWT", errOIDCRejected)
	}
	hdr, err := decodeHeaderSegment(parts[0])
	if err != nil {
		return oidcClaims{}, fmt.Errorf("%w: header: %v", errOIDCRejected, err)
	}
	if hdr.Alg != "RS256" {
		return oidcClaims{}, fmt.Errorf("%w: alg %q not RS256", errOIDCRejected, hdr.Alg)
	}
	key, err := v.key(ctx, hdr.Kid)
	if err != nil {
		return oidcClaims{}, fmt.Errorf("%w: key: %v", errOIDCRejected, err)
	}
	if err := verifyRS256(key, parts[0]+"."+parts[1], parts[2]); err != nil {
		return oidcClaims{}, fmt.Errorf("%w: signature: %v", errOIDCRejected, err)
	}
	claims, err := decodeClaimsSegment(parts[1])
	if err != nil {
		return oidcClaims{}, fmt.Errorf("%w: claims: %v", errOIDCRejected, err)
	}
	// A rejected token returns the zero value, never the claims it decoded
	// but refused: a caller that checks err after claims — one careless
	// read away given every caller here checks err first today — must not
	// be handed an attacker-controlled email/sub on the rejection path.
	if err := v.checkClaims(claims); err != nil {
		return oidcClaims{}, err
	}
	return claims, nil
}

func (v *oidcTokenVerifier) checkClaims(c oidcClaims) error {
	// Fail CLOSED on a missing issuer check rather than calling through a nil
	// and panicking. Both constructors supply one, so this cannot happen today
	// — but the consequence of it happening is a token nobody has established
	// the issuer of, and the safe reading of "I was not told whose tokens to
	// accept" is none.
	if v.checkIssuer == nil {
		return fmt.Errorf("%w: no issuer check wired", errOIDCRejected)
	}
	if err := v.checkIssuer(c); err != nil {
		return err
	}
	now := v.now()
	if c.Exp == 0 || now.After(time.Unix(c.Exp, 0).Add(oidcSkew)) {
		return fmt.Errorf("%w: expired", errOIDCRejected)
	}
	if c.Iat == 0 {
		return fmt.Errorf("%w: missing iat", errOIDCRejected)
	}
	if now.Add(oidcSkew).Before(time.Unix(c.Iat, 0)) {
		return fmt.Errorf("%w: issued in the future", errOIDCRejected)
	}
	return v.matchIdentity(c)
}

// key returns the cached public key for kid, refreshing the JWKS if the
// cache is empty, expired, or missing the kid (a rotation) — subject to
// jwksRefreshCooldown throttling refreshes across calls.
func (v *oidcTokenVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("no kid")
	}
	if k, ok := v.lookupKey(kid); ok {
		return k, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	k, ok := v.lookupKey(kid)
	if !ok {
		return nil, fmt.Errorf("unknown kid %q", kid)
	}
	return k, nil
}

// lookupKey reports the cached key for kid, if any, and whether the cache
// (as a whole) is still within its TTL.
func (v *oidcTokenVerifier) lookupKey(kid string) (*rsa.PublicKey, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && v.now().Before(v.expires) {
		return k, true
	}
	return nil, false
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// refresh bounds JWKS fetches: concurrent callers coalesce onto one in-flight
// fetch (waiting for its result rather than being rejected), and once a fetch
// completes, further refreshes are throttled for jwksRefreshCooldown. The
// network fetch runs without holding v.mu — only the flight bookkeeping and
// the cache swap are locked.
func (v *oidcTokenVerifier) refresh(ctx context.Context) error {
	v.mu.Lock()
	if fl := v.inflight; fl != nil {
		v.mu.Unlock()
		select {
		case <-fl.done:
			return fl.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if v.now().Before(v.nextRefresh) {
		v.mu.Unlock()
		return errors.New("jwks: refresh throttled")
	}
	fl := &jwksRefreshFlight{done: make(chan struct{})}
	v.inflight = fl
	v.mu.Unlock()

	keys, expires, err := v.fetchJWKS(ctx)

	v.mu.Lock()
	if err == nil {
		v.keys = keys
		v.expires = expires
	}
	v.nextRefresh = v.now().Add(jwksRefreshCooldown)
	v.inflight = nil
	fl.err = err
	close(fl.done)
	v.mu.Unlock()
	return err
}

// fetchJWKS performs the outbound HTTPS GET and parses the key set. It takes
// no lock: it is called from refresh with v.mu already released.
func (v *oidcTokenVerifier) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	// No credential rides on this request — a key set is published for anyone —
	// so the agent is the only thing the provider's operator can attribute it
	// by, and it is read on a schedule rather than once.
	req.Header.Set("User-Agent", outbound.KeySetHeader)
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if closeErr := resp.Body.Close(); closeErr != nil {
		return nil, time.Time{}, fmt.Errorf("jwks: close response body: %w", closeErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("jwks: status %d", resp.StatusCode)
	}
	if readErr != nil {
		return nil, time.Time{}, readErr
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, time.Time{}, err
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, time.Time{}, errors.New("jwks: no usable RSA keys")
	}
	return keys, v.now().Add(cacheTTL(resp.Header.Get("Cache-Control"))), nil
}

// cacheTTL reads max-age from a Cache-Control header, clamped to [1m, 24h]
// with a 1h default when absent — the JWKS is safe to reuse between rotations.
func cacheTTL(cacheControl string) time.Duration {
	ttl := time.Hour
	for _, part := range strings.Split(cacheControl, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "max-age="); ok {
			if secs, err := strconv.Atoi(v); err == nil {
				ttl = time.Duration(secs) * time.Second
			}
		}
	}
	if ttl < time.Minute {
		ttl = time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	return ttl
}

func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, errors.New("jwk: zero exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func verifyRS256(key *rsa.PublicKey, signingInput, sigB64 string) error {
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(signingInput))
	// RS256 signature VERIFICATION per RFC 7518 §3.3 — PKCS#1 v1.5 is the
	// algorithm Google signs these tokens with; nothing is encrypted here.
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sig) // NOSONAR(go:S5542) verification, not encryption
}

func decodeHeaderSegment(seg string) (oidcHeader, error) {
	var hdr oidcHeader
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return oidcHeader{}, err
	}
	if err := json.Unmarshal(b, &hdr); err != nil {
		return oidcHeader{}, err
	}
	return hdr, nil
}

func decodeClaimsSegment(seg string) (oidcClaims, error) {
	var claims oidcClaims
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return oidcClaims{}, err
	}
	if err := json.Unmarshal(b, &claims); err != nil {
		return oidcClaims{}, err
	}
	return claims, nil
}
