// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

var testNow = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

func hmacOf(secret string, message []byte) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(message)
	return h.Sum(nil)
}

func serverArrival(secret, key string, at time.Time, nonce string, body []byte) Arrival {
	payload := extension.SigningPayload(extension.ScopeInbound, SigningSlug, key, at, nonce, body)
	h := http.Header{}
	h.Set(extension.InboundHeaderTimestamp, strconv.FormatInt(at.Unix(), 10))
	h.Set(extension.InboundHeaderNonce, nonce)
	h.Set(extension.InboundHeaderSignature, "sha256="+hex.EncodeToString(hmacOf(secret, payload)))
	return Arrival{FormKey: key, Header: h, Body: body, Now: testNow}
}

func TestAServerPostSignedWithTheFormsSecretIsAdmittedAndKeyedByItsNonce(t *testing.T) {
	a := serverArrival("mwf_secret", "formkey0123456789", testNow, "a1b2c3", []byte(`{"email":"a@b.example"}`))
	nonce, err := verifyServer([]byte("mwf_secret"), a)
	if err != nil || nonce != "a1b2c3" {
		t.Fatalf("verifyServer = %q, %v; want the nonce and no error", nonce, err)
	}
}

func TestAServerSignatureIsRefusedWhenAnythingItCoversChanged(t *testing.T) {
	body := []byte(`{"email":"a@b.example"}`)
	cases := map[string]Arrival{
		"another secret":     serverArrival("someone-else", "formkey0123456789", testNow, "a1b2c3", body),
		"another form":       withKey(serverArrival("mwf_secret", "otherformkey12345", testNow, "a1b2c3", body), "formkey0123456789"),
		"an edited body":     withBody(serverArrival("mwf_secret", "formkey0123456789", testNow, "a1b2c3", body), []byte(`{"email":"x@y.example"}`)),
		"a stale timestamp":  serverArrival("mwf_secret", "formkey0123456789", testNow.Add(-6*time.Minute), "a1b2c3", body),
		"a future timestamp": serverArrival("mwf_secret", "formkey0123456789", testNow.Add(6*time.Minute), "a1b2c3", body),
		"a non-hex nonce":    serverArrival("mwf_secret", "formkey0123456789", testNow, "not-hex\n", body),
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyServer([]byte("mwf_secret"), a); err != errUnverified {
				t.Fatalf("verifyServer = %v, want errUnverified", err)
			}
		})
	}
}

func withKey(a Arrival, key string) Arrival { a.FormKey = key; return a }

func withBody(a Arrival, body []byte) Arrival { a.Body = body; return a }

func TestEachBuildersOwnSignatureIsVerified(t *testing.T) {
	body := []byte(`{"event_id":"e1"}`)
	ms := strconv.FormatInt(testNow.UnixMilli(), 10)
	webflow := http.Header{}
	webflow.Set(headerWebflowTimestamp, ms)
	webflow.Set(headerWebflowSignature, hex.EncodeToString(hmacOf("wf", append([]byte(ms+":"), body...))))
	typeform := http.Header{}
	typeform.Set(headerTypeformSignature, "sha256="+base64.StdEncoding.EncodeToString(hmacOf("tf", body)))
	tally := http.Header{}
	tally.Set(headerTallySignature, base64.StdEncoding.EncodeToString(hmacOf("ty", body)))

	for _, c := range []struct {
		sender, secret string
		header         http.Header
	}{{SenderWebflow, "wf", webflow}, {SenderTypeform, "tf", typeform}, {SenderTally, "ty", tally}} {
		t.Run(c.sender, func(t *testing.T) {
			a := Arrival{Header: c.header, Body: body, Now: testNow}
			if err := verifyBuilder(c.sender, []byte(c.secret), a); err != nil {
				t.Fatalf("a correctly signed %s post was refused: %v", c.sender, err)
			}
			if err := verifyBuilder(c.sender, []byte("wrong"), a); err != errUnverified {
				t.Fatalf("a %s post under another secret = %v, want errUnverified", c.sender, err)
			}
			a.Body = []byte(`{"event_id":"e2"}`)
			if err := verifyBuilder(c.sender, []byte(c.secret), a); err != errUnverified {
				t.Fatalf("an edited %s body = %v, want errUnverified", c.sender, err)
			}
		})
	}
}

