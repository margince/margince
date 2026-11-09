// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pictureWire is the connect flow over a mailbox the seat owns, with Google's userinfo
// answering status once release is called.
func pictureWire(t *testing.T, status int) (chainWire, chan offeredPicture, func()) {
	t.Helper()
	c := setupChainWire(t)
	c.h.gmailAPI = mailboxOwnedBy{owner: c.env.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, c.env.Rep1)}
	held := make(chan struct{})
	userinfo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-held
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(`{"picture":"` + accountPicture + `"}`)); err != nil {
			t.Errorf("answering userinfo: %v", err)
		}
	}))
	release := sync.OnceFunc(func() { close(held) })
	t.Cleanup(userinfo.Close)
	t.Cleanup(release)
	offered := make(chan offeredPicture, 1)
	c.h.picture = newConnectPicture(identity.NewServiceFor(c.env.DB()), func(_ context.Context, user ids.UserID, pictureURL string) error {
		offered <- offeredPicture{user, pictureURL}
		return nil
	})
	c.h.picture.userinfo = googleconn.NewUserinfo(userinfo.Client(), userinfo.URL)
	return c, offered, release
}

// The landing is written while userinfo still holds its answer: the picture rides
// after the connect, never in front of it.
func TestAPersonalGoogleConnectLandsAndThenOffersItsPicture(t *testing.T) {
	c, offered, release := pictureWire(t, http.StatusOK)
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"settings"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/settings/connections/ok/gmail")
	release()

	got := awaitOffer(t, offered)
	if got.user.UUID != c.env.Rep1 || got.url != accountPicture {
		t.Errorf("offered %v at %q, want the connecting seat and its account's picture", got.user, got.url)
	}
}

func TestAFailingUserinfoStillLandsTheConnect(t *testing.T) {
	c, _, release := pictureWire(t, http.StatusInternalServerError)
	release()
	jar := browserJar{}

	state := c.connect(t, jar, `{"return_to":"settings"}`)
	requireLocation(t, c.callback(t, jar, "gmail", state, granted()), "https://app.test/#/settings/connections/ok/gmail")

	if n := c.env.WsCount(t, `SELECT count(*) FROM capture_connection
		WHERE user_id = $1 AND provider = 'gmail' AND status = 'connected'`, c.env.Rep1); n != 1 {
		t.Errorf("connected mailboxes after a failing userinfo = %d, want 1", n)
	}
}
