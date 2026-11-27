// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

// The reference host is copied here so a source installation builds without
// fetching the checker. Its result types follow the bundled module's output.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// Grants is the set of attributes a product grant carries, decoded from the
// module's output. JSON numbers decode to float64.
type Grants = map[string]any

// Result is what the module reports: which license the installation holds, and
// what the requested product grants. The two are separate objects on the wire,
// so a product attribute can never collide with a metadata field.
type Result struct {
	Grants  Grants  `json:"grants"`
	License License `json:"license"`
}

// License is the metadata of the license the module verified.
//
// Company, ContactName and ContactEmail are empty for a license issued before those
// claims existed. A zero IssuedAt or NotBefore means the token carried no such
// claim. InGrace reports a license past its expiry that the grace period still
// accepts: the check passes today and will stop passing.
// GraceUntil comes from the module so callers need no copy of its grace period.
type License struct {
	IssuedAt     time.Time `json:"issued_at"`
	NotBefore    time.Time `json:"not_before"`
	Expiry       time.Time `json:"expiry"`
	GraceUntil   time.Time `json:"grace_until"`
	ID           string    `json:"id"`
	Subject      string    `json:"subject"`
	Company      string    `json:"org,omitempty"`
	ContactName  string    `json:"name,omitempty"`
	ContactEmail string    `json:"email,omitempty"`
	KeyID        string    `json:"key_id"`
	InGrace      bool      `json:"in_grace"`
}

// ErrVerdict marks an error as the module's JUDGMENT about a license — an
// untrusted signature, the wrong issuer, expiry past grace, no grant for this
// product — as opposed to a failure to run the module at all. Only run() wraps
// it, and Resolve is what reads it.
var ErrVerdict = errors.New("license rejected")

// check runs the module (compiled wasm bytes, gzipped or raw) to validate token
// for product at generation, issued by issuer. It returns the license the module
// verified and the granted attributes, or an error when the module rejects the
// license or fails to run. The token is passed to the module as the
// MARGINCE_LICENSE environment variable; issuer, product and generation are
// passed as arguments.
//
// The parameters this repository only ever passes constants for stay parameters:
// the signature is upstream's, and narrowing it here would make the copy harder
// to compare against the original than the unused generality costs. The tests
// pass the same constants for the same reason — what varies is the token.
//
//nolint:unparam // upstream's signature, kept comparable; see the vendoring note above
func check(ctx context.Context, module []byte, issuer, product string, generation int, token string) (Result, error) {
	module, err := maybeDecompress(module)
	if err != nil {
		return Result{}, err
	}
	out, err := run(ctx, module, issuer, product, generation, token)
	if err != nil {
		return Result{}, err
	}
	return decodeResult(out)
}

// wasmMagic starts every WebAssembly module: "\0asm".
var wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d}

// maybeDecompress returns the module bytes, decompressing them when the consumer
// embedded a compressed module — about a fifth of the size — and passed it
// straight to check. The published artifact is brotli, which has no magic
// number, so the module magic decides: bytes that already start with "\0asm" are
// a raw module, gzip is recognized by its own magic, and everything else is read
// as brotli.
func maybeDecompress(module []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(module, wasmMagic):
		return module, nil
	case len(module) >= 2 && module[0] == 0x1f && module[1] == 0x8b:
		reader, err := gzip.NewReader(bytes.NewReader(module))
		if err != nil {
			return nil, fmt.Errorf("gunzip module: %w", err)
		}
		//craft:ignore swallowed-errors a read-only gzip reader over a byte slice holds no resource whose close can fail meaningfully
		defer func() { _ = reader.Close() }()
		return readAll(reader, "gunzip module")
	default:
		return readAll(brotli.NewReader(bytes.NewReader(module)), "decompress module")
	}
}

func readAll(reader io.Reader, what string) ([]byte, error) {
	out, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return out, nil
}

// run instantiates the module and returns its stdout. A non-zero exit is the
// module's way of rejecting a license; run turns it into an error carrying the
// module's stderr.
//
// The run ends with ctx, so a hung module cannot outlive its caller's budget.
func run(ctx context.Context, module []byte, issuer, product string, generation int, token string) ([]byte, error) {
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	//craft:ignore swallowed-errors the runtime is torn down after its single-shot verdict is already in hand; a close failure cannot change it
	defer func() { _ = runtime.Close(ctx) }()
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	var stdout, stderr bytes.Buffer
	config := wazero.NewModuleConfig().
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithEnv("MARGINCE_LICENSE", token).
		WithArgs("module", issuer, product, strconv.Itoa(generation)).
		// The module checks the license against the current time (expiry, grace,
		// not-before). Without the real clock wazero uses a fake one fixed at the
		// epoch, so every unexpired license would read as "used before issued".
		WithSysWalltime().
		WithSysNanotime()

	// A Go wasip1 module that exits 0 returns a nil error; a non-zero exit
	// returns an ExitError (the module rejecting the license); any other error is
	// a run failure (a malformed module or a trap).
	_, err := runtime.InstantiateWithConfig(ctx, module, config)
	if err == nil {
		return stdout.Bytes(), nil
	}
	if exit, ok := errors.AsType[*sys.ExitError](err); ok {
		if stoppedByContext(ctx, exit) {
			return nil, fmt.Errorf("run module: stopped before the module answered: %w", exit)
		}
		return nil, fmt.Errorf("%w: %s", ErrVerdict, strings.TrimSpace(stderr.String()))
	}
	return nil, fmt.Errorf("run module: %w", err)
}

// stoppedByContext reports wazero's own stop on context end, which arrives as an
// ExitError too. It said nothing about the license, so it must not read as a
// refusal. A module that exited by itself keeps its verdict, even past the
// deadline, and the context check stops a module from forging the stop code.
func stoppedByContext(ctx context.Context, exit *sys.ExitError) bool {
	code := exit.ExitCode()
	return ctx.Err() != nil && (code == sys.ExitCodeDeadlineExceeded || code == sys.ExitCodeContextCanceled)
}

// decodeResult parses the module's JSON output.
func decodeResult(out []byte) (Result, error) {
	var result Result
	if err := json.Unmarshal(out, &result); err != nil {
		return Result{}, fmt.Errorf("decode result: %w", err)
	}
	return result, nil
}
