// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agentbundle builds the Margince skill an AI tool installs to call the
// API with a passport. It is a ZIP of one folder: two guides, the operation
// index and the agent contract, all naming this install's API address.
// tools/gen-agentcontract generates the contract and the index (make gen).
package agentbundle

import (
	"archive/zip"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"sync"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Folder is the one folder the ZIP holds, named for the skill.
const Folder = "margince"

var (
	//go:embed agentcontract_gen.yaml
	contract []byte
	//go:embed agentindex_gen.txt
	index []byte
	//go:embed README.md.tmpl SKILL.md.tmpl
	guides embed.FS
)

var guideTemplates = template.Must(template.ParseFS(guides, "*.md.tmpl"))

// recentBases is how many bases a Builder keeps. A few covers an install reached
// by more than one host, and the bound stops a caller growing it by varying Host.
const recentBases = 4

// Builder builds the ZIP and the JSON contract for an API base URL and keeps
// the most recent few.
type Builder struct {
	// MCPURL is the connector address the README leads with; empty, it tells
	// the reader to use a passport.
	MCPURL string
	mu     sync.Mutex
	recent []built // newest last
}

type built struct {
	base    string
	archive []byte
	spec    []byte
}

// Build returns the ZIP whose files name apiBase, the URL `/v1` included.
func (b *Builder) Build(apiBase string) ([]byte, error) {
	entry, err := b.forBase(apiBase)
	return entry.archive, err
}

// SpecJSON returns the ZIP's openapi.yaml as JSON, with servers naming apiBase.
func (b *Builder) SpecJSON(apiBase string) ([]byte, error) {
	entry, err := b.forBase(apiBase)
	return entry.spec, err
}

func (b *Builder) forBase(apiBase string) (built, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, entry := range b.recent {
		if entry.base == apiBase {
			return entry, nil
		}
	}
	doc, err := withServer(apiBase)
	if err != nil {
		return built{}, err
	}
	specYAML, err := marshalYAML(doc)
	if err != nil {
		return built{}, err
	}
	archive, err := build(guideData{BaseURL: apiBase, MCPURL: b.MCPURL}, specYAML)
	if err != nil {
		return built{}, err
	}
	var specJSON bytes.Buffer
	if err := writeJSON(&specJSON, doc); err != nil {
		return built{}, err
	}
	entry := built{base: apiBase, archive: archive, spec: specJSON.Bytes()}
	if len(b.recent) == recentBases {
		b.recent = b.recent[1:]
	}
	b.recent = append(b.recent, entry)
	return entry, nil
}

// guideData is what README.md.tmpl and SKILL.md.tmpl may name.
type guideData struct {
	BaseURL string
	MCPURL  string
}

func build(data guideData, spec []byte) ([]byte, error) {
	files := []struct {
		name string
		body []byte
	}{
		{"README.md", nil}, {"SKILL.md", nil}, {"INDEX.md", index}, {"openapi.yaml", spec},
	}
	for i, file := range files[:2] {
		var rendered bytes.Buffer
		if err := guideTemplates.ExecuteTemplate(&rendered, file.name+".tmpl", data); err != nil {
			return nil, fmt.Errorf("agentbundle: rendering %s: %w", file.name, err)
		}
		files[i].body = rendered.Bytes()
	}

	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for _, file := range files {
		w, err := archive.Create(Folder + "/" + file.name)
		if err != nil {
			return nil, fmt.Errorf("agentbundle: adding %s: %w", file.name, err)
		}
		if _, err := w.Write(file.body); err != nil {
			return nil, fmt.Errorf("agentbundle: writing %s: %w", file.name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("agentbundle: closing the archive: %w", err)
	}
	return out.Bytes(), nil
}

// withServer returns the agent contract with servers naming apiBase, placed
// after info where an OpenAPI reader looks for it.
func withServer(apiBase string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(contract, &doc); err != nil {
		return nil, fmt.Errorf("agentbundle: parsing the embedded contract: %w", err)
	}
	if len(doc.Content) == 0 || len(doc.Content[0].Content) == 0 {
		return nil, errors.New("agentbundle: the embedded contract is empty; run make gen")
	}
	root := doc.Content[0]
	// The generator's DO NOT EDIT line is for this tree, not for the user's copy.
	doc.HeadComment, root.HeadComment, root.Content[0].HeadComment = "", "", ""
	server := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar("url"), scalar(apiBase)}}
	servers := []*yaml.Node{scalar("servers"), {Kind: yaml.SequenceNode, Content: []*yaml.Node{server}}}
	at := len(root.Content)
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "info" {
			at = i + 2
		}
	}
	root.Content = append(root.Content[:at], append(servers, root.Content[at:]...)...)
	return &doc, nil
}

func marshalYAML(doc *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("agentbundle: marshaling the contract: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("agentbundle: marshaling the contract: %w", err)
	}
	return out.Bytes(), nil
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
