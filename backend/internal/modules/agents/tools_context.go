// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The search_context tool (🟢): the retrieval primitive underneath the intent
// tools, exposed for the first time.
//
// WHAT IT IS FOR. search_records matches the text ON a record and answers
// records; query_workspace answers a plan with structure. Neither answers "find
// me the material about X" — the ranked, meaning-first sweep whose result is a
// record AND the excerpt that made it rank. That is what the hybrid arm has
// always done for catch_me_up_on's grounding, and what no caller could ask for
// directly.
//
// This tool owns the half the retriever cannot: turning its refs into records
// through the seam every read on this surface uses. The seam stamps the trust
// tier, collects freshness and evidence, applies the caller's object RBAC and
// row scope, and takes the answer's served count.
//
// AND IT SAYS WHICH LANE RANKED IT. Hybrid retrieval has two lanes and can lose
// the vector one — no bound embed model, or an embed call that failed. The hits
// still come back, and calling that page semantic would be the quietest wrong
// answer on this surface: a caller asking for likeness would read a word-overlap
// list as a meaning-ranked one, and nothing about it looks wrong.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// CodeSemanticRankingDegraded is the note a lexically-ranked answer carries.
//
// It is the SAME string query_workspace's executor publishes, and it says one
// thing to a caller — this ranking is word overlap, not meaning — for what are
// two conditions underneath: no embed lane is bound, or the embed call failed.
// Splitting them on the wire would ask a caller to branch on an operational
// detail they can do nothing about, and the action is identical either way:
// treat the order as weaker than asked for, or ask again later.
// TestTheSurfaceAndTheExecutorAgreeOnDegradation in the composition layer — the
// one place that can see both — fails if the two spellings ever diverge.
const CodeSemanticRankingDegraded = "semantic_ranking_degraded_to_lexical"

// contextSearchMaxLimit bounds one sweep. Every hit is a record read back
// through the seam and charged, so the page size is a spend the caller chooses.
const contextSearchMaxLimit = 25

// contextSearchDefaultLimit is what an unstated limit means. It is deliberately
// well under the ceiling: the common call is a model looking for the few most
// relevant things, and a caller that wants the ceiling can ask for it.
const contextSearchDefaultLimit = 10

// contextSearchMaxQueryRunes bounds the description.
//
// It is a bound on EGRESS, not on parsing. The query is embedded, which sends it
// verbatim to the configured provider, so an unbounded field is an unbounded
// paid call any read-scoped passport can make — and the only ceiling underneath
// is the chassis's request-body limit, which is megabytes. A thousand runes is
// several times the longest description that is still a description; past that
// the caller is pasting a document, which is what query_workspace and the
// import path are for.
const contextSearchMaxQueryRunes = 1000

// contextSearchTypes is the closed set of record types this tool sweeps, and it
// is enforced HERE rather than left to the schema.
//
// The registry validates required-presence, id shape and numeric bounds; it does
// not enforce a JSON-Schema enum, so an undeclared type would pass straight
// through to the retrieval index. That index carries a branch this tool does not
// publish — `activity`, whose snippet is the first 200 characters of a message
// body. A surface that advertises five types and serves six has granted a
// capability nobody declared, whatever its row scope.
// Keyed by the seam's own type so the set and the value handed to the retriever
// are the same thing, rather than two spellings kept in step.
var contextSearchTypes = map[datasource.EntityType]bool{
	datasource.EntityContact: true,
	datasource.EntityCompany: true,
	datasource.EntityDeal:    true,
	datasource.EntityLead:    true,
	datasource.EntityProject: true,
}

// RegisterContextSearchTool joins search_context to the surface once a retriever
// exists — the same conditional registration the other injected-engine tools
// take. An installation with no retriever serves no search tool rather than one
// that refuses every call.
func RegisterContextSearchTool(r *Registry, p datasource.SystemOfRecordProvider, retriever retrieval.Retriever) {
	if retriever == nil {
		return
	}
	r.Register(searchContext{p: p, retriever: retriever})
}

type searchContext struct {
	p         datasource.SystemOfRecordProvider
	retriever retrieval.Retriever
}

