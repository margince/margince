// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Where `aitask retrieve` gets a corpus without a database, and what it costs.
//
// The prose is the handbook this binary ships (knowledge/handbook), which is the
// corpus every installation has and the one the field failures were asked of.
// It is cut by knowledge.ChunkText — the PRODUCTION chunker, at its real width
// and overlap, carrying its own StartLine through — because the shape of a
// chunk is most of what makes a retrieval surprising, and re-cutting it here
// would produce passages nothing ever retrieves.
//
// Embedding is the only thing here that costs money, and the cache is what stops
// it costing money twice. knowledge.Chunk.Hash exists for exactly this: the
// chunker is pure, so identical prose yields an identical hash, and a re-run
// over an unchanged handbook embeds nothing but the question.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/platform/vectorkit"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// embedBatchSize is how many chunks travel in one embed request. It bounds the
// payload rather than tuning anything: a whole handbook in one call is a request
// a provider may refuse for its size, and the refusal would arrive after the
// slowest part of the run.
const embedBatchSize = 32

// handbookChunks is the corpus: every page this build ships, cut the way an
// ingest cuts it.
//
// page narrows it to one file, for the loop where an operator already knows
// which page the answer should have come from and does not want to pay to embed
// the other twenty. Narrowing CHANGES THE RANKING — the eight closest passages
// of one page are not the eight closest of the handbook — which is why the
// report says which scope it ran under rather than leaving it to be remembered.
func handbookChunks(page string) ([]knowledge.EmbeddedPassage, error) {
	pages, err := handbook.Pages()
	if err != nil {
		return nil, fmt.Errorf("aitask retrieve: reading the embedded handbook: %w", err)
	}
	var corpus []knowledge.EmbeddedPassage
	names := make([]string, 0, len(pages))
	for _, p := range pages {
		names = append(names, p.Filename)
		if page != "" && p.Filename != page {
			continue
		}
		for _, chunk := range knowledge.ChunkText(string(p.Content)) {
			corpus = append(corpus, knowledge.EmbeddedPassage{DocumentName: p.Filename, Chunk: chunk})
		}
	}
	if len(corpus) == 0 {
		return nil, fmt.Errorf("aitask retrieve: no handbook page named %q — this build ships %s",
			page, strings.Join(names, ", "))
	}
	return corpus, nil
}

// embedChunks fills in each passage's vector, paying only for the ones the cache
// does not already hold.
func embedChunks(
	ctx context.Context, corpus []knowledge.EmbeddedPassage,
	embedder vectorkit.Embedder, cache *vectorCache, dims int,
) ([]knowledge.EmbeddedPassage, error) {
	if err := cache.load(); err != nil {
		return nil, err
	}
	var pending []int
	for i := range corpus {
		if vec, held := cache.vectors[corpus[i].Chunk.Hash()]; held && len(vec) == dims {
			corpus[i].Vector = vec
			continue
		}
		pending = append(pending, i)
	}
	for start := 0; start < len(pending); start += embedBatchSize {
		end := min(start+embedBatchSize, len(pending))
		batch := pending[start:end]
		inputs := make([]string, 0, len(batch))
		for _, at := range batch {
			inputs = append(inputs, corpus[at].Chunk.Text)
		}
		vectors, err := embedInputs(ctx, embedder, inputs, dims)
		if err != nil {
			return nil, fmt.Errorf("aitask retrieve: embedding the corpus: %w", err)
		}
		for i, at := range batch {
			corpus[at].Vector = vectors[i]
			cache.vectors[corpus[at].Chunk.Hash()] = vectors[i]
		}
	}
	if len(pending) > 0 {
		if err := cache.save(); err != nil {
			return nil, err
		}
	}
	return corpus, nil
}

// embedInputs is the one embed call this verb makes, with the shape check the
// knowledge store makes on its own: a lane that answered a different number of
// vectors, or vectors of another width, has not embedded these inputs, and
// ranking against whatever it did send would rank against noise.
func embedInputs(ctx context.Context, embedder vectorkit.Embedder, inputs []string, dims int) ([][]float32, error) {
	res, err := embedder.Embed(ctx, model.EmbedRequest{Inputs: inputs, Dimensions: dims})
	if err != nil {
		return nil, err
	}
	if len(res.Vectors) != len(inputs) || res.Dims != dims {
		return nil, fmt.Errorf("the embed lane returned %d vectors of width %d, need %d×%d",
			len(res.Vectors), res.Dims, len(inputs), dims)
	}
	return res.Vectors, nil
}

// vectorCache is the on-disk store of what has already been embedded, keyed by
// content hash under the live binding's identity.
//
// Per IDENTITY, in the filename, because vectors from two bindings live in
// different spaces: one file holding both would serve a mistral vector for a
// gemini run and rank it against the question without complaining. Under the
// gitignored work directory, because it holds the handbook's prose in numeric
// form and nothing about a probe belongs in a commit.
type vectorCache struct {
	path    string
	vectors map[string][]float32
}

func newVectorCache(workDir, identity string) *vectorCache {
	return &vectorCache{
		path:    filepath.Join(workDir, "vectors", slugify(identity)+".json"),
		vectors: map[string][]float32{},
	}
}

// load reads what is already known. An ABSENT file is not an error — the first
// run has no cache — but an unreadable or malformed one is: silently re-embedding
// a whole handbook because a file was truncated is a bill nobody asked for and
// nothing would say why.
func (c *vectorCache) load() error {
	raw, err := os.ReadFile(c.path) // #nosec G304 -- the path is this verb's own work directory, from the operator's own flag
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("aitask retrieve: reading the vector cache: %w", err)
	}
	if err := json.Unmarshal(raw, &c.vectors); err != nil {
		return fmt.Errorf("aitask retrieve: the vector cache at %s is not readable, so delete it and re-run: %w", c.path, err)
	}
	return nil
}

func (c *vectorCache) save() error {
	doc, err := json.Marshal(c.vectors)
	if err != nil {
		return fmt.Errorf("aitask retrieve: rendering the vector cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return fmt.Errorf("aitask retrieve: %w", err)
	}
	if err := os.WriteFile(c.path, doc, 0o600); err != nil {
		return fmt.Errorf("aitask retrieve: writing the vector cache: %w", err)
	}
	return nil
}
