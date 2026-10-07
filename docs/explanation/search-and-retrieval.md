<!-- prose:plain -->
# Search & retrieval: two arms, one row scope, two kinds of stale data

Search is how a typed query becomes a ranked list of records. Behind one more seam, it is also how
the AI layers build their answers on the workspace's own data.

Two separate ranks run for each query. The **lexical** arm is Postgres full-text search over
generated `search_tsv` columns. The **vector** arm ranks by how much the meaning matches, over
pgvector embeddings. RRF (`reciprocal rank fusion`) merges the two into one list.

One rule holds for both: a search hit *is* a read. So the row-scope authority of the caller is built **into** the query.
It is not a filter on its results after.

The second half covers the part that many get wrong. An embedding store can be stale in two
different ways, and only one of them is a decision for a human. Where the model for the embed lane
comes from is in [ai-runtime.md](ai-runtime.md); who may see a row is in
[authorization.md](authorization.md). The record of who knows who, used here, is in
[relationship-graph.md](relationship-graph.md).

## The whole shape

```text
 query text
     │
     ├──────────── LEXICAL ARM ────────────┐        ┌──── VECTOR ARM ─────────────────┐
     │  one UNION branch per entity type   │        │  embed the query on the CURRENT │
     │  websearch_to_tsquery('simple',     │        │  binding (identity, dims)       │
     │    f_unaccent(q))                   │        │      │                          │
     │  ‖ f_fold_apostrophes(q)            │        │      ▼                          │
     │  ‖ german/english stems (activity)  │        │  embedding e JOIN <entity> t    │
     │      │                              │        │  WHERE e.model = $identity      │
     │      ▼                              │        │  1 - (e.embedding <=> $q)       │
     │  ts_rank_cd(t.search_tsv, …)        │        └──────────┬──────────────────────┘
     └──────────┬──────────────────────────┘                   │
                │   every branch of BOTH arms:                 │
                │   auth.Require(entity, read)  ← denied type contributes no branch
                │   + ScopeClause / ActivityContentClause        │
                │   + archived_at IS NULL                      │
                ▼                                              ▼
            ┌──────────────── RRF fusion (k = 60) ────────────────┐
            │  score = Σ lanes 1/(k + rank)   ties → (type, id)   │
            └──────────────────────┬─────────────────────────────┘
                                   ▼
                         ranked hits (fused score)
```

- **The entity types search can find** are declared once, in `searchBranches`: `contact`,
  `company`, `deal`, `lead`, `project`, `product`, `offer_template`, `tag`, `activity`. To let search
  find another entity, add a row there, plus the matching `embedText` and `pendingSources`
  entries. The query builder works out the rest. `SearchedTables()` is what the GIN check on the
  table shape asks about, so a branch added without an index fails and is not missed.
- **Three of them carry no owner**: `product`, `offer_template` and `tag`. `product` and
  `offer_template` are the price list and the offer templates: one catalog the whole workspace
  shares. So they declare `workspaceWide`, and the object grant is the whole of the gate. `tag` is
  `workspaceWide` for the same reason, and `textOnly` too. A word has no record shape to plan
  a query over, nothing next to it to walk to, and no text to embed. That is why it alone is not a
  context anchor.
- **`active` does not narrow what search finds.** A product the company no longer offers still stands on
  older offers, and a user who looks one up may be holding one of them. `archived_at`, which every
  branch carries, is the one "is it still live" question that counts when search looks for a record.
- **The lexical arm** ranks with `ts_rank_cd` over the generated `search_tsv` column of each table.
  It pages by a keyset cursor on `(score DESC, rtype, id)`, so the edge of a page holds still while
  others write at the same time. Name fields parse `simple`, with accents removed (`Muller` finds
  `Müller`). They also match the parse with apostrophes removed (`oreilly` finds `O'Reilly`). The
  activity branch also matches German and English stems, so `Vertrag` reaches a row that holds
  `Verträge`.
- **The vector arm** is one row per `(entity, chunk_ix)` in `embedding`. It is ranked by the cosine
  operator (`<=>`), and always kept to the current embed identity (next section). There is no HNSW
  index. The query per branch, kept to one identity, reads the table in order. An index over a
  column that holds vectors of more than one length could not be used in any case.
