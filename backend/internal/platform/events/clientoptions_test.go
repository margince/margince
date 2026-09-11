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

import (
	"context"
	"strings"
	"testing"
)

func TestABareAddressKeepsTheDefaultDatabase(t *testing.T) {
	opts, err := ClientOptions("localhost:16379", "")
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
	opts, err := ClientOptions("localhost:16379/7", "")
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
		if _, err := ClientOptions(addr, ""); err == nil {
			t.Errorf("%q was accepted; a bad index must refuse rather than fall back to db 0, "+
				"because falling back shares a consumer group with every other stack", addr)
		}
	}
}

// A UNIX SOCKET is an address, not a suffixed host. go-redis reads a leading
// slash as one, and every path has slashes in it — splitting those would
// refuse a deployment that worked before this parameter existed.
func TestAUnixSocketIsAnAddressNotADatabaseSuffix(t *testing.T) {
	opts, err := ClientOptions("/var/run/redis.sock", "")
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
	opts, err := ClientOptions("localhost:16379/79", "")
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
	opts, err := ClientOptions("localhost:16379/0", "")
	if err != nil {
		t.Fatalf("an explicit db 0 was refused: %v", err)
	}
	if opts.DB != 0 {
		t.Errorf("db = %d, want 0", opts.DB)
	}
}

// The credential reaches the options on every address shape.
//
// The bus carries job payloads and therefore CRM data, so a client that
// connected without it would not fail — it would connect unauthenticated
// wherever the instance allows, and read the stream. That failure is silent by
// construction: everything works, and a second local account can MONITOR.
//
// Every shape, because the parser has three returns and a credential dropped on
// one of them is the same hole reached by an address nobody tested with.
func TestTheBusCredentialReachesEveryAddressShape(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t"
	for _, addr := range []string{
		"localhost:16379",     // bare host
		"localhost:16379/7",   // host with a logical database
		"/var/run/redis.sock", // unix socket
	} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			opts, err := ClientOptions(addr, secret)
			if err != nil {
				t.Fatalf("ClientOptions(%q): %v", addr, err)
			}
			if opts.Password != secret {
				t.Errorf("Password = %q, want the credential — this client would connect "+
					"unauthenticated and read the stream", opts.Password)
			}
		})
	}
}

// An instance that requires no credential takes none. Empty is the ordinary
// case rather than a fallback: go-redis sends no AUTH for it, which is the same
// connection every deployment already makes.
func TestNoCredentialLeavesThePasswordEmpty(t *testing.T) {
	t.Parallel()
	opts, err := ClientOptions("localhost:16379/7", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Password != "" {
		t.Errorf("Password = %q, want empty", opts.Password)
	}
}

// NewClient reads the address before it reaches for the network, so an address
// nobody can parse is refused as a configuration error rather than reported as
// an unreachable bus. The two read the same to an operator and are fixed in
// different places.
func TestAnUnparseableAddressIsRefusedBeforeDialling(t *testing.T) {
	t.Parallel()
	// A context already cancelled: a dial attempted despite the parse failure
	// would fail on it rather than on a timeout, so this test cannot hang and
	// cannot reach a bus that happens to be listening.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewClient(ctx, "localhost:16379/notanumber", "secret", false)
	if err == nil {
		t.Fatal("an address naming a logical database that is not a number was accepted")
	}
	if !strings.Contains(err.Error(), "not an integer") {
		t.Errorf("NewClient reported %q, want the parse complaint — an operator told the bus is "+
			"unreachable looks at the bus, and the address is what is wrong", err)
	}
}
