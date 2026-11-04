// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

func TestVersionAnswersWithoutConfiguration(t *testing.T) {
	var out bytes.Buffer
	if err := start(context.Background(), []string{"--version"}, &out, io.Discard); err != nil {
		t.Fatalf("--version: %v", err)
	}
	if got, want := out.String(), buildinfo.VersionLine("margince-migrate")+"\n"; got != want {
		t.Errorf("--version printed %q, want %q", got, want)
	}
}
