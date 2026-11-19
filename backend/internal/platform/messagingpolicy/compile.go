// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// IndexFile names the layers of a policy folder, most general first.
const IndexFile = "index.md"

// Policy is one pack's compiled policy.
type Policy struct {
	// Rules is what the pack declares, its Digest stamped from Sources.
	Rules messaging.Rules
	// Sources are the files Rules was compiled from, index first.
	Sources []string
	// Fixtures are the paired drafts every layer names.
	Fixtures []FixturePair
}

// FixturePair is a draft that must raise Finding and one that must not.
type FixturePair struct {
	// Layer is the policy file that names the pair.
	Layer    string
	Finding  string
	Triggers Fixture
	Clears   Fixture
}

// Fixture is one draft and the send it is part of.
type Fixture struct {
	Path         string
	Subject      string
	Body         string
	Advertising  bool
	Sender       string
	Language     string
	MailLanguage string
	Particulars  Particulars
	Translations []Particulars
}

// layerClass orders the layers. A later class may only tighten an earlier one.
type layerClass int

const (
	fallbackLayer layerClass = iota
	regionalLayer
	countryLayer
	overlayLayer
)

// classOf answers which layer a policy file is, from where it sits. Names are
// relative to the policy folder. pack is the folder of the pack being compiled,
// "." when the policy folder is the one pack. fallback.md and regional/ sit at
// the top and may be shared by every pack; the country and overlay layers sit
// in the pack's own folder, and a pack folder holds the country named for it.
func classOf(pack, name string) (layerClass, error) {
	if top, file := path.Split(name); name == "fallback.md" || (top == "regional/" && strings.HasSuffix(file, ".md")) {
		if top == "" {
			return fallbackLayer, nil
		}
		return regionalLayer, nil
	}
	rest, inPack := name, true
	if pack != "." {
		rest, inPack = strings.CutPrefix(name, pack+"/")
	}
	folder, file := path.Split(rest)
	switch {
	case inPack && folder == "overlay/" && strings.HasSuffix(file, ".md"):
		return overlayLayer, nil
	case inPack && folder == "" && file != IndexFile && strings.HasSuffix(file, ".md") && (pack == "." || file == pack+".md"):
		return countryLayer, nil
	}
	if pack == "." {
		return 0, fmt.Errorf("%s is not a policy layer: name fallback.md, regional/*.md, <country>.md or overlay/*.md", name)
	}
	return 0, fmt.Errorf("%s is not a layer of pack %s: name fallback.md, regional/*.md, %s/%s.md or %s/overlay/*.md",
		name, pack, pack, pack, pack)
}

// Compile reads the policy folder fsys, which holds one pack, and answers what
// it declares. Every failure names the file and the key it is about.
func Compile(fsys fs.FS) (Policy, error) {
	return compilePack(fsys, ".")
}

// compilePack compiles the pack whose index sits in the folder pack.
func compilePack(fsys fs.FS, pack string) (Policy, error) {
	layers, err := readIndex(fsys, pack)
	if err != nil {
		return Policy{}, err
	}
	out := Policy{Sources: []string{path.Join(pack, IndexFile)}}
	var provenances []messaging.Provenance
	for i, name := range layers {
		layer, body, err := readLayer(fsys, name)
		if err != nil {
			return Policy{}, err
		}
		if strings.TrimSpace(string(body)) == "" {
			return Policy{}, fmt.Errorf("%s: the body explains the layer to its reviewer, and it is empty", name)
		}
		below := out.Rules
		if err := layer.apply(name, &out.Rules); err != nil {
			return Policy{}, err
		}
		if loosened := messagingrules.Loosened(below, out.Rules); i > 0 && len(loosened) > 0 {
			return Policy{}, fmt.Errorf("%s loosens %s of the layer below it; a later layer may only tighten",
				name, strings.Join(loosened, ", "))
		}
		if err := mandatory(pack, name, layer); err != nil {
			return Policy{}, err
		}
		pairs, err := readPairs(fsys, pack, name, layer.Fixtures)
		if err != nil {
			return Policy{}, err
		}
		out.Fixtures = append(out.Fixtures, pairs...)
		out.Sources = append(out.Sources, name)
		provenances = append(provenances, layer.provenance())
	}
	out.Rules.Provenance = weakest(provenances)
	if out.Rules.Digest, err = digest(fsys, out.Sources); err != nil {
		return Policy{}, err
	}
	if err := out.Rules.Validate(); err != nil {
		return Policy{}, fmt.Errorf("the compiled policy: %w", err)
	}
	return out, nil
}

