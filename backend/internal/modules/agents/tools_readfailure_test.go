// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestAReadFailureKeepsTheRoutedArchiveInterfaces(t *testing.T) {
	ctx := context.Background()
	refused := errors.New("fixture read refusal")
	provider := &narrowArchiver{types: threeTypes()}
	registry := NewRegistry(nil, nil, WithReadRecordFailure(func(context.Context, datasource.EntityRef) error { return refused }))
	RegisterCoreTools(registry, provider, nil, nil, nil, nil, nil)
	args := archiveArgsJSON(t, "contact", ids.NewV7())
	if _, err := registry.tools["read_record"].Handle(ctx, args); !errors.Is(err, refused) {
		t.Fatalf("read_record bypassed the configured refusal: %v", err)
	}
	archive, ok := registry.tools["archive_record"].(archiveRecord)
	if !ok {
		t.Fatal("archive_record is not the core archive implementation")
	}
	if _, err := archive.StageInfo(ctx, args); err != nil {
		t.Fatalf("read-only fault changed archive staging: %v", err)
	}
	if archive.p != provider {
		t.Fatal("archive lost the original provider and its optional write interfaces")
	}
}
