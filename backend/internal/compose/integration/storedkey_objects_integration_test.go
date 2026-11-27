// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every owner of object-store keys names the installation's stored-key id, the
// one every object already stored was written under. These cases write or read
// through the owner and count what sits under the legacy prefix. A key that
// moved to another id would show as an object outside it.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func legacyPrefix(e *Env, kind string) string {
	return blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), kind, "")
}

func countUnder(t *testing.T, ctx context.Context, blob blobstore.Store, prefix string) int {
	t.Helper()
	n, err := blob.DeletePrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("counting the objects under %q: %v", prefix, err)
	}
	return n
}

func TestAnAttachmentUploadLandsUnderTheLegacyPrefix(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	contact := e.SeedContact(t, "Legacy Prefix", &e.Rep1)
	_, err := e.Activities.WithBlobstore(blob).UploadAttachment(e.Admin(), activities.AttachmentInput{
		EntityType: "contact", EntityID: contact, Filename: "a.pdf", Content: bytes.NewReader([]byte("%PDF-1.4 legacy")),
	})
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if got := countUnder(t, e.Admin(), blob, legacyPrefix(e, "attachment")); got != 1 {
		t.Errorf("%d objects under the legacy attachment prefix, want the one just uploaded", got)
	}
}

func TestACapturedFileWhoseBytesAnAttachmentHoldsIsNotStoredAgain(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	store := e.Activities.WithBlobstore(blob)
	contact := e.SeedContact(t, "Reused Copy", &e.Rep1)
	body := []byte("%PDF-1.4 the same bytes")
	if _, err := store.UploadAttachment(e.Admin(), activities.AttachmentInput{
		EntityType: "contact", EntityID: contact, Filename: "a.pdf", Content: bytes.NewReader(body),
	}); err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if _, err := store.StageCapturedFiles(e.Admin(), []activities.CapturedFile{{
		PartID: "p1", Filename: "copy.pdf", ContentType: "application/pdf", Body: body,
	}}); err != nil {
		t.Fatalf("StageCapturedFiles: %v", err)
	}
	if got := countUnder(t, e.Admin(), blob, legacyPrefix(e, "attachment")); got != 1 {
		t.Errorf("%d objects under the legacy prefix, want 1: the capture must find the copy stored under the id every earlier object used", got)
	}
}

func TestAKnowledgeDocumentLandsUnderTheLegacyPrefix(t *testing.T) {
	ie := newIngestEnv(t)
	ie.upload(t, "page.md", "text/markdown", "# Page")
	if got := countUnder(t, ie.ctx, ie.blob, legacyPrefix(ie.env, "knowledge")); got != 1 {
		t.Errorf("%d objects under the legacy knowledge prefix, want the one just uploaded", got)
	}
}

func TestACompanyLogoKeyIsMintedUnderTheLegacyPrefix(t *testing.T) {
	e := Setup(t)
	base, err := e.Contacts.NewLogoBase(e.Admin(), "legal-entity/"+ids.NewV7().String())
	if err != nil {
		t.Fatalf("NewLogoBase: %v", err)
	}
	if !strings.HasPrefix(base, legacyPrefix(e, contacts.LogoKeyKind)+"legal-entity/") {
		t.Errorf("logo base %q is not under %q", base, legacyPrefix(e, contacts.LogoKeyKind))
	}
}

func TestAnExtensionObjectStoredUnderTheLegacyPrefixIsStillRead(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	ctx := e.Admin()
	ws := ids.From[ids.WorkspaceKind](e.WS)
	if err := blob.Put(ctx, extcapability.Prefix(ws, "summaries")+"report", strings.NewReader("kept"), 4, "text/plain"); err != nil {
		t.Fatal(err)
	}
	objects := extcapability.Objects{
		Store: blob, Ledger: extcapability.PoolLedger(e.Pool), Unit: "summaries",
		Gate:      func(ctx context.Context) (context.Context, error) { return ctx, nil },
		Workspace: extcapability.StoredWorkspace(e.Pool),
	}
	got, err := objects.Get(ctx, "report")
	if err != nil || string(got.Content) != "kept" {
		t.Fatalf("Get = %q, %v; want the object written under the legacy prefix", got.Content, err)
	}
}
