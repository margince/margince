// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The rejection's mode guard, as far as a unit test can reach it.
//
// What is NOT here is a case for refuseInOverlayMode itself: it is the shared
// helper, and nativeonlytools_test.go already holds both of its answers. A pair
// of copies keyed on this verb's name would prove the same two lines twice and
// go stale in the same breath.
//
// What is left is the wiring — that Server.RejectCompany runs the guard
// BEFORE the contacts transport sees the request — and that needs a Dispatcher
// this package cannot fake: `sorDispatch` is the concrete type, so every write
// shadow in this tree is in the same position. The wiring is covered where it
// can be, by compose/integration's own overlay case.

import (
	"os"
	"strings"
	"testing"
)

// The shadow calls the guard. Read from the SOURCE rather than served, because
// the alternative is a Dispatcher against a live database for a claim that is
// one line long — and a shadow silently losing its guard is wrong only in
// overlay mode, which is exactly where nobody looks.
func TestTheRejectShadowRunsTheModeGuardFirst(t *testing.T) {
	src := readComposeSource(t, "companyreject.go")
	_, shadow, found := strings.Cut(src, "func (s Server) RejectCompany(")
	if !found {
		t.Fatal("the shadow is gone from this file — the router then reaches the contacts transport directly, unguarded")
	}
	guard := strings.Index(shadow, "refuseInOverlayMode(")
	transport := strings.Index(shadow, "s.contactsHandlers.RejectCompany(")
	switch {
	case guard < 0:
		t.Fatal("the shadow does not run the mode guard — in overlay mode it reaches a native store " +
			"holding none of this workspace's records, and answers not-found about a company the reader is looking at")
	case transport < 0:
		t.Fatal("the shadow does not reach the contacts transport, so the verb is unserved")
	case guard > transport:
		t.Fatal("the shadow guards AFTER delegating, which is not a guard")
	}
}

// readComposeSource reads one file of this package, so a case can assert about
// wiring a fake cannot reach.
func readComposeSource(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(src)
}
