// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

func TestVersionAnswersWithoutConfiguration(t *testing.T) {
	var out bytes.Buffer
	if err := start(context.Background(), []string{"--version"}, &out); err != nil {
		t.Fatalf("--version: %v", err)
	}
	if got, want := out.String(), buildinfo.VersionLine("margince-worker")+"\n"; got != want {
		t.Errorf("--version printed %q, want %q", got, want)
	}
}
