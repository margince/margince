// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestAnnounceNamesTheConfiguredAdmin holds the start message to the file.
//
// configuredAdminEmail's own tests pass just as well while announce carries a
// literal, so the address a reader is actually offered has to be asserted on
// where they read it.
func TestAnnounceNamesTheConfiguredAdmin(t *testing.T) {
	l := layout{root: t.TempDir()}
	const configured = "admin@demo.test"
	if err := os.WriteFile(l.configPath(),
		[]byte("bootstrap_admin:\n  email: "+configured+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	printed := capture(t, func() { announce("http://127.0.0.1:8800", l, "") })

	if !strings.Contains(printed, configured) {
		t.Fatalf("the start message never names the address the installation was bootstrapped\n"+
			"with, so a reader has nothing that works to type:\n%s", printed)
	}
	if strings.Contains(printed, defaultAdminEmail) {
		t.Fatalf("the start message still names %q alongside the configured address — the\n"+
			"literal is back, and a reader is offered an account that does not exist:\n%s",
			defaultAdminEmail, printed)
	}
}

// capture swaps os.Stdout for a pipe, because say() IS fmt.Printf on purpose
// (console.go) and the console deliberately has no seam to inject.
func capture(t *testing.T, run func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = write
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		//craft:ignore swallowed-errors a copy from a pipe this test owns; a short read shows up as a failed assertion on the text
		_, _ = io.Copy(&buf, read)
		done <- buf.String()
	}()

	run()

	os.Stdout = saved
	if err := write.Close(); err != nil {
		t.Fatalf("close the write end: %v", err)
	}
	return <-done
}