- **The merge is RRF**, `k = 60` (`rrfK`). Each lane adds `1/(k + rank)`, so an entity both lanes
  agree on ranks above the top entry of either lane alone. Both lanes fetch `3 × limit` rows, because
  an entity ranked right past `limit` in each lane can still merge into the top set. The `Score` of
  each returned hit is the **merged** score, not the lane score it comes with.
- **The vector arm drops back to lexical.** Take a nil embedder, or an embedder with a binding whose
  `EmbedIdentity()` is `""`. Both look the same from the query side: no live embed lane to rank
  against. The merged path then returns the lexical lane alone, and does not call `Embed()` with no
  binding.

  A query vector of all zeros is refused for a different reason. The cosine of a zero vector is
  `0/0 = NaN`, and an `ORDER BY sim DESC` with no guard puts NaN *first*. It would then rank above
  every real match, and no error would show it. The same check sits on the write side: a zero
  vector is never stored.

**Two entry points run different queries.** `GET /v1/search` runs the **lexical arm alone**
(`Store.Search`). It is ranked and paged by cursor, and every result is marked
`trust_tier: authoritative`. That is the grade the contract puts on records Margince holds itself.
`external` is kept for rows that come from a connector, and Margince does not send it yet.

**`ts_rank_cd` cannot be checked across types.** Take a message that names an account in its
subject and again in its body. It ranks above the account, whose name is one word in class `A`.
So a short list ranked across types can hold nothing but mail. `per_type=N` asks `GET /v1/search`
for a **grouped** page instead (`groupedShape`).

Each branch that passes the gate is capped at `N + 1` rows before the `UNION`. The last row is
dropped and reported in `types_with_more`, and the page takes no cursor or limit. To page through
the rest, narrow to one type with `types`. The `⌘K` menu (`per_type=3`) and the full results screen
with no type set (`per_type=5`) both ask for it. They show an activity that carries an
`email_summary` under `Emails`, separate from the calls and notes beside it.

**`with_employees=true` finds contacts through their company.** The `search_tsv` of
a contact holds its name and title, never the company. So "Acme" alone would find Acme, and none
of the contacts who work there. The flag adds one more `UNION` part, the **employer arm**
(`employerArmSQL`). The store builds it; it is not declared in `searchBranches`.

That table means one row per entity search can find, to the vector lane, the plan compiler and two
AST gates. The employer arm is a second way to reach the contact type. The arm:

- **starts from the companies the company branch finds**: the same match, `archived_at`,
  `NOT is_anchor` and company row scope, capped to the top `maxPerType` (20) matches. Every
  company a grouped page can show adds the contacts who work there. But a two-letter prefix, or a
  word for a line of business, cannot fan out to every contact at every company in the workspace.
  The installation's own company adds no one, because all its contacts work there;
- **follows current jobs only**: `kind = 'employment'`, not archived, and
  `employment.IsCurrentSQL`, the same reading as the company's own list of who works there. A
  contact in their notice time still counts; a past job, or a contact who only gets invoices, does
  not;
- **passes three gates, and refuses without an error**. They are `contact` and `company` read,
  the edge gate `auth.EdgeReadScope`, and both row scopes. (Who works where is a fact about the two
  records.) Any
  refusal drops the arm, and the search still answers. A query with the web-search operators drops
  it too, because `-acme` would match most companies;
- **never shows a contact twice**: it leaves out contacts the contact branch already matches by
  their own text. It keeps one row per contact (`DISTINCT ON`): the employer with the top match, and
  the job marked first for that contact on top. Each hit carries that company as `works_at`;
- **ranks below every hit on own text**: its score is `-1/(1+rank)`, in `[-1, 0)`, below any
  `ts_rank_cd`. So both page shapes put a contact matched by name first, and the
  `(score, type, id)` cursor needs no second order. On a grouped page the arm is capped the same as
  any branch, and `page()` counts by type. So `per_type` still limits the contacts shown, and
  `types_with_more` stays true.

You must turn it on, because `Store.Search` has other callers. The agent retriever, `similar_to` in
the plan runner, and `HybridSearch` all use the lexical lane too, and `Classify` promises to agree
with it. Only the HTTP handler sets `Input.WithEmployees`, so those callers keep matching by own
text alone.

A company hit also carries `logo_url`. The own reader of the contacts module reads it for the
companies on the page (`contacts.CompanyLogoURLsBatch`, passed in by compose). So the URL comes from
the place that owns the company record.