// readIndex answers the layers the pack's index.md lists, checked for order
// and kind.
func readIndex(fsys fs.FS, pack string) ([]string, error) {
	indexPath := path.Join(pack, IndexFile)
	raw, err := fs.ReadFile(fsys, indexPath)
	if err != nil {
		return nil, fmt.Errorf("a policy folder has an %s: %w", indexPath, err)
	}
	front, body, err := splitFrontMatter(indexPath, raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil, fmt.Errorf("%s: the body explains the layers to their reviewer, and it is empty", indexPath)
	}
	var index indexFile
	if err := decodeStrict(indexPath, front, &index); err != nil {
		return nil, err
	}
	countries, last := 0, fallbackLayer
	for i, name := range index.Layers {
		if slices.Contains(index.Layers[:i], name) {
			return nil, fmt.Errorf("%s lists %s twice", indexPath, name)
		}
		class, err := classOf(pack, name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", indexPath, err)
		}
		if class < last {
			return nil, fmt.Errorf("%s: %s comes after a narrower layer; list fallback, regional, country, overlay in that order",
				indexPath, name)
		}
		last = class
		if class == countryLayer {
			countries++
		}
	}
	if countries != 1 {
		return nil, fmt.Errorf("%s lists %d country layers; a pack has exactly one", indexPath, countries)
	}
	if err := unlisted(fsys, pack, index.Layers); err != nil {
		return nil, err
	}
	return index.Layers, nil
}

// unlisted refuses a file in the pack's folder that its index does not list.
// It would compile into nothing and stay out of the digest, while a reader
// takes it for policy. fixtures/ holds drafts and research/ holds notes the
// compiler never reads.
func unlisted(fsys fs.FS, pack string, layers []string) error {
	indexPath := path.Join(pack, IndexFile)
	return fs.WalkDir(fsys, pack, func(name string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir() && (name == path.Join(pack, "fixtures") || name == path.Join(pack, "research")):
			return fs.SkipDir
		case entry.IsDir() || name == indexPath || !strings.HasSuffix(name, ".md"):
			return nil
		case !slices.Contains(layers, name):
			return fmt.Errorf("%s is in the policy folder and %s does not list it; list it or move it to research/",
				name, indexPath)
		}
		return nil
	})
}

func readLayer(fsys fs.FS, name string) (layerFile, []byte, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return layerFile{}, nil, fmt.Errorf("%s lists %s: %w", IndexFile, name, err)
	}
	front, body, err := splitFrontMatter(name, raw)
	if err != nil {
		return layerFile{}, nil, err
	}
	var layer layerFile
	if err := decodeStrict(name, front, &layer); err != nil {
		return layerFile{}, nil, err
	}
	return layer, body, nil
}

// mandatory refuses a layer that leaves out what a reviewer needs. The country
// layer names its jurisdiction, version and creation date. Every layer names at
// least one fixture pair.
func mandatory(pack, name string, layer layerFile) error {
	if len(layer.Fixtures) == 0 {
		return fmt.Errorf("%s names no fixtures; pair a draft that raises a finding with one that does not", name)
	}
	class, err := classOf(pack, name)
	if err != nil || class != countryLayer {
		return err
	}
	var missing []string
	if layer.Jurisdiction == nil {
		missing = append(missing, "jurisdiction")
	} else if *layer.Jurisdiction != strings.TrimSuffix(path.Base(name), ".md") {
		return fmt.Errorf("%s states jurisdiction %q; the country layer is named for its jurisdiction", name, *layer.Jurisdiction)
	}
	if layer.Version == nil {
		missing = append(missing, "version")
	}
	if layer.Created == nil {
		missing = append(missing, "created")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s leaves out %s", name, strings.Join(missing, ", "))
	}
	return nil
}

