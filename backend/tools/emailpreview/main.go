// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command emailpreview writes fictional examples using the production renderers.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func main() {
	input := flag.String("in", "", "directory of fictional production message JSON files")
	directory := flag.String("out", "", "directory for HTML and plain-text previews")
	flag.Parse()
	if *directory == "" || *input == "" {
		fmt.Fprintln(os.Stderr, "pass -in with production message JSON and -out with a preview directory")
		os.Exit(2)
	}
	if err := writePreviews(*input, *directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type preview struct {
	name string
	text string
}

func writePreviews(input, directory string) (err error) {
	root, err := os.OpenRoot(input)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	entries, err := os.ReadDir(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	var gallery []preview
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := root.ReadFile(entry.Name())
		if err != nil {
			return err
		}
		var message struct{ Subject, Text, HTML string }
		if err := json.Unmarshal(data, &message); err != nil {
			return err
		}
		if message.Subject == "" || message.Text == "" || message.HTML == "" {
			return fmt.Errorf("incomplete production message: %s", entry.Name())
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		if err := writePair(directory, name, message.Text, message.HTML); err != nil {
			return err
		}
		gallery = append(gallery, preview{name: name, text: message.Text})
	}
	if len(gallery) == 0 {
		return errors.New("no production messages in input directory")
	}
	return writeGallery(directory, gallery)
}

func writePair(directory, name, text, markup string) error {
	if err := os.WriteFile(filepath.Join(directory, name+".txt"), []byte(text), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, name+".html"), []byte(markup), 0o600)
}

func writeGallery(directory string, previews []preview) error {
	slices.SortFunc(previews, func(a, b preview) int { return strings.Compare(a.name, b.name) })
	var page strings.Builder
	page.WriteString(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Product email previews</title><style>body{font:16px Arial;margin:24px}article{margin:32px 0}.pair{display:flex;gap:24px;flex-wrap:wrap}iframe{width:390px;height:900px;border:1px solid #ccc}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-width:600px}</style><h1>Product email previews</h1><p>Fictional production-assembled messages. Each HTML and text pair is captured from its message builder or delivery path. Browser dark mode and text zoom can be applied to each preview.</p>`)
	for _, preview := range previews {
		page.WriteString(`<article><h2>` + html.EscapeString(preview.name) + `</h2><div class="pair"><iframe title="` + html.EscapeString(preview.name) + `" src="` + html.EscapeString(preview.name) + `.html"></iframe><pre>` + html.EscapeString(preview.text) + `</pre></div></article>`)
	}
	page.WriteString(`</html>`)
	return os.WriteFile(filepath.Join(directory, "index.html"), []byte(page.String()), 0o600)
}