The **merged** path (`Store.HybridSearch`) is reached through the `shared/ports/retrieval` seam
(`search.Retriever`), which is what the AI layers build their answers on. `cmd/api` connects it to the
embedder of the resolved model path for the offer draft surface. `cmd/worker` connects it as the
source for the Surface-B runner. The agent intent tools in `compose/registry.go` are built with a
**nil** embedder. They use the context graph walk, which needs no embed lane.

## Row scope is not a filter on the results

A search hit *is* a read. So the authority of the caller is built into the query that makes the result
set:

- **Object RBAC chooses the branches.** `branchScope` runs `auth.Require(ctx, entity, read)` for each
  entity type. A type the role cannot read adds **no `UNION` branch at all**, with no error. So search
  can never see more than the list for each entity. If RBAC refuses every type asked for, the answer
  is an empty page, never a 403 that would show which types exist.
- **Row scope goes in the same branch.** `auth.ScopeClauseFor` (or `auth.ActivityContentClause` for
  the link walk of the activity branch) is added to the `WHERE` of that branch, inside the same
  `database.WithWorkspaceTx` transaction. Both arms use the same helper; the merge adds no
  visibility of its own.
- **The context walk gates the same way.** `anchorProfile` is the gate for the whole result. It checks
  both whether the record exists *and* whether the caller can see it: `auth.EnsureVisible`, then a
  real row read whose `pgx.ErrNoRows` becomes `apperrors.ErrNotFound`. Every record the second hop
  reaches is checked
  on its own with `auth.VisibleTo`. The walk adds context, never authority.

So a row-scope miss answers **404, not 403**. A record you cannot see looks the same as one that
does not exist. Every read of a single record works this way, and never says whether a record
exists. See
[authorization.md](authorization.md).

## Embedding identity: `provider/model@dimensions`

`Embedder.EmbedIdentity()` returns the mark of the current binding and the vector length it
expects. The identity is `<provider>/<model>@<dims>`, and it is written into
`embedding.model` on every row. By contract it costs no API call, so every read, every job check
and the readiness check can check against it.

**Retrieval keeps to the current identity.** There are two reasons. First, to stay right: rows
embedded by an older model must not rank against a query embedded in the new one.
Second, so it does not crash: the `embedding.embedding` column is a `vector` with no fixed length. So
`e.embedding <=> $query` by itself, against a row of another length, fails with
`different vector dimensions`. The identity filter leaves those rows out before the query
works out `<=>`.

**The write side keys on a text hash**, and knows the identity. `UpsertEmbedding` reads
the stored `(chunk_hash, model)` first. The same text under the same binding costs **no model
call**. The same text under a *changed* binding still embeds again.

A skip on the hash alone
would leave a row marked with a model that no longer serves the workspace. That row would look the
same as a live one. The write is a CAS on the hash it read.

Say a writer at the same time already moved the row past it. Then that writer keeps its row, and it is not written over.

Running this day to day rests on one fact these give together: results stay right even if a
reindex never finishes. A row that is stale, missing, or stored at another length is *kept out* of
retrieval, and never served as if it is current. Every row that is already current keeps
answering queries while a new build runs.

An embed lane **with no binding** (`--ai-fake`, or a routing config that never set a binding for `embeddings:`) is a
normal way to run. No binding marker is written, and `/readyz` reports `unknown`. The three
reindex endpoints stay their generated 501, no drift sweep job registers, and the merged
query drops back to lexical. Which model serves the lane, and its `dimensions`, are runtime config:
[ai-runtime.md](ai-runtime.md) and [../reference/configuration.md](../reference/configuration.md).

## Two kinds of stale data, two answers

| What is stale | How Margince finds it | Who fixes it |
|---|---|---|
| An entity with no embedding row **under the current identity**, while the identity the store holds **matches** the set one | the pending scan (`NOT EXISTS` against `embedding` at the current identity) | the **drift sweep** that runs on a schedule, with no operator and no confirm |
| The set identity **is not the same as** `embed_store_binding.populated_identity` | the marker read (one PK read, no scan) | an operator, through **preview → confirm** |

Both are "rows missing at the current identity", so one tool for both looks enough. They stay separate
because their *cost* is not the same. The first is work the system already agreed to do and failed to
do. The second is a rebuild of the whole corpus, and a human should see the price first.

### Drift: the bus lost the event, a worker sweep fixes it

