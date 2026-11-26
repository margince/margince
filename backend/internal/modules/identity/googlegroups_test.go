// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// testDirectoryCredential is a credential over a freshly generated key, for a
// service account that exists nowhere.
func testDirectoryCredential(t *testing.T) directoryCredential {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	file, err := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "reader@example-project.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return directoryCredential{ServiceAccountKey: string(file), AdminSubject: "admin@example.test"}
}

// fakeGoogle serves the token endpoint and the Directory API, recording what
// each was asked. groupsStatus and pages decide the Directory API's answer.
type fakeGoogle struct {
	mu           sync.Mutex
	assertions   []map[string]any
	userKeys     []string
	tokenStatus  int
	groupsStatus int
	// known, when set, is who the Directory answers for. Anyone else is a 404.
	known []string
	pages [][]string
	hang  bool
	srv   *httptest.Server
}

func newFakeGoogle(t *testing.T, pages ...[]string) *fakeGoogle {
	t.Helper()
	f := &fakeGoogle{tokenStatus: http.StatusOK, groupsStatus: http.StatusOK, pages: pages}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", f.token)
	mux.HandleFunc("/groups", f.groups)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGoogle) reader(t *testing.T) *GoogleGroupsReader {
	t.Helper()
	return &GoogleGroupsReader{
		http: f.srv.Client(), now: func() time.Time { return clocktest.Now(t) }, timeout: 2 * time.Second,
		tokenURL: f.srv.URL + "/token", apiBase: f.srv.URL,
	}
}

func (f *fakeGoogle) token(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	parts := strings.Split(r.PostForm.Get("assertion"), ".")
	if len(parts) == 3 {
		raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims map[string]any
		if json.Unmarshal(raw, &claims) == nil {
			f.assertions = append(f.assertions, claims)
		}
	}
	if f.tokenStatus != http.StatusOK {
		w.WriteHeader(f.tokenStatus)
		_, _ = w.Write([]byte(`{"error":"unauthorized_client"}`))
		return
	}
	_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3599}`))
}

func (f *fakeGoogle) groups(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.userKeys = append(f.userKeys, r.URL.Query().Get("userKey"))
	status, hang, pages := f.groupsStatus, f.hang, f.pages
	if len(f.known) > 0 && !slices.Contains(f.known, r.URL.Query().Get("userKey")) {
		status = http.StatusNotFound
	}
	f.mu.Unlock()
	if hang {
		<-r.Context().Done()
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	index := 0
	if token := r.URL.Query().Get("pageToken"); token != "" {
		index = 1
	}
	body := map[string]any{}
	emails := make([]map[string]string, 0)
	for _, e := range pages[index] {
		emails = append(emails, map[string]string{"email": e})
	}
	body["groups"] = emails
	if index+1 < len(pages) {
		body["nextPageToken"] = "next"
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func TestReaderSignsForTheAdminWithTheOneReadOnlyScopeAndPagesThroughTheAnswer(t *testing.T) {
	google := newFakeGoogle(t, []string{"sales@example.test"}, []string{"ops@example.test"})
	cred := testDirectoryCredential(t)

	got, err := google.reader(t).Groups(context.Background(), cred, "member@example.test")
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if want := []string{"sales@example.test", "ops@example.test"}; !slices.Equal(got, want) {
		t.Fatalf("groups = %v, want both pages %v", got, want)
	}
	if len(google.assertions) != 1 {
		t.Fatalf("token requests = %d, want one for the whole lookup", len(google.assertions))
	}
	claims := google.assertions[0]
	if claims["sub"] != "admin@example.test" || claims["scope"] != directoryGroupScope {
		t.Fatalf("assertion subject=%v scope=%v, want the admin and the one read-only scope", claims["sub"], claims["scope"])
	}
	if !slices.Equal(google.userKeys, []string{"member@example.test", "member@example.test"}) {
		t.Fatalf("userKey per page = %v, want the member on both", google.userKeys)
	}
}

func TestReaderClassesEveryFailureAndReturnsNoGroups(t *testing.T) {
	cases := []struct {
		name   string
		arm    func(*fakeGoogle)
		reason DirectoryFailure
	}{
		{"delegation refused at the token endpoint", func(f *fakeGoogle) { f.tokenStatus = http.StatusBadRequest }, DirectoryDelegationRefused},
		{"directory forbids the read", func(f *fakeGoogle) { f.groupsStatus = http.StatusForbidden }, DirectoryPermissionDenied},
		{"directory does not know the member", func(f *fakeGoogle) { f.groupsStatus = http.StatusNotFound }, DirectoryNotFound},
		{"directory fails", func(f *fakeGoogle) { f.groupsStatus = http.StatusBadGateway }, DirectoryUnavailable},
		{"directory is throttling", func(f *fakeGoogle) { f.groupsStatus = http.StatusTooManyRequests }, DirectoryUnavailable},
		{"directory is too slow", func(f *fakeGoogle) { f.hang = true }, DirectoryTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			google := newFakeGoogle(t, []string{"sales@example.test"})
			tc.arm(google)
			reader := google.reader(t)
			if tc.reason == DirectoryTimeout {
				reader.timeout = 100 * time.Millisecond
			}

			groups, err := reader.Groups(context.Background(), testDirectoryCredential(t), "member@example.test")
			var failed DirectoryError
			if !errors.As(err, &failed) || failed.Reason != tc.reason {
				t.Fatalf("err = %v, want reason %q", err, tc.reason)
			}
			if len(groups) != 0 {
				t.Fatalf("a failed lookup returned groups %v", groups)
			}
		})
	}
}

func TestReaderRefusesAKeyItCannotSignWithoutCallingGoogle(t *testing.T) {
	google := newFakeGoogle(t)
	cred := directoryCredential{ServiceAccountKey: `{"type":"service_account"}`, AdminSubject: "admin@example.test"}
	_, err := google.reader(t).Groups(context.Background(), cred, "member@example.test")
	var failed DirectoryError
	if !errors.As(err, &failed) || failed.Reason != DirectoryKeyInvalid {
		t.Fatalf("err = %v, want key_invalid", err)
	}
	if len(google.assertions) != 0 {
		t.Fatal("a key that cannot sign must not reach Google")
	}
}

func TestDirectoryCredentialNeedsAnAdminAddress(t *testing.T) {
	cred := testDirectoryCredential(t)
	for _, subject := range []string{"", "not an address", "Admin <admin@example.test>"} {
		cred.AdminSubject = subject
		if err := cred.validate(); err == nil {
			t.Fatalf("admin_subject %q was accepted", subject)
		}
	}
}