func TestAWebflowSignatureOutsideTheWindowIsRefused(t *testing.T) {
	body := []byte(`{}`)
	ms := strconv.FormatInt(testNow.Add(-10*time.Minute).UnixMilli(), 10)
	h := http.Header{}
	h.Set(headerWebflowTimestamp, ms)
	h.Set(headerWebflowSignature, hex.EncodeToString(hmacOf("wf", append([]byte(ms+":"), body...))))
	if err := verifyBuilder(SenderWebflow, []byte("wf"), Arrival{Header: h, Body: body, Now: testNow}); err != errUnverified {
		t.Fatalf("a ten-minute-old Webflow signature = %v, want errUnverified", err)
	}
}

func TestABrowserPostIsAdmittedOnlyFromTheFormsWebsites(t *testing.T) {
	f := Form{Sender: SenderBrowser, AllowedOrigins: []string{"https://www.example.com"}}
	cases := []struct {
		name, origin, referer string
		admitted              bool
	}{
		{"its own website", "https://www.example.com", "", true},
		{"its own website in capitals", "https://WWW.example.com", "", true},
		{"no Origin, its own page as Referer", "", "https://www.example.com/contact?x=1", true},
		{"another website", "https://evil.example", "", false},
		{"a look-alike host", "https://www.example.com.evil.example", "", false},
		{"plain http", "http://www.example.com", "", false},
		{"an opaque origin", "null", "https://www.example.com/", false},
		{"neither header", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			if c.origin != "" {
				h.Set("Origin", c.origin)
			}
			if c.referer != "" {
				h.Set("Referer", c.referer)
			}
			err := admitBrowser(f, Arrival{Header: h})
			if (err == nil) != c.admitted {
				t.Fatalf("admitBrowser = %v, want admitted=%v", err, c.admitted)
			}
		})
	}
}

func TestTheSpamChecksDropAFilledHoneypotAndAnInstantSubmit(t *testing.T) {
	rendered := func(ago time.Duration) string { return strconv.FormatInt(testNow.Add(-ago).UnixMilli(), 10) }
	cases := []struct {
		name     string
		controls map[string]string
		dropped  bool
	}{
		{"a human", map[string]string{controlRendered: rendered(20 * time.Second)}, false},
		{"no snippet controls at all", map[string]string{}, false},
		{"a filled honeypot", map[string]string{controlHoneypot: "http://spam.example"}, true},
		{"submitted a second after rendering", map[string]string{controlRendered: rendered(time.Second)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksAutomated(Enquiry{controls: c.controls}, testNow); got != c.dropped {
				t.Fatalf("looksAutomated = %v, want %v", got, c.dropped)
			}
		})
	}
}

func TestAnOriginIsStoredAsTheBrowserSendsIt(t *testing.T) {
	got, err := normalizeOrigins([]string{"https://Example.com/contact", "https://example.com", "http://localhost:3000/"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://example.com", "http://localhost:3000"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("normalizeOrigins = %v, want %v", got, want)
	}
	for _, bad := range []string{"http://example.com", "example.com", "https://user:pw@example.com", "javascript:alert(1)"} {
		if _, err := normalizeOrigins([]string{bad}); err == nil {
			t.Errorf("normalizeOrigins accepted %q", bad)
		}
	}
}

func TestASourceKeyFitsTheColumnWhateverTheSenderSent(t *testing.T) {
	long := make([]byte, 4096)
	for i := range long {
		long[i] = 'f'
	}
	key := sourceKey(testFormID, string(long))
	if len(key) > 128 {
		t.Fatalf("sourceKey is %d characters, over source_id's 128", len(key))
	}
	if sourceKey(testFormID, "a") == sourceKey(testFormID, "b") {
		t.Fatal("two submissions share a source key")
	}
}
