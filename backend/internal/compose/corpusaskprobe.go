// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Retrieval for the DB-less evaluation loop: the handbook this binary ships,
// chunked, embedded and ranked without a database behind it.
//
// It lives in compose because the knowledge module is compose's to reach — a
// process role may not import it — and because the ranking has to be the
// product's own. The chunker is knowledge.ChunkText at its real width, so the
// passages are the ones an ingest would produce; re-cutting them here would
// rank prose nothing ever retrieves.
//
// The caller gets passages already flattened, so a command can print and file
// them without naming a knowledge type.

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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// embedBatchSize bounds one embed request rather than tuning anything: a whole
// handbook in one call is a payload a provider may refuse for its size, and the
// refusal would arrive after the slowest part of the run.
const embedBatchSize = 32

// The knowledge module's own numbers, re-exported rather than restated so a
// process role can name them without importing the module. A copied value here
// would be a second definition of the floor and the limit, and the whole point
// of the probe is that it ranks by the same two the SQL does.
const (
	CorpusProbeFloor = knowledge.DefaultMinSimilarity
	CorpusProbeLimit = knowledge.RetrieveLimit
)

// CorpusProbe is one question asked of the shipped handbook.
type CorpusProbe struct {
	// ModelSpec is the EMBED binding, provider:model. Ranking a question against
	// a corpus embedded by a different one measures nothing.
	ModelSpec string
	Question  string
	// Page narrows the corpus to one file. It CHANGES THE RANKING — the closest
	// eight of one page are not the closest eight of the handbook — so the
	// result says which scope it ran under rather than leaving it remembered.
	Page string
	// WorkDir holds the vector cache, and is gitignored: it carries the
	// handbook's prose in numeric form.
	WorkDir string
	Floor   float64
}

// CorpusProbePassage is one retrieved passage, flattened for a caller that must
// not name a knowledge type.
type CorpusProbePassage struct {
	DocumentName string
	Text         string
	StartLine    int
	Similarity   float64
}

// CorpusProbeResult is what retrieval would hand the lane.
type CorpusProbeResult struct {
	Banner        string
	EmbedIdentity string
	// Embedded is the whole corpus, not the ranked slice: an empty ranking means
	// opposite things at ten embedded passages and at four hundred.
	Embedded int
	Passages []CorpusProbePassage
	// Fixture is the corpus_ask fixture for these passages, nil when nothing
	// cleared the floor — production would not ask the lane at all, so there is
	// no run to hand one to.
	Fixture []byte
}

// ProbeCorpusRetrieval chunks the shipped handbook, embeds what the cache does
// not hold, and ranks it the way Store.Retrieve would.
func ProbeCorpusRetrieval(ctx context.Context, probe CorpusProbe) (CorpusProbeResult, error) {
	embedder, banner, err := TaskProbeEmbedder(probe.ModelSpec)
	if err != nil {
		return CorpusProbeResult{}, err
	}
	// The embed lane meters every call against a workspace and refuses one
	// raised outside a workspace context. This probe belongs to none, so it
	// mints an id per invocation rather than borrowing a real one: an id that
	// matched a customer's would file this run's spend under their budget.
	ctx = principal.WithWorkspaceID(ctx, ids.NewV7())
	identity, dims := embedder.EmbedIdentity()
	corpus, err := handbookChunks(probe.Page)
	if err != nil {
		return CorpusProbeResult{}, err
	}
	corpus, err = embedChunks(ctx, corpus, embedder, newVectorCache(probe.WorkDir, identity), dims)
	if err != nil {
		return CorpusProbeResult{}, err
	}
	question, err := embedQuestion(ctx, probe.Question, embedder, dims)
	if err != nil {
		return CorpusProbeResult{}, err
	}
	ranked := knowledge.RankInMemory(question, corpus, probe.Floor)
	result := CorpusProbeResult{
		Banner: banner, EmbedIdentity: identity, Embedded: len(corpus),
		Passages: flattenProbePassages(ranked),
	}
	if len(ranked) == 0 {
		return result, nil
	}
	if result.Fixture, err = CorpusAskFixtureFrom(probe.Question, ranked); err != nil {
		return CorpusProbeResult{}, err
	}
	return result, nil
}

