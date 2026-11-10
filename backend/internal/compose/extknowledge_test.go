// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

func knowledgeRuntime(unit string, deps extensionRuntimeBinding) *callRuntime {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return runtimeFor(ctx, unit, "1.0.0", "job/probe", deps)
}

// A unit the core keeps no corpus for is told so, by name, whatever it sends.
func TestAUnitWithNoCorpusCannotFileADocument(t *testing.T) {
	rt := knowledgeRuntime("openchannel", extensionRuntimeBinding{pool: &pgxpool.Pool{}, knowledgeBlob: blobstore.NewMemory()})
	_, err := rt.Knowledge().UpsertDocument(context.Background(), extension.KnowledgeDocument{ExternalID: "x"})
	if !errors.Is(err, extension.ErrNoKnowledge) {
		t.Fatalf("got %v, want ErrNoKnowledge", err)
	}
	if err := rt.Knowledge().DeleteDocument(context.Background(), "x"); !errors.Is(err, extension.ErrNoKnowledge) {
		t.Fatalf("delete: got %v, want ErrNoKnowledge", err)
	}
}

// A role that bound no object store refuses by name instead of dereferencing nil.
func TestFilingWithNoObjectStoreBoundRefusesByName(t *testing.T) {
	rt := knowledgeRuntime("gdocs", extensionRuntimeBinding{pool: &pgxpool.Pool{}})
	_, err := rt.Knowledge().UpsertDocument(context.Background(), extension.KnowledgeDocument{ExternalID: "x"})
	if !errors.Is(err, errKnowledgeUnwired) {
		t.Fatalf("got %v, want errKnowledgeUnwired", err)
	}
}

// A Runtime released with its call answers ErrRuntimeExpired, like every capability.
func TestAReleasedRuntimeCannotFileADocument(t *testing.T) {
	rt := knowledgeRuntime("gdocs", extensionRuntimeBinding{})
	rt.release()
	_, err := rt.Knowledge().UpsertDocument(context.Background(), extension.KnowledgeDocument{ExternalID: "x"})
	if !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Fatalf("got %v, want ErrRuntimeExpired", err)
	}
}

// The store's refusals reach the unit in the published vocabulary.
func TestTheStoresRefusalsReachTheUnitInPublishedWords(t *testing.T) {
	for name, tc := range map[string]struct {
		in   error
		want error
	}{
		"busy":        {&knowledge.IngestBusyError{Filename: "a"}, extension.ErrKnowledgeBusy},
		"unsupported": {&knowledge.UnsupportedTypeError{Got: "application/pdf"}, extension.ErrInvalid},
		"duplicate":   {&knowledge.AlreadyFiledError{}, extension.ErrInvalid},
		"full":        {&knowledge.CorpusFullError{Have: 1, Adding: 2, Ceiling: 2}, extension.ErrInvalid},
	} {
		if got := knowledgeRefusal(tc.in); !errors.Is(got, tc.want) {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
	if knowledgeRefusal(nil) != nil {
		t.Error("nil must stay nil")
	}
}