// weakest is the provenance of the whole compiled set: as reviewed as its
// least-reviewed layer, and due as soon as its soonest one.
func weakest(layers []messaging.Provenance) messaging.Provenance {
	if len(layers) == 0 {
		return messaging.Provenance{}
	}
	out := layers[0]
	var reviewers []string
	for _, p := range layers {
		out.CreatedAt = earliest(out.CreatedAt, p.CreatedAt, false)
		out.SourceCheckedAt = earliest(out.SourceCheckedAt, p.SourceCheckedAt, true)
		out.ReviewedAt = earliest(out.ReviewedAt, p.ReviewedAt, true)
		out.ReviewDue = earliest(out.ReviewDue, p.ReviewDue, false)
		if p.Reviewer != "" && !slices.Contains(reviewers, p.Reviewer) {
			reviewers = append(reviewers, p.Reviewer)
		}
	}
	out.Reviewer = strings.Join(reviewers, ", ")
	if out.ReviewedAt.IsZero() {
		out.Reviewer = ""
	}
	return out
}

// digest is the SHA-256 of the sources, each framed by its name and length so
// moving text from one file to the next changes it.
func digest(fsys fs.FS, sources []string) (string, error) {
	sum := sha256.New()
	for _, name := range sources {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", fmt.Errorf("digest: %w", err)
		}
		// hash.Hash.Write never returns an error.
		sum.Write(fmt.Appendf(nil, "%s\x00%d\x00", name, len(raw)))
		sum.Write(raw)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// readPairs reads the fixtures a layer names. Both drafts of a pair must
// exist, sit under the top fixtures/ or the pack's own, and differ.
func readPairs(fsys fs.FS, pack, layer string, pairs []fixturePair) ([]FixturePair, error) {
	out := make([]FixturePair, 0, len(pairs))
	for _, p := range pairs {
		if strings.TrimSpace(p.Finding) == "" {
			return nil, fmt.Errorf("%s: a fixture pair names no finding", layer)
		}
		if p.Triggers == p.Clears {
			return nil, fmt.Errorf("%s: the fixture pair for %s names one draft twice", layer, p.Finding)
		}
		triggers, err := readFixture(fsys, pack, layer, p.Triggers)
		if err != nil {
			return nil, err
		}
		clears, err := readFixture(fsys, pack, layer, p.Clears)
		if err != nil {
			return nil, err
		}
		out = append(out, FixturePair{Layer: layer, Finding: p.Finding, Triggers: triggers, Clears: clears})
	}
	return out, nil
}

func readFixture(fsys fs.FS, pack, layer, name string) (Fixture, error) {
	underFixtures := strings.HasPrefix(name, "fixtures/") || strings.HasPrefix(name, path.Join(pack, "fixtures")+"/")
	if !underFixtures || !fs.ValidPath(name) {
		return Fixture{}, fmt.Errorf("%s names fixture %q; a fixture sits under fixtures/ or the pack's own fixtures/", layer, name)
	}
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return Fixture{}, fmt.Errorf("%s names a fixture that cannot be read: %w", layer, err)
	}
	front, body, err := splitFrontMatter(name, raw)
	if err != nil {
		return Fixture{}, err
	}
	var f fixtureFile
	if err := decodeStrict(name, front, &f); err != nil {
		return Fixture{}, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return Fixture{}, errors.New(name + ": the body is the draft, and it is empty")
	}
	return Fixture{
		Path: name, Subject: f.Subject, Body: strings.TrimPrefix(string(body), "\n"), Advertising: f.Advertising,
		Sender: f.Sender, Language: f.Language, MailLanguage: f.MailLanguage,
		Particulars: f.Particulars, Translations: f.Translations,
	}, nil
}
