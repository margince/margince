// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Client authentication at the token and revoke endpoints (RFC 6749 client
// password). Only a client an admin registered with a secret is confidential.
// Every DCR and CIMD client stays public, proven by PKCE alone.

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

const oauthParamClientSecret = "client_secret"

// errMalformedBasic is a Basic header that does not decode to id:secret.
var errMalformedBasic = errors.New("oauth: malformed basic credentials")

// PresentedClient is what a request offers as its client identity, before
// anything is checked. The Basic header and the form are kept apart: a
// confidential client may use one of the two, never both.
type PresentedClient struct {
	BasicID, BasicSecret string
	HasBasic             bool
	FormID, FormSecret   string
}

// ID is the client the request names: the form's client_id when it sends one,
// which is what a public client has always been identified by.
func (p PresentedClient) ID() string {
	if p.FormID != "" {
		return p.FormID
	}
	return p.BasicID
}

// ClientCredentials reads the client a token or revoke request presents. The
// limiter calls it with its own bounded peek of the form, so the two cannot
// name a different client for one request.
func ClientCredentials(header http.Header, form url.Values) (PresentedClient, error) {
	p := PresentedClient{FormID: form.Get(oauthParamClientID), FormSecret: form.Get(oauthParamClientSecret)}
	scheme, encoded, found := strings.Cut(header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return p, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return p, errMalformedBasic
	}
	id, secret, ok := strings.Cut(string(raw), ":")
	if !ok {
		return p, errMalformedBasic
	}
	// RFC 6749 form-encodes each half before joining them, so a secret holding
	// a colon or a plus sign survives the trip.
	if p.BasicID, err = url.QueryUnescape(id); err != nil {
		return p, errMalformedBasic
	}
	if p.BasicSecret, err = url.QueryUnescape(secret); err != nil {
		return p, errMalformedBasic
	}
	p.HasBasic = true
	return p, nil
}

// clientAuthRefusal is an RFC 6749 §5.2 answer the caller writes as it is.
type clientAuthRefusal struct {
	status            int
	code, description string
	basicChallenge    bool
}

func (r *clientAuthRefusal) write(w http.ResponseWriter) {
	if r.basicChallenge {
		w.Header().Set("WWW-Authenticate", `Basic realm="oauth"`)
	}
	oauthError(w, r.status, r.code, r.description)
}

var errClientSecret = &clientAuthRefusal{
	status: http.StatusUnauthorized, code: oauthErrInvalidClient,
	description: "client authentication failed",
}

// authenticateClient answers which client a token or revoke request is from,
// and whether it proved itself with a secret. A public client is returned
// unchecked, any secret it sent ignored. DCR and CIMD clients hold none, and a
// library that sends an empty one must keep working.
func (h Handlers) authenticateClient(ctx context.Context, header http.Header, form url.Values) (clientID string, confidential bool, refusal *clientAuthRefusal) {
	presented, err := ClientCredentials(header, form)
	if err != nil {
		return "", false, &clientAuthRefusal{
			status: http.StatusUnauthorized, code: oauthErrInvalidClient,
			description: "the Basic credentials do not decode to client_id:client_secret", basicChallenge: true,
		}
	}
	clientID = presented.ID()
	hash, err := h.clientSecretHash(ctx, clientID)
	if err != nil {
		return "", false, &clientAuthRefusal{status: http.StatusInternalServerError, code: "server_error", description: "the client could not be read"}
	}
	if hash == "" {
		return clientID, false, nil
	}
	return clientID, true, checkClientSecret(presented, clientID, hash)
}

func checkClientSecret(presented PresentedClient, clientID, hash string) *clientAuthRefusal {
	if presented.HasBasic && presented.FormSecret != "" {
		return &clientAuthRefusal{
			status: http.StatusBadRequest, code: oauthErrInvalidRequest,
			description: "send the client secret in the Basic header or in the form, not both",
		}
	}
	secret := presented.FormSecret
	if presented.HasBasic {
		if presented.BasicID != clientID {
			return &clientAuthRefusal{
				status: http.StatusUnauthorized, code: oauthErrInvalidClient,
				description: "the Basic client_id does not match the form's", basicChallenge: true,
			}
		}
		secret = presented.BasicSecret
	}
	if secret == "" || subtle.ConstantTimeCompare([]byte(hashToken(secret)), []byte(hash)) != 1 {
		refusal := *errClientSecret
		refusal.basicChallenge = presented.HasBasic
		return &refusal
	}
	return nil
}

// clientSecretHash is the stored digest of a client's secret, or "" for a
// public client and for one that does not exist. The grant checks that follow
// refuse an unknown client as they always have.
func (h Handlers) clientSecretHash(ctx context.Context, clientID string) (string, error) {
	if clientID == "" {
		return "", nil
	}
	var hash *string
	err := h.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT client_secret_hash FROM oauth_client WHERE client_id = $1`, clientID).
			Scan(&hash)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil || hash == nil {
		return "", err
	}
	return *hash, nil
}
