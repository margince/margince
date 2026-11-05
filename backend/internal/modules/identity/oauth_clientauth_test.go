// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func basicHeader(id, secret string) http.Header {
	h := http.Header{}
	raw := url.QueryEscape(id) + ":" + url.QueryEscape(secret)
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(raw)))
	return h
}

func TestClientCredentialsReadTheFormWhenThereIsNoBasicHeader(t *testing.T) {
	got, err := ClientCredentials(http.Header{}, url.Values{"client_id": {"c1"}, "client_secret": {"s1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.HasBasic || got.ID() != "c1" || got.FormSecret != "s1" {
		t.Errorf("got %+v, want the form's client c1 with secret s1 and no Basic", got)
	}
}

// RFC 6749 §2.3.1 form-encodes each half before joining them, so a secret with a
// colon or a plus sign must come back as it was minted.
func TestClientCredentialsDecodeTheFormEncodedBasicHalves(t *testing.T) {
	got, err := ClientCredentials(basicHeader("gemini client", "a:b+c"), url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasBasic || got.BasicID != "gemini client" || got.BasicSecret != "a:b+c" || got.ID() != "gemini client" {
		t.Errorf("got %+v, want the Basic id and secret decoded", got)
	}
}

func TestClientCredentialsIgnoreABearerHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer abc")
	got, err := ClientCredentials(h, url.Values{"client_id": {"c1"}})
	if err != nil || got.HasBasic {
		t.Errorf("got %+v, %v; a bearer header is not client authentication", got, err)
	}
}

func TestClientCredentialsRefuseAMalformedBasicHeader(t *testing.T) {
	for name, value := range map[string]string{
		"not base64":   "Basic !!!",
		"no colon":     "Basic " + base64.StdEncoding.EncodeToString([]byte("justanid")),
		"bad escaping": "Basic " + base64.StdEncoding.EncodeToString([]byte("id:%zz")),
		"bad id":       "Basic " + base64.StdEncoding.EncodeToString([]byte("%zz:secret")),
	} {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			h.Set("Authorization", value)
			if _, err := ClientCredentials(h, url.Values{}); err == nil {
				t.Errorf("%q decoded; it must be refused", value)
			}
		})
	}
}

func TestTheFormClientIDNamesTheClientOverTheBasicOne(t *testing.T) {
	got, err := ClientCredentials(basicHeader("from-basic", ""), url.Values{"client_id": {"from-form"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != "from-form" {
		t.Errorf("ID() = %q; a public client has always been named by its form client_id", got.ID())
	}
}

func TestAConfidentialClientPassesWithItsSecretInEitherPlace(t *testing.T) {
	hash := hashToken("the-secret")
	form := PresentedClient{FormID: "c1", FormSecret: "the-secret"}
	if refusal := checkClientSecret(form, "c1", hash); refusal != nil {
		t.Errorf("form secret refused: %+v", refusal)
	}
	basic := PresentedClient{HasBasic: true, BasicID: "c1", BasicSecret: "the-secret", FormID: "c1"}
	if refusal := checkClientSecret(basic, "c1", hash); refusal != nil {
		t.Errorf("Basic secret with an equal form client_id refused: %+v", refusal)
	}
}

func TestAConfidentialClientIsRefusedWithoutItsSecret(t *testing.T) {
	hash := hashToken("the-secret")
	for name, tc := range map[string]struct {
		presented PresentedClient
		status    int
		code      string
		challenge bool
	}{
		"no secret":    {PresentedClient{FormID: "c1"}, http.StatusUnauthorized, oauthErrInvalidClient, false},
		"wrong form":   {PresentedClient{FormID: "c1", FormSecret: "nope"}, http.StatusUnauthorized, oauthErrInvalidClient, false},
		"wrong basic":  {PresentedClient{HasBasic: true, BasicID: "c1", BasicSecret: "nope"}, http.StatusUnauthorized, oauthErrInvalidClient, true},
		"two methods":  {PresentedClient{HasBasic: true, BasicID: "c1", BasicSecret: "the-secret", FormSecret: "the-secret"}, http.StatusBadRequest, "invalid_request", false},
		"basic for c2": {PresentedClient{HasBasic: true, BasicID: "c2", BasicSecret: "the-secret", FormID: "c1"}, http.StatusUnauthorized, oauthErrInvalidClient, true},
	} {
		t.Run(name, func(t *testing.T) {
			refusal := checkClientSecret(tc.presented, "c1", hash)
			if refusal == nil {
				t.Fatal("accepted; it must be refused")
			}
			if refusal.status != tc.status || refusal.code != tc.code || refusal.basicChallenge != tc.challenge {
				t.Errorf("got %d %s challenge=%v, want %d %s challenge=%v",
					refusal.status, refusal.code, refusal.basicChallenge, tc.status, tc.code, tc.challenge)
			}
		})
	}
}

func TestARefusalOfBasicCredentialsAsksForBasic(t *testing.T) {
	w := httptest.NewRecorder()
	(&clientAuthRefusal{status: http.StatusUnauthorized, code: oauthErrInvalidClient, description: "x", basicChallenge: true}).write(w)
	if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("got %d with WWW-Authenticate %q, want 401 asking for Basic", w.Code, w.Header().Get("WWW-Authenticate"))
	}
}

// Discovery must keep none first. Claude, Codex and ChatGPT choose a metadata
// document only when it is offered; without it they fall back to
// self-registration.
func TestDiscoveryOffersNoneFirst(t *testing.T) {
	if oauthAuthMethodsSupported[0] != oauthAuthMethodNone {
		t.Errorf("auth methods = %v, want none first", oauthAuthMethodsSupported)
	}
}
