// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"strings"
	"testing"
	"testing/fstest"
)

// perCountryFS is a policy folder with packs aa and bb on one regional layer
// that states regional. Each country layer states its own front matter.
func perCountryFS(regional, aa, bb string) fstest.MapFS {
	fsys := fstest.MapFS{
		"regional/eu.md": {Data: []byte("---\n" + regional + pairKeys + "---\nWhat every member state shares.\n")},
		"fixtures/a.md":  {Data: []byte(draft)},
		"fixtures/b.md":  {Data: []byte(draft)},
	}
	for code, front := range map[string]string{"aa": aa, "bb": bb} {
		fsys[code+"/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/eu.md, " + code + "/" + code + ".md]\n---\nThe layers.\n")}
		fsys[code+"/"+code+".md"] = &fstest.MapFile{Data: []byte("---\njurisdiction: " + code + "\nversion: 1\ncreated: 2026-01-02\n" +
			front + "fixtures:\n  - finding: objection_route\n    triggers: " + code + "/fixtures/a.md\n    clears: fixtures/b.md\n" +
			"---\nWhy.\n")}
		fsys[code+"/fixtures/a.md"] = &fstest.MapFile{Data: []byte(draft)}
	}
	return fsys
}

func TestEveryCountryFolderCompilesOnTheSharedRegionalLayer(t *testing.T) {
	unit, err := CompileUnit(perCountryFS("reply-window: 720h\n", "consent-only: true\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !unit.PerCountry || len(unit.Policies) != 2 {
		t.Fatalf("compiled %d packs, per country %v; want two from the country folders", len(unit.Policies), unit.PerCountry)
	}
	aa, bb := unit.Policies[0], unit.Policies[1]
	if aa.Rules.Jurisdiction != "aa" || bb.Rules.Jurisdiction != "bb" {
		t.Errorf("packs %q and %q, want aa and bb", aa.Rules.Jurisdiction, bb.Rules.Jurisdiction)
	}
	if aa.Rules.ReplyWindow.Hours() != 720 || bb.Rules.ReplyWindow.Hours() != 720 {
		t.Errorf("the regional window did not reach both packs: %s, %s", aa.Rules.ReplyWindow, bb.Rules.ReplyWindow)
	}
	if !aa.Rules.ConsentOnly || bb.Rules.ConsentOnly {
		t.Errorf("one country's key reached the other: aa %v, bb %v", aa.Rules.ConsentOnly, bb.Rules.ConsentOnly)
	}
	if got := aa.Sources; strings.Join(got, " ") != "aa/index.md regional/eu.md aa/aa.md" {
		t.Errorf("aa compiled from %v", got)
	}
	if len(aa.Fixtures) != 2 || aa.Fixtures[1].Triggers.Path != "aa/fixtures/a.md" {
		t.Errorf("aa's fixtures are %+v; want the regional pair and its own", aa.Fixtures)
	}
	if aa.Rules.Digest == bb.Rules.Digest {
		t.Error("two packs with different sources share a digest")
	}
}

func TestASingleIndexStillCompilesAsOnePack(t *testing.T) {
	unit, err := CompileUnit(policyFS(country))
	if err != nil {
		t.Fatal(err)
	}
	if unit.PerCountry || len(unit.Policies) != 1 || unit.Policies[0].Rules.Jurisdiction != "aa" {
		t.Errorf("a folder with index.md compiled to %+v", unit)
	}
}

func TestACountryThatLoosensTheRegionalLayerIsRefused(t *testing.T) {
	_, err := CompileUnit(perCountryFS("reply-window: 720h\n", "reply-window: 8760h\n", ""))
	if err == nil || !strings.Contains(err.Error(), "aa/aa.md loosens ReplyWindow") {
		t.Errorf("a country layer with a longer window than the regional one compiled: %v", err)
	}
}

func TestAPerCountryFolderIsRefused(t *testing.T) {
	for name, change := range map[string]func(fstest.MapFS){
		"a country folder with no index": func(fsys fstest.MapFS) { delete(fsys, "bb/index.md") },
		"no country folder at all": func(fsys fstest.MapFS) {
			for file := range fsys {
				if strings.HasPrefix(file, "aa/") || strings.HasPrefix(file, "bb/") {
					delete(fsys, file)
				}
			}
		},
		"a regional layer no pack lists": func(fsys fstest.MapFS) { fsys["regional/nordic.md"] = fsys["regional/eu.md"] },
		"a stray file at the top":        func(fsys fstest.MapFS) { fsys["notes.md"] = fsys["regional/eu.md"] },
		"an unlisted file in a pack":     func(fsys fstest.MapFS) { fsys["aa/extra.md"] = fsys["aa/aa.md"] },
		"a country not named for its folder": func(fsys fstest.MapFS) {
			fsys["aa/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/eu.md, aa/bb.md]\n---\nThe layers.\n")}
			fsys["aa/bb.md"] = fsys["bb/bb.md"]
			delete(fsys, "aa/aa.md")
		},
		"a layer of another pack": func(fsys fstest.MapFS) {
			fsys["aa/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/eu.md, bb/bb.md]\n---\nThe layers.\n")}
		},
		"a fixture in another pack's folder": func(fsys fstest.MapFS) {
			fsys["aa/aa.md"].Data = []byte(strings.Replace(string(fsys["aa/aa.md"].Data), "aa/fixtures/a.md", "bb/fixtures/a.md", 1))
		},
		"a pack that leaves out the regional layer": func(fsys fstest.MapFS) {
			fsys["aa/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [aa/aa.md]\n---\nThe layers.\n")}
		},
		"a pack naming another regional layer": func(fsys fstest.MapFS) {
			fsys["regional/nordic.md"] = fsys["regional/eu.md"]
			fsys["aa/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/nordic.md, aa/aa.md]\n---\nThe layers.\n")}
		},
		"two packs listing the shared layers in other orders": func(fsys fstest.MapFS) {
			fsys["regional/nordic.md"] = fsys["regional/eu.md"]
			fsys["aa/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/eu.md, regional/nordic.md, aa/aa.md]\n---\nThe layers.\n")}
			fsys["bb/index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/nordic.md, regional/eu.md, bb/bb.md]\n---\nThe layers.\n")}
		},
	} {
		fsys := perCountryFS("", "", "")
		change(fsys)
		if _, err := CompileUnit(fsys); err == nil {
			t.Errorf("a policy folder with %s compiled", name)
		}
	}
	if _, err := CompileUnit(perCountryFS("", "", "")); err != nil {
		t.Errorf("the unchanged folder does not compile, so the cases above prove nothing: %v", err)
	}
}
