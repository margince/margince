// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The `aitask retrieve` verb: what retrieval hands the model for one question,
// against the handbook this binary ships.
//
// It answers a question the rest of the tool cannot. `run` grades a model over
// passages somebody chose; this says which passages production would have
// chosen, and that is the half nobody can write by hand — the production
// chunker cuts spans that bundle a definition, a phase list and half of the
// next heading, while a fixture typed from memory is four tidy sentences. A
// fixture that supplies its own version of retrieval grades the model on input
// production never sends.
//
// IT OPENS NO DATABASE, which is why it can sit beside aitask.go without
// costing that file its claim. The corpus is the embedded handbook, the chunker
// is knowledge.ChunkText, and the ranking is knowledge.RankInMemory — a
// DECLARED MIRROR of the SQL in Store.Retrieve, held by an integration-lane
// gate that runs both over one ingested corpus. askmirror.go carries the whole
// argument; the short version is that a DB-free eval loop was worth a second
// implementation of ranking only because that gate exists.
//
// What it costs: one embed call per chunk the vector cache does not already
// hold, plus one for the question. A re-run over an unchanged handbook embeds
// the question alone.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/cliflags"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/vectorkit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// previewWidth is how much of a passage the table shows: wide enough to
// recognise which part of a page was retrieved, narrow enough that eight rows
// stay one screen. Recognising the passage is the column's whole job.
const previewWidth = 64

type aiTaskRetrieveFlags struct {
	question  string
	page      string
	modelSpec string
	floor     float64

	workDir string
	outPath string

	logLevel  string
	logFormat string
}

