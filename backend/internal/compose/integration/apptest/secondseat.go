// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
)

// AsSecondSeat is e signed in as another active seat with password. It has its
// own client and owner connection, so it can work beside e at the same time.
func (e *AppEnv) AsSecondSeat(t *testing.T, email, password string) *AppEnv {
	t.Helper()
	base := e.Client.Transport
	if seat, isSeat := base.(forwardedFor); isSeat {
		base = seat.next
	}
	transport, ok := base.(*http.Transport)
	jar, err := cookiejar.New(nil)
	if !ok || err != nil {
		t.Fatalf("the harness client cannot be copied for a second seat: %T, %v", base, err)
	}
	seat := *e
	own := transport.Clone()
	t.Cleanup(own.CloseIdleConnections)
	seat.Client = &http.Client{Transport: forwardedFor{next: own, addr: nextSeatAddr()}, Jar: jar}
	seat.Owner = connectOwner(t, os.Getenv("MARGINCE_TEST_DSN"))
	if status := seat.Call(t, "POST", "/v1/auth/login", map[string]any{"email": email, "password": password}, nil, nil); status != http.StatusOK {
		t.Fatalf("signing in as %s → %d", email, status)
	}
	return &seat
}

// seatAddrs numbers the second seats, so each comes from its own address.
var seatAddrs atomic.Uint32

// nextSeatAddr is an address in the benchmarking range no real client has.
func nextSeatAddr() string {
	n := seatAddrs.Add(1)
	return fmt.Sprintf("198.18.%d.%d", n/250, 1+n%250)
}

// forwardedFor sends each request as from addr, through a proxy the server
// trusts. A server that trusts no proxy reads the peer and ignores it. Each
// seat then has its own per-address sign-in budget, as each user would.
type forwardedFor struct {
	next http.RoundTripper
	addr string
}

func (f forwardedFor) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Forwarded-For", f.addr)
	return f.next.RoundTrip(r)
}

// connectOwner opens an owner connection that closes with t. A pgx.Conn runs
// one statement at a time, so each seat that seeds beside another needs one.
func connectOwner(t *testing.T, ownerDSN string) *pgx.Conn {
	t.Helper()
	owner, err := pgx.Connect(context.Background(), ownerDSN)
	if err != nil {
		t.Fatalf("connecting as owner: %v", err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	return owner
}