`EmbedGen` subscribes to `cg:context-graph`. It embeds again an entity whose content changed
(`.created`, `.updated`, `.captured`, `.promoted`, `.merged`). That bus delivers each event **once or more**,
which does not mean it delivers to a process that keeps running. A worker that stops between ack
and write leaves an entity with no embedding row. The vector arm cannot see it until something
embeds it again. Without a sweep, it would wait until a user confirmed a reindex they never asked for.

- **`embed_drift_sweep`** is the dispatcher. It runs every **15 minutes**, starts one job per
  workspace, and does no tenant work itself. The contract states the budget. An empty pass is a
  small number of `NOT EXISTS` checks per workspace, each on an index. And 15 minutes is how long a
  lost embed event may keep a record out of vector search.
- **`embed_drift_workspace`** is the pass for one workspace. It has no time limit: the pending set
  limits the pass, and the time limit per call of the model lane limits each embed. It has three
  attempts, because the next dispatcher run *is* the real retry.

Both are declared in `backend/api/jobs.yaml`. `compose/embeddriftsweep.go` registers them under a
rule the contract's `when: [Embedder]` cannot state. The lane must also have a *binding* (an
`EmbedIdentity()` that is not empty). A lane that is set but has no binding writes no marker, so there
is nothing to check a row against.

What the sweep promises:

- **It is idempotent.** The same skip check makes reindex safe to stop and start again. It calls
  `UpsertEmbedding`, so an entity a normal embed already handled at the same time costs nothing,
  and a job that runs again is free.
- **A fix reads the current source text again.** The pending scan reads
  **IDs only**. `healEntity` reads the text again in its own transaction before it embeds. If it
  embedded the snapshot from the scan, it would store old text under the current identity. The row
  would then never look pending again, even if its embedding is wrong. An entity archived or
  made empty since the scan is no longer pending: it is skipped, and that is not an error.
- **It runs as the system principal.** Say an index is repaired through the row scope of one
  caller. It would leave out the records that caller cannot see, for everyone, and nothing would
  show it.

### A changed binding keeps its preview → confirm

When the operator changes the embed binding, the cost is theirs to decide, so a human must
confirm it. `GET /v1/embeddings/reindex/preview` returns the scope of the work before any cost.
It is always `estimate_quality: heuristic`: a work-shape floor of `SUM(octet_length(text))/4` over
the pending set, plus the budget band of each workspace. Then `POST /v1/embeddings/reindex`
confirms. Only admin and ops may call it, and it is `x-agent-access: human-only`.

The confirm claims
the marker and queues the run for all workspaces in **one** transaction. So a claim can never be
left behind with no job queued.

**By design, the sweep does nothing there.** It refuses in three cases, in this order:

1. the set identity is `""` (no lane with a binding) → there is nothing to fix under;
2. `populated_identity ≠ configured` → this is the binding change case, not drift;
3. `status = 'reembedding'` → a run for all workspaces already holds the marker.

That marker read happens **per workspace, inside the pass for that workspace**, not once for all of
them. Say a reindex is claimed, or a binding changes, after the dispatcher listed the workspaces.
Then the rest of the workspaces stop. They do not work at the same time as the run for all
workspaces. The sweep never
writes the marker at all.

The run the confirm starts is `embed_reindex`. It is a dispatcher that runs on **no schedule**,
because a reindex is a user's confirm and never a timer. It fans out to `embed_reindex_workspace`
(queue `ai_capture`, 5 attempts, no time limit). The marker is what makes it *one* run. The confirm
claims it under a new run ID. The dispatcher writes the pending set of workspaces and queues their
jobs in the same transaction.

Each workspace job leaves the set when it ends for good, and the job that empties the set hands the
marker back. Every marker write **fences on the run ID**. So a slow job from a finished run cannot
move the marker of the run that replaced it. Three things follow:

- **Margince stops a run when its binding changes.** The job carries
  the identity that held when it claimed the marker. A worker whose live embedder no
  longer matches it fails with `ErrIdentityDrift` → `river.JobCancel`. What all the workspaces need
  is a *new* run under the current config, not the rest of this row's attempts.
- **`populated_identity` names the last run that let go** of the marker. That is a
  smaller claim than "every workspace is embedded again under it". A run lets go when no workspace
  has a result left to reach, and *used up its attempts* is one of those results. The `active` of
  `/readyz` takes on that smaller claim. The pending counts on the status endpoint are what tell an
  operator what is missing.