func (t searchContext) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "search_context", Title: "Search for relevant material", Version: toolVersionV1,
		Description:   searchContextCopy.render(),
		Instead:       searchContextCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		Unlisted:  true,
		OpenAPIOp: "searchContext",
		// The input is three members and stays three. The retrieval seam
		// serves exactly these, and a filter vocabulary invented here would be
		// a SECOND vocabulary for a question margince://schema/query already
		// answers — which is the thing the closed, catalog-derived grammar
		// exists to prevent. A caller who needs a date bound, an owner or a
		// relationship hop has query_workspace, and this tool's own
		// description says so rather than declaring a filter it would ignore.
		InputSchema: schema(`{"type":"object","required":["query"],"properties":{
			"query":{"type":"string","maxLength":1000,"description":"What to look for, in your own words. The wording is matched by meaning as well as by the words themselves, so a phrase that appears nowhere on a record can still rank it."},
			"record_types":{"type":"array","items":{"type":"string","enum":["contact","company","deal","lead","project"]},"description":"Restrict the sweep to these types; omit to sweep all of them."},
			"limit":{"type":"integer","minimum":1,"maximum":25}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[SearchContextResult](),
	}
}

// CoverageClasses is the closed set this tool's `coverage` field can hold, and
// it is deliberately SHORT one word: complete_exact is not in it and cannot be.
//
// A ranked hybrid page is the top of an ordering, never the whole of a match
// set — there is no threshold below which a record stops being relevant, only a
// limit past which it is not returned. So this tool has no exhaustive answer to
// give, and declaring the class would let a caller read one into it.
func (t searchContext) CoverageClasses() []string {
	return []string{CoverageRankedSemantic, CoveragePartialDegraded}
}

func (t searchContext) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Query       string   `json:"query"`
		RecordTypes []string `json:"record_types"`
		Limit       int      `json:"limit"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	// Trimmed before it is judged AND before it is sent: "   " is not a query,
	// and passing it on answers an empty page that a caller reads as "there is
	// nothing like that here" — the wrong answer this refusal exists to prevent.
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return nil, &BadArgsError{Cause: errors.New("`query` is required and takes the words to look for; " +
			"an empty query has no ranking to produce")}
	}
	if n := len([]rune(args.Query)); n > contextSearchMaxQueryRunes {
		return nil, &BadArgsError{Cause: fmt.Errorf(
			"`query` takes at most %d characters and this one carries %d; describe what you are looking for "+
				"rather than pasting the material", contextSearchMaxQueryRunes, n)}
	}
	q := retrieval.Query{Text: args.Query, Limit: contextSearchLimit(args.Limit)}
	for _, recordType := range args.RecordTypes {
		entity := datasource.EntityType(recordType)
		if !contextSearchTypes[entity] {
			return nil, &BadArgsError{Cause: fmt.Errorf(
				"`record_types` does not take %q; this tool sweeps contact, company, deal, lead and project",
				recordType)}
		}
		q.EntityTypes = append(q.EntityTypes, entity)
	}
	found, err := t.retriever.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	result, err := t.hydrate(ctx, found)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// contextSearchLimit resolves the page size. The seam clamps too, but silently
// and to its own ceiling. This tool publishes a maximum of 25 in its schema, so
// it has to mean that number.
func contextSearchLimit(asked int) int {
	if asked <= 0 {
		return contextSearchDefaultLimit
	}
	if asked > contextSearchMaxLimit {
		return contextSearchMaxLimit
	}
	return asked
}

// hydrate turns the retriever's hits into records through the datasource seam.
//
// READING AND SERVING ARE SEPARATE STEPS, for the reason query_workspace states:
// a hit that cannot be read back is dropped, and a dropped hit must not be
// counted or named. Noting during the read would charge the caller for a record
// they were never given and put it in the envelope's evidence, describing an
// answer that does not contain it.
//
// A hit CAN fail to read back even though both retrieval lanes are row-scoped.
// The two reads are separate, and the world moves between them: a record
// archived, or an authority narrowed, in the moment between ranking and
// hydration. Dropping such a hit in silence would hand back a short page that
// claims to be the top of the ordering.
func (t searchContext) hydrate(ctx context.Context, found retrieval.Result) (SearchContextResult, error) {
	result := SearchContextResult{
		Hits:     make([]SearchContextHit, 0, len(found.Hits)),
		Coverage: CoverageRankedSemantic,
		Notes:    []QueryNote{},
	}
	var dropped bool
	for _, hit := range found.Hits {
		record, readable, err := readServable(ctx, t.p, hit.Ref)
		if err != nil {
			return SearchContextResult{}, err
		}
		if !readable {
			dropped = true
			continue
		}
		result.Hits = append(result.Hits, SearchContextHit{
			Record:   newWireRecord(ctx, record),
			Score:    hit.Score,
			Excerpts: excerptsOf(hit.Evidence),
		})
	}
	if !found.SemanticRanking {
		result.Coverage = CoveragePartialDegraded
		result.Notes = append(result.Notes, QueryNote{
			Code: CodeSemanticRankingDegraded,
			Detail: "the meaning lane contributed nothing to this ranking — it is not configured, it " +
				"could not be reached, or nothing is indexed under the current model — so these " +
				"results are ranked by word overlap alone, and a phrase sharing no words with a " +
				"record cannot rank it",
		})
	}
	if dropped {
		result.Coverage = CoveragePartialDegraded
		result.Notes = append(result.Notes, QueryNote{
			Code: CodeRowUnreadable,
			// No COUNT. How many records a caller may not read is the size of
			// what was withheld, and stating it is the side channel
			// existence-hiding exists to close.
			Detail: "at least one ranked record could not be read back and is not among these hits; " +
				"search again to see the current answer",
		})
	}
	return result, nil
}

// excerptsOf carries the retriever's grounding onto the wire. An excerpt with no
// text is dropped rather than served empty: this surface's rule is
// evidence-or-omit, and an excerpt saying nothing is not evidence.
func excerptsOf(evidence []retrieval.Evidence) []ContextEvidence {
	out := make([]ContextEvidence, 0, len(evidence))
	for _, ev := range evidence {
		if ev.Snippet == "" {
			continue
		}
		out = append(out, ContextEvidence{Source: ev.Source, Snippet: ev.Snippet})
	}
	return out
}
