// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

//go:embed module/licensecheck.wasm.module.sha256
var recordedDigest string

// The bundled module is a binary nobody reviews by reading it, fetched from a
// release this repository cannot rebuild. What CAN be reviewed is the digest
// beside it, so the two are held together here: a blob that was swapped,
// truncated, or edited on one side alone fails the build gate rather than booting
// and quietly trusting a different keyset than the one that was reviewed.
//
// module/VERSION is held to the blob by TestModuleVersionWasWrittenForTheBundledModule.
func TestBundledModuleMatchesItsRecordedDigest(t *testing.T) {
	t.Parallel()
	want, name, ok := strings.Cut(strings.TrimSpace(recordedDigest), " ")
	if !ok {
		t.Fatalf("module/licensecheck.wasm.module.sha256 is not in `shasum -a 256` format: %q", recordedDigest)
	}
	// The digest names the UPSTREAM asset, which is how the tree records which
	// artifact was fetched once the bundled file name stopped saying so.
	if !strings.HasPrefix(strings.TrimSpace(name), "licensecheck.wasm.") {
		t.Errorf("the digest file names %q, not a published licensecheck module", strings.TrimSpace(name))
	}
	sum := sha256.Sum256(bundledModule)
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("the bundled module hashes to %s but module/licensecheck.wasm.module.sha256 records %s —\n"+
			"re-install the pair with the publisher's tooling, which rewrites the blob, "+
			"the pin and this digest together; editing either file so they agree is not the fix", got, want)
	}
}

// What the blob has to BE, whatever framing upstream published it in: something
// the host can unwrap into a WebAssembly module. Asserted through the real
// maybeDecompress rather than by sniffing a magic number here, so this keeps
// holding across a compression change instead of pinning the tree to gzip.
func TestBundledModuleUnwrapsToAWebAssemblyModule(t *testing.T) {
	t.Parallel()
	raw, err := maybeDecompress(bundledModule)
	if err != nil {
		t.Fatalf("unwrap the bundled module: %v", err)
	}
	// The wasm preamble: \0asm plus the version word.
	if len(raw) < 8 || !bytes.HasPrefix(raw, wasmMagic) {
		t.Fatal("the unwrapped module does not begin with the WebAssembly preamble")
	}
	// A compressed artifact is the point of bundling one: the raw module is
	// roughly five times the size, and it is embedded in every binary.
	if len(bundledModule) >= len(raw) {
		t.Errorf("the bundled module is %d bytes and unwraps to %d — it is not the compressed artifact",
			len(bundledModule), len(raw))
	}
}

// VERSION records the digest of the blob its tag was written for. A refresh that
// leaves VERSION alone would make every posture name the wrong release, so it
// fails the build gate here. The tree cannot prove the tag is right, because the
// blob carries no release of its own. This only forces whoever refreshes the
// pair to touch the line that holds the tag.
func TestModuleVersionWasWrittenForTheBundledModule(t *testing.T) {
	t.Parallel()
	_, forDigest := parseModuleVersion(moduleVersion)
	sum := sha256.Sum256(bundledModule)
	got := hex.EncodeToString(sum[:])
	if forDigest != got {
		t.Errorf("module/VERSION says its tag was written for blob %q but the bundled module hashes to %s: "+
			"the blob changed and VERSION did not. Confirm the release tag of the artifact you fetched, "+
			"then write both lines of VERSION (the tag, then `for <sha256 of the blob>`)", forDigest, got)
	}
	recorded, _, _ := strings.Cut(strings.TrimSpace(recordedDigest), " ")
	if forDigest != recorded {
		t.Errorf("module/VERSION is written for %q but module/licensecheck.wasm.module.sha256 records %q",
			forDigest, recorded)
	}
}

func TestParseModuleVersionReadsTagAndDigest(t *testing.T) {
	t.Parallel()
	tag, digest := parseModuleVersion("sha-abc123def456\nfor 00ff\n")
	if tag != "sha-abc123def456" || digest != "00ff" {
		t.Errorf("parsed (%q, %q), want (sha-abc123def456, 00ff)", tag, digest)
	}
	tag, digest = parseModuleVersion("sha-abc123def456\n")
	if tag != "sha-abc123def456" || digest != "" {
		t.Errorf("one-line file parsed (%q, %q), want the tag and no digest", tag, digest)
	}
}

// The recorded release tag reaches every posture the process reports, so it has
// to be the upstream tag and not a hand-written note.
func TestModuleVersionIsAnUpstreamReleaseTag(t *testing.T) {
	t.Parallel()
	got := ModuleVersion()
	if !regexp.MustCompile(`^sha-[0-9a-f]{12,}$`).MatchString(got) {
		t.Errorf("ModuleVersion() = %q, want an upstream `sha-<commit>` release tag: the rolling `latest` tag "+
			"names a different build every day and could not identify the bundled one", got)
	}
}
