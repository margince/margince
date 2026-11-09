// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"path"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// linkDigests sets ViewDigests to a manifest of the given documents, keyed by
// file name the way the web build writes it.
func linkDigests(t *testing.T, docs map[string]string) {
	t.Helper()
	byFile := map[string]string{}
	for uri, doc := range docs {
		sum := sha256.Sum256([]byte(doc))
		byFile[path.Base(uri)] = hex.EncodeToString(sum[:])
	}
	manifest, err := json.Marshal(byFile)
	if err != nil {
		t.Fatal(err)
	}
	linkManifest(t, string(manifest))
}

// linkManifest sets ViewDigests for one test. Empty is a binary linked with none.
func linkManifest(t *testing.T, manifest string) {
	t.Helper()
	linked := ViewDigests
	ViewDigests = manifest
	t.Cleanup(func() { ViewDigests = linked })
}

// newPinnedWebTier is newWebTier linked with the digests of the documents the
// stand-in web tier serves.
func newPinnedWebTier(t *testing.T) (*webTier, *Provider) {
	t.Helper()
	return newWebTierLinkedWith(t, func() {
		docs := map[string]string{}
		for _, v := range catalog {
			docs[v.uri] = documentFor(v.uri)
		}
		linkDigests(t, docs)
	})
}

// stampRelease sets the release this test binary claims to be.
func stampRelease(t *testing.T, release string) {
	t.Helper()
	stamped := buildinfo.ReleaseVersion
	buildinfo.ReleaseVersion = release
	t.Cleanup(func() { buildinfo.ReleaseVersion = stamped })
}

func TestADocumentWithItsBuildDigestIsHeld(t *testing.T) {
	_, p := newPinnedWebTier(t)
	if err := p.Prime(t.Context()); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if p.heldCount() != len(catalog) {
		t.Fatalf("held %d of %d views whose bytes match the build", p.heldCount(), len(catalog))
	}
}

// One byte is enough: the comparison is over the whole document.
func TestADocumentWithOneChangedByteIsRefusedCountedAndLogged(t *testing.T) {
	tier, p := newPinnedWebTier(t)
	var logged strings.Builder
	p.log = slog.New(slog.NewTextHandler(&logged, nil))
	changed := strings.Replace(documentFor(CreateFollowupsURI), "<html", "<HTML", 1)
	tier.answer(CreateFollowupsURI, ok(changed))

	if err := p.Prime(t.Context()); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if p.Holds(CreateFollowupsURI) {
		t.Fatal("a document that is not the one this build produced is being served")
	}
	if !p.Holds(ApprovalURI) {
		t.Error("one refused document took a matching neighbour down")
	}
	if p.digestMismatches.Load() == 0 {
		t.Error("the refusal did not raise the digest counter")
	}
	if !strings.Contains(logged.String(), "want_sha256=") || !strings.Contains(logged.String(), "got_sha256=") {
		t.Errorf("the refusal was not logged with both digests:\n%s", logged.String())
	}
}

func TestARefreshThatFindsChangedBytesKeepsTheLastGoodCopy(t *testing.T) {
	tier, p := newPinnedWebTier(t)
	if err := p.Prime(t.Context()); err != nil {
		t.Fatalf("priming: %v", err)
	}
	before, _ := p.served(CreateFollowupsURI)
	tier.answer(CreateFollowupsURI, ok(documentFor(CreateFollowupsURI)+"<!-- changed -->"))
	p.Refresh(t.Context())
	after, holding := p.served(CreateFollowupsURI)
	if !holding || after != before {
		t.Fatal("a document that does not match the build replaced the last good copy")
	}
	if p.digestMismatches.Load() == 0 {
		t.Error("the refused refresh did not raise the digest counter")
	}
}

// A view the manifest does not name is refused, not waved through.
func TestAViewTheBuildPinnedNoDigestForIsRefused(t *testing.T) {
	_, p := newWebTierLinkedWith(t, func() {
		docs := map[string]string{}
		for _, v := range catalog {
			if v.uri != secondViewURI {
				docs[v.uri] = documentFor(v.uri)
			}
		}
		linkDigests(t, docs)
	})
	if err := p.Prime(t.Context()); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if p.Holds(secondViewURI) {
		t.Fatal("a view with no digest in the manifest is being served")
	}
	if !p.Holds(CreateFollowupsURI) {
		t.Error("a pinned view was refused alongside the unpinned one")
	}
}

func TestAReleaseLinkedWithoutAManifestServesNoView(t *testing.T) {
	stampRelease(t, "v1.2.0")
	_, p := newWebTierLinkedWith(t, func() { linkManifest(t, "") })
	var logged strings.Builder
	p.log = slog.New(slog.NewTextHandler(&logged, nil))
	if err := p.Prime(t.Context()); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if p.heldCount() != 0 {
		t.Fatalf("a release with no digest manifest serves %d views", p.heldCount())
	}
	if !strings.Contains(logged.String(), "level=ERROR") || !strings.Contains(logged.String(), "cannot check view digests") {
		t.Errorf("the boot did not say why no view is served:\n%s", logged.String())
	}
}

func TestADevelopmentBuildWithoutAManifestServesUnpinnedAndWarns(t *testing.T) {
	stampRelease(t, buildinfo.Unknown)
	_, p := newWebTierLinkedWith(t, func() { linkManifest(t, "") })
	var logged strings.Builder
	p.log = slog.New(slog.NewTextHandler(&logged, nil))
	if err := p.Prime(t.Context()); err != nil {
		t.Fatalf("priming: %v", err)
	}
	if p.heldCount() != len(catalog) {
		t.Fatalf("a development build served %d of %d views", p.heldCount(), len(catalog))
	}
	if !strings.Contains(logged.String(), "served unpinned") {
		t.Errorf("the boot did not warn that views are unpinned:\n%s", logged.String())
	}
}

func TestAManifestThatDoesNotParseRefusesEveryView(t *testing.T) {
	if err := pinsFrom("{not json", buildinfo.Unknown).check(CreateFollowupsURI, cleanDocument); err == nil {
		t.Fatal("a broken manifest let a view through")
	}
}