- **A marker left held has a way back.** A workspace job that is dropped or canceled can leave the
  marker held with nothing running. (River starts again a job that runs too long only if the job
  declares a time limit, and a workspace job declares none.) A confirm with `force` set takes a
  marker whose last work is over an hour old (`reembedStaleAfter`). A pass that is working updates
  that timestamp before and after every step of its own work. That makes the window mean something,
  and makes `last progress N ago` a real sign in the settings card.

### Why the banner keys on the identities not matching

`frontend/src/app/embedreindexbanner.tsx` shows when, and only when,
`configured_identity !== populated_identity`. It does **not** also check `status`, even if a
banner shown while a build is working says nothing new. Turning it off while
`status = "reembedding"` would also cover up a build that stopped. That is a build whose job is canceled
for drift, or dropped after it used up its attempts, and that left the marker held. The `force`
build in the settings card exists to fix that state. So a banner that says nothing new while a
build works is the smaller cost.

Entities pending under a matching identity are not the banner's business either. The drift sweep is
already fixing them, and a banner about drift the system is repairing makes an admin stop reading
the banner. The pending count lives in the settings card (`frontend/src/screens/embedreindex.tsx`)
instead. The banner's own status read is gated in the client on `embedding_reindex:read`, the same
grant the server checks. It asks *before* it sends a query that could only 403.

## The context graph

`GET /v1/records/{entity_type}/{id}/context` returns what is linked to one record, and not a
ranked list of records. The AI layers and the `catch_me_up_on` and `prep_for_meeting` intent tools
consume it through the same `retrieval` seam. Every item carries a mark of where it comes from.

By design, the walk always takes two steps: two joins, not a walk that can go on and on. The walk
is: the anchor profile → the linked activities of the anchor → the *other* link targets of those
activities. The first hop is split into `recent_touches` and `open_tasks`. The second hop is sent as
`related_contacts`, `related_companies`, `related_deals` and `related_projects`. Every step reads at most 50 rows
before ranking cuts the list to `max_items` (default 5, capped at 25). So an anchor with 5,000 links
costs about what one with 50 costs.

The anchor types are the types search can find that are not activities and not `textOnly`, named by the path enum
in the contract. They come from `searchBranches`, not from a second list kept beside it. An activity
is a link, not a thing links point to. Only `contact`, `company`, `deal` and `project` can be walked
(`anchorLinkColumn`). `lead`, `product` and `offer_template` name no `activity_link` column this walk
follows, so their context is their profile alone.

Ranking follows the retrieval ranking `0.60·similarity + 0.30·recency + 0.10·source_trust`.
When two scores are the same, the smaller ID comes first. The recency part drops by half every 30 days.
Source trust is keyed on `captured_by`, the signed-in principal a write stores. A human's own
statement scores 1.0 (`T0`), and an agent write on a human's authority scores 0.7 (`T1`). Captured
or connector content scores 0.4 (`T2`).

`T2` is also the floor for a `captured_by` that is empty or that Margince does not know. Too much
trust is the error that does harm.

Margince checks for an imported row first, and it takes `T2`
even when it carries a human `captured_by`. An import runs as the user who starts it. So every row it
writes names that admin, and would wrongly read as their own words. Graph items carry no query
match, because there is no query. So their rank is recency × trust, with the same numbers.

A **contact** anchor also carries a `who_knows` section. It lists which users meet or write to this
contact, the most in touch first. Each comes with its band and interaction count. So a model that gets
the list cannot take the first name without looking. That section reads the `graph_interaction_edge`
projection, which has its own rules for how it stays current; see
[relationship-graph.md](relationship-graph.md).

## Reference