func aiTaskRetrieveFlagSet() (*flag.FlagSet, *aiTaskRetrieveFlags, *cliflags.Env) {
	cfg := &aiTaskRetrieveFlags{}
	env := &cliflags.Env{}
	fs := flag.NewFlagSet("worker aitask "+verbRetrieve, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.StringVar(&cfg.question, "question", "", "the question, spelled exactly as the reader who hit the failure spelled it")
	// -q because this is the one flag retyped on every iteration of the loop.
	fs.StringVar(&cfg.question, "q", "", "short form of --question")
	fs.StringVar(&cfg.page, "page", "", "restrict the corpus to one handbook page (e.g. records.md); empty ranks the whole handbook")
	fs.StringVar(&cfg.modelSpec, "model", "", "embedding model, provider:model (e.g. mistral:mistral-embed)")
	fs.Float64Var(&cfg.floor, "floor", knowledge.DefaultMinSimilarity,
		"the grounding floor a passage must reach to be citable; a corpus row carries its own, this is the shipped default")
	env.String(fs, &cfg.workDir, "work-dir", "MARGINCE_AITASK_DIR", workDirDefault,
		"gitignored directory the captured fixture and the vector cache live in")
	fs.StringVar(&cfg.outPath, "out", "", "write the fixture here instead of the work directory")
	env.String(fs, &cfg.logLevel, "log-level", "MARGINCE_LOG_LEVEL", "info", "log level: debug|info|warn|error")
	env.String(fs, &cfg.logFormat, "log-format", "MARGINCE_LOG_FORMAT", "text", "log format: text|json")
	return fs, cfg, env
}

func parseAITaskRetrieveFlags(args []string) (aiTaskRetrieveFlags, error) {
	fs, parsed, env := aiTaskRetrieveFlagSet()
	if err := fs.Parse(args); err != nil {
		return aiTaskRetrieveFlags{}, err
	}
	env.Apply(fs, config.FromOS)
	cfg := *parsed
	if rest := fs.Args(); len(rest) > 0 {
		return aiTaskRetrieveFlags{}, fmt.Errorf(
			"aitask retrieve takes no positional arguments, got %q — a question belongs in --question, quoted",
			strings.Join(rest, " "))
	}
	return cfg, cfg.validate()
}

// validate refuses what could only fail after the first paid call, and names the
// flag that would fix each.
func (c aiTaskRetrieveFlags) validate() error {
	switch {
	case strings.TrimSpace(c.question) == "":
		return errors.New("aitask retrieve needs --question (or -q): a blank question retrieves nothing and proves nothing")
	case c.modelSpec == "":
		return errors.New("aitask retrieve needs --model provider:model: ranking means nothing unless the question and the corpus are embedded by one binding")
	case c.floor < 0 || c.floor > 1:
		return fmt.Errorf("aitask retrieve: --floor is a cosine and lives in [0,1], got %v", c.floor)
	case c.outPath == "-":
		return errors.New("aitask retrieve writes a report and a file, so --out names a file rather than '-'")
	}
	return nil
}

// runAITaskRetrieve is the verb: chunk the handbook, embed it, rank it, and keep
// what a run would be handed.
func runAITaskRetrieve(ctx context.Context, args []string, stdout io.Writer) error {
	cfg, err := parseAITaskRetrieveFlags(args)
	if err != nil {
		return err
	}
	if _, err := httpserver.InstallProcessLogger(stdout, cfg.logLevel, cfg.logFormat); err != nil {
		return err
	}
	embedder, banner, err := compose.TaskProbeEmbedder(cfg.modelSpec)
	if err != nil {
		return fmt.Errorf("aitask retrieve: %w", err)
	}
	// The embed lane meters every call against a workspace and refuses one
	// raised outside a workspace context. This probe belongs to no workspace, so
	// it mints an id per invocation rather than borrowing a real one: an id that
	// matched a customer's would file this run's spend under their budget. The
	// run verb mints one for the chat lane for the same reason.
	ctx = principal.WithWorkspaceID(ctx, ids.NewV7())
	identity, dims := embedder.EmbedIdentity()
	if _, err := fmt.Fprintf(stdout, "%s → %s\n", banner, identity); err != nil {
		return err
	}
	chunks, err := handbookChunks(cfg.page)
	if err != nil {
		return err
	}
	embedded, err := embedChunks(ctx, chunks, embedder, newVectorCache(cfg.workDir, identity), dims)
	if err != nil {
		return err
	}
	question, err := embedQuestion(ctx, cfg.question, embedder, dims)
	if err != nil {
		return err
	}
	passages := knowledge.RankInMemory(question, embedded, cfg.floor)
	if err := writeRetrieval(stdout, cfg, len(embedded), passages); err != nil {
		return err
	}
	return emitRetrievedFixture(stdout, cfg, passages)
}

// embedQuestion embeds the question the way knowledge.questionVector does,
// including refusing a zero vector: every cosine against it is NaN, and a
// ranking sorted on NaN puts arbitrary passages at the top of an answer.
func embedQuestion(ctx context.Context, question string, embedder vectorkit.Embedder, dims int) ([]float32, error) {
	vectors, err := embedInputs(ctx, embedder, []string{strings.TrimSpace(question)}, dims)
	if err != nil {
		return nil, fmt.Errorf("aitask retrieve: embedding the question: %w", err)
	}
	if vectorkit.IsZero(vectors[0]) {
		return nil, errors.New("aitask retrieve: the embed lane returned a zero vector for this question, so nothing can be ranked")
	}
	return vectors[0], nil
}

// writeRetrieval reports what would have reached the model: the status a screen
// would show, then one row per passage.
//
// The status is printed even when passages came back, because it answers a
// different question from the table's. The table says what was retrieved; the
// status says how much corpus it was retrieved from, and an empty table means
// opposite things at ten embedded passages and at four hundred.
func writeRetrieval(w io.Writer, cfg aiTaskRetrieveFlags, embedded int, passages []knowledge.Passage) error {
	scope := "the whole handbook"
	if cfg.page != "" {
		scope = cfg.page
	}
	outcome := "answered"
	if len(passages) == 0 {
		outcome = "not_covered — nothing reached the floor, so production would never ask the lane"
	}
	if _, err := fmt.Fprintf(w,
		"\ncorpus    %s, %d embedded passage(s)\nquestion  %s\nfloor     %.3f (the closest %d are ranked, then the floor is applied)\noutcome   %s\n\n",
		scope, embedded, cfg.question, cfg.floor, knowledge.RetrieveLimit, outcome); err != nil {
		return err
	}
	if len(passages) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "LABEL\tSIM\tDOCUMENT\tLINE\tPREVIEW"); err != nil {
		return err
	}
	for i, p := range passages {
		if _, err := fmt.Fprintf(tw, "p%d\t%.4f\t%s\t%d\t%s\n",
			i+1, p.Similarity, p.DocumentName, p.StartLine, passagePreview(p.Text)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// passagePreview flattens a passage to one scannable line. The newlines go
// because a passage spans several and one table row must stay one row.
func passagePreview(text string) string {
	flat := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(flat) <= previewWidth {
		return flat
	}
	return string([]rune(flat)[:previewWidth]) + "…"
}

// emitRetrievedFixture hands the result to the other command, in the shape that
// command's site decodes.
//
// An empty ranking writes nothing and says why: production never asks the lane
// without passages, so there is no run to hand this to. That is a real finding
// about the question — it is simply not something `run` can be pointed at.
func emitRetrievedFixture(w io.Writer, cfg aiTaskRetrieveFlags, passages []knowledge.Passage) error {
	if len(passages) == 0 {
		_, err := fmt.Fprintln(w,
			"Nothing cleared the floor, so there is no fixture to write — production would not ask the lane at all. The outcome above is the finding.")
		return err
	}
	fixture, err := compose.CorpusAskFixtureFrom(cfg.question, passages)
	if err != nil {
		return fmt.Errorf("aitask retrieve: %w", err)
	}
	path := artifactOutOr(cfg.outPath, cfg.workDir, retrieveArtifactName(cfg.question))
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := emitArtifact(w, path, fixture, "captured the retrieval as a corpus_ask fixture"); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, retrieveFollowUp, path)
	return err
}

const retrieveFollowUp = `
Read those passages with the shipped corpus_ask case (this one calls a model):

  make ai-probe ARGS="run --site corpus_ask/corpus_ask --fixture %s --expect <expectation.json> --model anthropic:claude-sonnet-4-6"

The expectation is yours to write: the labels above that a correct answer must
rest on, or [] where a correct answer cites nothing.
`

// retrieveArtifactName names a capture after the question, so two retrievals in
// one session do not overwrite each other.
func retrieveArtifactName(question string) string {
	slug := slugify(question)
	if len(slug) > maxArtifactSlug {
		slug = slug[:maxArtifactSlug]
	}
	return "retrieve-" + slug + ".json"
}
