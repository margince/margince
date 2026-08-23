// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// The bus address may name a logical database, and getting that wrong is
// silent in the direction that hurts.
//
// One Redis serves every dev stack on a machine, and the stream names and
// consumer groups are constants. Two stacks on one index share a consumer
// group, so whichever worker reads an entry first consumes it, resolves it
// against its OWN Postgres, finds nothing, and acks — the other stack's event
// is gone and its projection simply never runs. An address that quietly fell
// back to db 0 would restore exactly that, while looking configured.

import "testing"

func TestABareAddressKeepsTheDefaultDatabase(t *testing.T) {
	opts, err := ClientOptions("localhost:16379")
	if err != nil {
		t.Fatalf("a bare address was refused: %v", err)
	}
	if opts.Addr != "localhost:16379" {
		t.Errorf("addr = %q, want it carried through unchanged", opts.Addr)
	}
	// Zero is Redis's own default, and bare `make dev` relies on it so every
	// existing redis-cli recipe still points at the same place.
	if opts.DB != 0 {
		t.Errorf("db = %d, want 0 for an address that names none", opts.DB)
	}
}

func TestASuffixSelectsThatLogicalDatabase(t *testing.T) {
	opts, err := ClientOptions("localhost:16379/7")
	if err != nil {
		t.Fatalf("a suffixed address was refused: %v", err)
	}
	if opts.Addr != "localhost:16379" {
		t.Errorf("addr = %q, want the suffix stripped from the host", opts.Addr)
	}
	if opts.DB != 7 {
		t.Errorf("db = %d, want 7", opts.DB)
	}
}

// The refusals. Each one would otherwise land on db 0 and share a bus with
// every other stack — the failure this parameter exists to prevent, reached
// by way of a typo.
func TestAnUnusableDatabaseIsRefusedRatherThanIgnored(t *testing.T) {
	for _, addr := range []string{
		"localhost:16379/",    // suffix present, index missing
		"localhost:16379/x",   // not a number
		"localhost:16379/-1",  // below the range
		"localhost:16379/80",  // Redis ships `databases 16`, so 0..15
		"localhost:16379/1/2", // two suffixes
	} {
		if _, err := ClientOptions(addr); err == nil {
			t.Errorf("%q was accepted; a bad index must refuse rather than fall back to db 0, "+
				"because falling back shares a consumer group with every other stack", addr)
		}
	}
}

// A UNIX SOCKET is an address, not a suffixed host. go-redis reads a leading
// slash as one, and every path has slashes in it — splitting those would
// refuse a deployment that worked before this parameter existed.
func TestAUnixSocketIsAnAddressNotADatabaseSuffix(t *testing.T) {
	opts, err := ClientOptions("/var/run/redis.sock")
	if err != nil {
		t.Fatalf("a unix socket was refused: %v", err)
	}
	if opts.Network != "unix" {
		t.Errorf("network = %q, want unix", opts.Network)
	}
	if opts.Addr != "/var/run/redis.sock" {
		t.Errorf("addr = %q, want the path carried through whole", opts.Addr)
	}
	if opts.DB != 0 {
		t.Errorf("db = %d, want 0 — a socket path names no database", opts.DB)
	}
}

// The top of the range the dev compose actually serves.
func TestTheHighestServedDatabaseIsAccepted(t *testing.T) {
	opts, err := ClientOptions("localhost:16379/79")
	if err != nil {
		t.Fatalf("db 79 was refused, but the instance serves 80: %v", err)
	}
	if opts.DB != 79 {
		t.Errorf("db = %d, want 79", opts.DB)
	}
}

// db 0 stays reachable by name: bare `make dev` uses it, and an explicit "/0"
// is how a caller says so rather than a value to reject.
func TestZeroIsAValidIndex(t *testing.T) {
	opts, err := ClientOptions("localhost:16379/0")
	if err != nil {
		t.Fatalf("an explicit db 0 was refused: %v", err)
	}
	if opts.DB != 0 {
		t.Errorf("db = %d, want 0", opts.DB)
	}
}
