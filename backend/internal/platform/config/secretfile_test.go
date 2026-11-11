// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// ReadSecretFile and a ${file:} reference are one reader. Each case reads the
// same file both ways and must get the same value or the same refusal.
func TestReadSecretFileMatchesAFileReference(t *testing.T) {
	t.Parallel()
	for name, body := range map[string][]byte{
		"one trailing newline is dropped": []byte("s3cret\n"),
		"other whitespace is kept":        []byte("  s3cret \r\n\n"),
		"at the size limit":               bytes.Repeat([]byte("x"), config.SecretFileLimit),
		"over the size limit":             bytes.Repeat([]byte("x"), config.SecretFileLimit+1),
		"the absence marker is refused":   []byte("${none}\n"),
		"an empty file reads as empty":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			assertOneReader(t, path)
		})
	}
	t.Run("an unreadable file", func(t *testing.T) {
		t.Parallel()
		assertOneReader(t, filepath.Join(t.TempDir(), "missing"))
	})
}

// assertOneReader reads path through ReadSecretFile and through a ${file:}
// reference to relayPassword, a secret that accepts an empty value.
func assertOneReader(t *testing.T, path string) {
	t.Helper()
	direct, directErr := config.ReadSecretFile(path)
	cfg, refErr := load(t, fixture{env: []string{"TEST_RELAY_PASSWORD=${file:" + path + "}"}}, relayPassword)
	if (directErr == nil) != (refErr == nil) {
		t.Fatalf("ReadSecretFile err %v, ${file:} err %v; want both to refuse or both to read", directErr, refErr)
	}
	if directErr != nil {
		return
	}
	if ref := config.Val(cfg, relayPassword); ref != direct {
		t.Errorf("ReadSecretFile = %q, ${file:} = %q; want one value", direct, ref)
	}
}
