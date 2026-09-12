// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// The bounce sink stops the address it just marked dead.
//
// comms.Store takes its BounceObserver as an OPTION, because a store with no
// observer behaves exactly as it did before the seam existed — which is right
// for the dozen read-only constructions and every test store, and dangerous for
// the ONE construction that receives delivery reports.
//
// The failure is silent in both directions. A sink built without the observer
// marks bounces and stops nothing, so the address stays dead on the record and
// live to the send path. Nothing errors, no test fails, and the only way to
// notice is to send to a dead mailbox and watch it go out.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheBounceSinkCarriesTheObserverThatStopsTheAddress holds the claim in
// capturebounce.go's newBounceSink doc comment.
//
// It reads the one construction that matters rather than every comms.NewStore
// call, because the others are deliberately observer-free: binding a
// suppression writer to a read-only store would be a capability nothing uses
// and everything has to reason about.
func TestTheBounceSinkCarriesTheObserverThatStopsTheAddress(t *testing.T) {
	t.Parallel()
	path := filepath.Join(repoRoot, "backend", "internal", "compose", "capturebounce.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("capturebounce.go no longer exists — this gate names the file that wires "+
			"delivery reports to the ledger, point it at what replaced it: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "WithBounceObserver(") {
		t.Error("newBounceSink builds the comms store without WithBounceObserver. The sink " +
			"that receives delivery reports would then MARK a bounce and stop nothing: the " +
			"address stays dead on the record and live to the send path, with nothing " +
			"failing anywhere to say so.")
	}
	if !strings.Contains(body, "RecordHardBounceTx") {
		t.Error("the observer no longer reaches consent.RecordHardBounceTx, which is the " +
			"only writer of a hard_bounce suppression. The engine can refuse on one " +
			"(consent/authorizetransmitrecord.go maps it to ReasonHardBounce) and without " +
			"this writer that refusal is unreachable — a rule that reads as though the " +
			"product handles dead addresses while it does not.")
	}
}

// TestOnlyOneWriterMakesABounceStop is the other half.
//
// A second writer would be free to disagree about the scope, the authority
// level or the idempotence — and the one that matters most is scope: an
// address-scoped stop silences one dead mailbox, while a contact-scoped one
// would silence every address that contact has, so correcting a typo in one
// would leave the others stopped.
func TestOnlyOneWriterMakesABounceStop(t *testing.T) {
	t.Parallel()
	var writers []string
	for _, dir := range []string{
		filepath.Join(repoRoot, "backend", "internal", "modules"),
		filepath.Join(repoRoot, "backend", "internal", "compose"),
	} {
		found, err := filesInserting(dir, "'hard_bounce'")
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
		writers = append(writers, found...)
	}
	if len(writers) != 1 {
		t.Errorf("%d file(s) insert a hard_bounce suppression, want exactly one "+
			"(consent/bouncesuppress.go):\n\t%s\n\nTwo writers are free to disagree about "+
			"the scope, the authority level and the idempotence. Scope is the one that "+
			"bites: address-scoped stops one dead mailbox, contact-scoped would stop every "+
			"address that contact has.",
			len(writers), strings.Join(writers, "\n\t"))
	}
}

// filesInserting answers which non-test Go files under dir contain an INSERT
// into communication_suppression naming the given literal.
func filesInserting(dir, literal string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(src)
		if !strings.Contains(body, "INSERT INTO communication_suppression") {
			return nil
		}
		if !strings.Contains(body, literal) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	return out, err
}
