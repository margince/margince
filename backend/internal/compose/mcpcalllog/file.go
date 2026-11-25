// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mcpcalllog writes the mcp.call_log file: one JSON line for each /mcp
// call, secrets stripped.
package mcpcalllog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// File appends each /mcp call to a file as one JSON line. A file outlives the
// database restores the e2e lane runs between cases, which a table would not.
type File struct {
	mu   sync.Mutex
	file *os.File
	// strip is the pass AI payload capture runs, so a credential in an argument
	// or an answer is scrubbed here as it is there.
	strip model.SecretStripper
	log   *slog.Logger
}

// Open opens path for appending, creating it readable by its owner
// only, because its lines hold personal data.
func Open(path string, log *slog.Logger) (*File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600) // #nosec G304 G703 -- the path IS the operator's mcp.call_log setting
	if err != nil {
		return nil, fmt.Errorf("mcp.call_log: open %s: %w", path, err)
	}
	return &File{file: file, strip: ai.NewSecretStripper(), log: log}, nil
}

// Record writes one call. A record that cannot be written is logged and
// dropped: the log is evidence about calls, and never a reason to fail one.
func (f *File) Record(ctx context.Context, record agents.CallRecord) {
	line, err := json.Marshal(record)
	if err != nil {
		f.log.ErrorContext(ctx, "mcp call log: a record could not be encoded; it is dropped", "error", err)
		return
	}
	clean, _, err := f.strip.Strip(ctx, line)
	if err != nil {
		f.log.ErrorContext(ctx, "mcp call log: a record could not be stripped of secrets; it is dropped", "error", err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.file.Write(append(clean, '\n')); err != nil {
		f.log.ErrorContext(ctx, "mcp call log: a record could not be written; it is dropped", "error", err)
	}
}

// Close closes the file.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Close()
}