| Part | Where |
|---|---|
| Lexical index | generated `search_tsv` columns on `contact`, `company`, `deal`, `activity`, `lead`, `project`, `tag`, and `product` + `offer_template`, with the functions that remove accents and apostrophes (`backend/migrations/core/0001_baseline.up.sql`) |
| Vector store | `embedding` (`backend/migrations/core/0001_baseline.up.sql`: identity mark + a `vector` with no fixed length). **Not a tenant table**: no `workspace_id`, no RLS |
| Binding marker | `embed_store_binding` (`backend/migrations/core/0001_baseline.up.sql`: run, identity and the pending set of workspaces). **Not a tenant table**: no `workspace_id`, no RLS |
| Interaction projection | `graph_interaction_edge`; see [relationship-graph.md](relationship-graph.md) |
| RBAC object | `embedding_reindex` (`read`/`update`, admin + ops only) |
| Job kinds | `embed_drift_sweep` → `embed_drift_workspace` (on a schedule, every 15 minutes) · `embed_reindex` → `embed_reindex_workspace` (when asked), declared in `backend/api/jobs.yaml` |
| Routes | `GET /v1/search` · `GET /v1/records/{entity_type}/{id}/context` · `GET /v1/embeddings/reindex/status` · `GET /v1/embeddings/reindex/preview` · `POST /v1/embeddings/reindex` |
| Conflict codes on confirm | `reindex_running` · `reindex_not_needed` · `reindex_identity_drift` (all 409) |
| `/readyz` embed line | `active` · `needs_reindex` · `reembedding` · `unknown` (no lane, or the marker read failed); never gates readiness |
| Agent access | `/v1/search` is a 🟢 `search_records` read under a passport; the context walk and all three reindex endpoints are `x-agent-access: human-only` |
| Settings (embed binding, `dimensions`, reindex endpoints, provider notes) | [../reference/configuration.md](../reference/configuration.md) |

## Short rules

- **A search hit is a read.** Authority is built into the query, never a filter on its results. So
  an entity type RBAC refuses adds no branch, and a row-scope miss answers 404, not 403.
- **Never rank across embed identities.** Every vector read keeps to the current
  `provider/model@dims`. A row from an older binding is kept out, not served at the wrong length.
- **The vector lane drops back without an error.** No embedder, an identity with no binding, or a
  zero query vector drops back to the lexical arm. Some answers are more use than none.
- **Drift fixes itself; binding changes ask a human.** An entity pending under the *current*
  identity is a lost event, and the sweep embeds it again without being asked. A *changed* binding
  is a cost decision, so it keeps preview → confirm.
- **Right results never wait on a reindex.** A store that is half built again keeps stale rows out,
  and does not rank them. So results stay right while a new build runs.

## Where the code lives

| | |
|---|---|
| Lexical query | `internal/modules/search/store.go` (`Search`) |
| Ranked page + keyset cursor | `internal/modules/search/ranked.go` (`rankedShape`) |
| Grouped page (`per_type`) | `internal/modules/search/grouped.go` (`groupedShape`) |
| Contacts through the company that employs them (`with_employees`) | `internal/modules/search/employerarm.go` (`employerArmSQL`) |
| Company hit logo | `internal/modules/search/companylogo.go` ← `internal/modules/contacts/companylogobatch.go` |
| Which entities search can find, and who may see one | `internal/modules/search/branches.go` (`searchBranches`, `branchScope`, `SearchedTables`) |
| Vector write + read by meaning | `internal/modules/search/embedding.go` (`UpsertEmbedding`, `SimilarEntities`) |
| RRF merge | `internal/modules/search/fuse.go` (`HybridSearch`, `fuseRankedResults`, `rrfK`) |
| Indexer run by events | `internal/modules/search/embedgen.go` (`EmbedGen`, `embedText`) |
| Drift sweep (store half) | `internal/modules/search/driftsweep.go` (`SweepWorkspaceEmbeddingDrift`, `healEntity`) |
| Drift sweep (jobs) | `internal/compose/embeddriftsweep.go` |
| Binding marker + pending scan | `internal/modules/search/binding.go` (`SeedBinding`, `PopulatedIdentity`, `ReindexNeeded`, `pendingSources`) |
| Reindex run (store half) | `internal/modules/search/reembed.go` (`ReembedWorkspace`, `ErrIdentityDrift`) |
| Reindex transport + jobs | `internal/compose/embedreindextransport.go`, `internal/compose/jobs_embedreindex.go` |
| Readiness line | `internal/compose/embedreadyz.go` |
| Context graph walk | `internal/modules/search/graph.go`, `handlers_context.go` |
| Retrieval seam | `internal/shared/ports/retrieval` ← `internal/modules/search/retriever.go` |
| SPA surfaces | `frontend/src/app/embedreindexbanner.tsx` (advisory banner) · `frontend/src/screens/embedreindex.tsx` (settings card, preview/confirm/rebuild) |

## Where to go next

- [authorization.md](authorization.md): the gate every branch calls.
- [ai-runtime.md](ai-runtime.md): where the model for the embed lane comes from.
-
[write-backbone.md](write-backbone.md): the outbox the indexer consumes.
- [relationship-graph.md](relationship-graph.md).
- [../reference/configuration.md](../reference/configuration.md).
