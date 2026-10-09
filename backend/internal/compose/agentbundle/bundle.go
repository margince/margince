// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agentbundle builds the Margince skill an AI tool installs to call the
// API with a passport: a ZIP of one folder holding the two guides, the
// operation index and the agent contract, all naming this install's API
// address. The contract and the index are generated from api/crm.yaml by
// tools/gen-agentcontract (make gen).
package agentbundle

import (
	"archive/zip"
	"bytes"
	"embed"
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

// Builder builds the ZIP for one API base URL at a time, and keeps the last
// one: an install answers one configured base, so a rebuild is rare, and
// keeping more would let a caller grow the cache by varying the request host.
type Builder struct {
	mu      sync.Mutex
	base    string
	archive []byte
}

// Build returns the ZIP whose files name apiBase, the URL `/v1` included.
func (b *Builder) Build(apiBase string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.archive != nil && b.base == apiBase {
		return b.archive, nil
	}
	archive, err := build(apiBase)
	if err != nil {
		return nil, err
	}
	b.base, b.archive = apiBase, archive
	return archive, nil
}

func build(apiBase string) ([]byte, error) {
	spec, err := withServer(apiBase)
	if err != nil {
		return nil, err
	}
	files := []struct {
		name string
		body []byte
	}{
		{"README.md", nil}, {"SKILL.md", nil}, {"INDEX.md", index}, {"openapi.yaml", spec},
	}
	for i, file := range files[:2] {
		var rendered bytes.Buffer
		if err := guideTemplates.ExecuteTemplate(&rendered, file.name+".tmpl", struct{ BaseURL string }{apiBase}); err != nil {
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
func withServer(apiBase string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(contract, &doc); err != nil {
		return nil, fmt.Errorf("agentbundle: parsing the embedded contract: %w", err)
	}
	if len(doc.Content) == 0 || len(doc.Content[0].Content) == 0 {
		return nil, fmt.Errorf("agentbundle: the embedded contract is empty; run make gen")
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

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
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