func flattenProbePassages(ranked []knowledge.Passage) []CorpusProbePassage {
	out := make([]CorpusProbePassage, 0, len(ranked))
	for _, p := range ranked {
		out = append(out, CorpusProbePassage{
			DocumentName: p.DocumentName, Text: p.Text,
			StartLine: p.StartLine, Similarity: p.Similarity,
		})
	}
	return out
}

// handbookChunks is the corpus: every page this build ships, cut the way an
// ingest cuts it.
func handbookChunks(page string) ([]knowledge.EmbeddedPassage, error) {
	pages, err := handbook.Pages()
	if err != nil {
		return nil, fmt.Errorf("corpus probe: reading the embedded handbook: %w", err)
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
		return nil, fmt.Errorf("corpus probe: no handbook page named %q — this build ships %s",
			page, strings.Join(names, ", "))
	}
	return corpus, nil
}

// embedChunks fills in each passage's vector, paying only for the ones the cache
// does not already hold. knowledge.Chunk.Hash exists for exactly this: the
// chunker is pure, so a re-run over unchanged prose embeds nothing but the
// question.
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
		batch := pending[start:min(start+embedBatchSize, len(pending))]
		inputs := make([]string, 0, len(batch))
		for _, at := range batch {
			inputs = append(inputs, corpus[at].Chunk.Text)
		}
		vectors, err := embedInputs(ctx, embedder, inputs, dims)
		if err != nil {
			return nil, fmt.Errorf("corpus probe: embedding the corpus: %w", err)
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

// embedQuestion refuses a zero vector for the reason the knowledge store does:
// every cosine against it is NaN, and a ranking sorted on NaN puts arbitrary
// passages at the top of an answer.
func embedQuestion(ctx context.Context, question string, embedder vectorkit.Embedder, dims int) ([]float32, error) {
	vectors, err := embedInputs(ctx, embedder, []string{strings.TrimSpace(question)}, dims)
	if err != nil {
		return nil, fmt.Errorf("corpus probe: embedding the question: %w", err)
	}
	if vectorkit.IsZero(vectors[0]) {
		return nil, errors.New("corpus probe: the embed lane returned a zero vector for this question, so nothing can be ranked")
	}
	return vectors[0], nil
}

// embedInputs carries the shape check the knowledge store makes: a lane that
// answered a different number of vectors, or vectors of another width, has not
// embedded these inputs, and ranking against whatever it did send ranks noise.
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
// Per IDENTITY in the filename, because vectors from two bindings live in
// different spaces: one file holding both would serve a mistral vector for a
// gemini run and rank it without complaining.
type vectorCache struct {
	path    string
	vectors map[string][]float32
}

func newVectorCache(workDir, identity string) *vectorCache {
	return &vectorCache{
		path:    filepath.Join(workDir, "vectors", cacheFileName(identity)+".json"),
		vectors: map[string][]float32{},
	}
}

// cacheFileName keeps an embed identity readable in a path — the point of
// naming the file after the binding is that a reader can see which one a cache
// holds. It is deliberately not the artifact slug a command uses for
// operator-facing filenames: this one names a cache key and may never change,
// where that one is free to.
func cacheFileName(identity string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		default:
			return '_'
		}
	}, identity)
}

// load reads what is already known. An ABSENT file is not an error — the first
// run has no cache — but an unreadable one is: silently re-embedding a whole
// handbook because a file was truncated is a bill nobody asked for.
func (c *vectorCache) load() error {
	raw, err := os.ReadFile(c.path) // #nosec G304 -- the path is the probe's own work directory, from the operator's own flag
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("corpus probe: reading the vector cache: %w", err)
	}
	if err := json.Unmarshal(raw, &c.vectors); err != nil {
		return fmt.Errorf("corpus probe: the vector cache at %s is not readable, so delete it and re-run: %w", c.path, err)
	}
	return nil
}

func (c *vectorCache) save() error {
	doc, err := json.Marshal(c.vectors)
	if err != nil {
		return fmt.Errorf("corpus probe: rendering the vector cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o750); err != nil {
		return fmt.Errorf("corpus probe: %w", err)
	}
	if err := os.WriteFile(c.path, doc, 0o600); err != nil {
		return fmt.Errorf("corpus probe: writing the vector cache: %w", err)
	}
	return nil
}
