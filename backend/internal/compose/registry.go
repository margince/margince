// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The governed MCP tool surface, assembled: the agents registry over the
// composite datasource provider, with the approvals engine injected as
// the staging/redemption dependency — composed here so agents never
// imports a sibling module (ADR-0054 §9).

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/overlay"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// NewRegistry wires the full 🟢/🟡 tool set over the composite provider.
// The admission gate re-derives authority through the shared/ports/authz
// seam, which identity implements — injected here so platform/auth never
// imports a module (ADR-0054 §5).
func NewRegistry(pool *pgxpool.Pool, send SendPath) *agents.Registry {
	return NewRegistryFor(InstallationDB(pool), send)
}

// NewRegistryFor is NewRegistry over a handle whose workspace is already
// decided. A server resolves the installation's singleton, which is what
// NewRegistry does for it; a harness that seeds a second workspace on purpose
// has no singleton to resolve and names the one it means instead. Same wiring,
// same gate — only where the tenant comes from differs (ADR-0091 §9 step 3).
func NewRegistryFor(db *database.DB, send SendPath) *agents.Registry {
	// The gate resolves seats through identity, and identity binds the same
	// handle: a registry built for a named workspace must not admit through a
	// service that resolves a different one.
	return registryWithGate(db, auth.NewGate(identity.NewServiceFor(db)), nil, nil, send, companyEnricher{}, nil, nil, nil,
		meetingBriefReader(newMeetingBriefService(db)), slog.Default())
}

// NewRegistryWithIncumbent is NewRegistry plus the per-workspace live-incumbent
// resolver the overlay write-back path (Create/Update/Archive) reaches HubSpot
// through — the wiring a role with a vault (the api server) installs so the MCP
// tool surface can actually write back, not just answer errNoWriteIncumbent.
func NewRegistryWithIncumbent(pool *pgxpool.Pool, resolveIncumbent func(context.Context) (overlay.Incumbent, error), send SendPath) *agents.Registry {
	db := InstallationDB(pool)
	return registryWithGate(db, auth.NewGate(identity.NewService(pool)), nil, resolveIncumbent, send, companyEnricher{}, nil, nil, nil,
		meetingBriefReader(newMeetingBriefService(db)), slog.Default())
}

func registryWithDraftBrain(pool *pgxpool.Pool, brain completer, resolveIncumbent func(context.Context) (overlay.Incumbent, error), send SendPath) *agents.Registry {
	db := InstallationDB(pool)
	brief := meetingBriefReader(newMeetingBriefService(db))
	if brain == nil {
		return registryWithGate(db, auth.NewGate(identity.NewService(pool)), nil, resolveIncumbent, send, companyEnricher{}, nil, nil, nil, brief, slog.Default())
	}
	return registryWithGate(db, auth.NewGate(identity.NewService(pool)), newReplyDrafter(pool, brain, nil), resolveIncumbent, send, companyEnricher{}, nil, nil, nil, brief, slog.Default())
}

// registryWithGate composes the tool surface. The volume budget charger arrives as
// an option rather than a parameter because only the API server — the one role
// that serves agent principals through the MCP and REST doors — has a meter to
// charge. The Surface-B runner and the workflow paths run as the human or the
// system that started them, and the volume meter governs agents only, so a registry
// built without one is not an unmetered agent surface; it is a surface no agent
// reaches.
//
// embedder is the RETRIEVAL embed lane, and it is a parameter rather than a
// construction detail because it is the composition root's to choose: a role
// with no model path has none. A nil lane is still legal — a role with no
// model path has none, and the offline fake binds no embeddings model — and
// every path that can lose the vector lane says so on the wire rather than
// serving a lexically-ranked page under a semantic label.
func registryWithGate(db *database.DB, gate *auth.Gate, drafter activities.EmailDrafter, resolveIncumbent func(context.Context) (overlay.Incumbent, error), send SendPath, enricher agents.CompanyEnricher, embedder search.Embedder, transcriptOnLanding activities.TranscriptReadEnqueue, imports agents.Imports, meetingBrief agents.MeetingBriefReader, log *slog.Logger, opts ...agents.RegistryOption) *agents.Registry {
	// The Dispatcher is the datasource seam every core/slipping tool
	// rides: a native-mode workspace lands on the composite SoR
	// Provider exactly as before, an overlay-mode workspace's reads land
	// on the mirror (design.md §4.2/§4.6) — chosen per call from
	// ctx, never fixed at registry construction time.
	//
	// No tool reaches Dispatcher.Freshness, the only route to a force-fresh
	// reservation on this provider, so this surface has no spend of its own to
	// account for and its OVB meter is a fail-closed placeholder (no Redis),
	// never charged; the live reservations and charges live in the refetch and
	// reconcile pollers, which take their own Redis-backed meters. When a
	// metered force-fresh path lands for a tool, this becomes a Redis-backed
	// NewOverlayMeter like the REST surface's, sharing the same per-workspace
	// windows.
	pool := db.Pool()
	native := NewProviderFor(db)
	if transcriptOnLanding != nil {
		native = native.WithTranscriptEnqueue(transcriptOnLanding)
	}
	provider := NewDispatcher(native, NewOverlayProviderFor(db, failClosedOverlayMeter(), resolveIncumbent), pool)
	// Retry safety, wired for EVERY role that composes this surface rather than
	// arriving as the API server's option the way the read charger does. The
	// difference is who the promise is made to: the read bound governs agent
	// principals, so a role no agent reaches needs no meter — but the retry key
	// is advertised on every mutating tool's schema by the registry itself, so
	// any surface built here can be asked to honour one, and a surface that
	// advertises the key and cannot claim it refuses the call.
	//
	// The replay reader is the composite provider this registry is already
	// composed over, so a recorded result's records are re-checked through the
	// same door — mirror included — that a live read of them would take.
	// The contract's per-record-type tier floor, on EVERY role that composes this
	// surface. A verb's own tier cannot express "confirm-first for a project and
	// auto-execute for a person", so without this the tool door admits at a tier
	// the contract tightened and the REST door refuses (#982) — one credential,
	// two answers, which is what ADR-0055 exists to prevent.
	opts = append(opts, withContractTierFloor(),
		agents.WithIdempotency(toolIdempotency(pool)), agents.WithReplayReader(provider))
	// ONE approvals service for both directions of the 🟡 loop, and it is the
	// service that carries the follow-on EFFECTS. Staging can run on a bare
	// engine — it writes a proposal and nothing else — but deciding cannot: a
	// kind whose release the engine does not know about would be marked approved
	// and never performed, so a held message would read as sent and stay held.
	// The HTTP door decides on an effects-registered service for exactly this
	// reason (approvalsHandlersWithEffects); here approvalQueue refuses at boot
	// an engine that could decide a kind it cannot release.
	approvalsSvc := decidingApprovalsService(pool, send, log)
	registry := agents.NewRegistry(approvalsAdapter{svc: approvalsSvc}, gate, opts...)
	// The guards take the Dispatcher as an overlayModeChecker — the interface
	// whose method IS the uncached read, so no wiring here can hand them the
	// cached mode. See overlayModeChecker for why that distinction is typed.
	sorMode := overlayModeChecker(provider)
	agents.RegisterCoreTools(registry, provider, provider, provider, fieldOwnership{pool: pool}, newConsumerMailSeam(db),
		// A create says what it filed for review. Without this seam it stays
		// silent, which is what it did before — see tools_dupereport.go for why
		// silence was the defect.
		openDuplicatesFor(pool))
	// list_records reads its rows through the Dispatcher like every other
	// record verb, and its filter VOCABULARY off the native provider: the
	// vocabulary is a property of the deployment's own stores, resolved once at
	// boot, while whether a given workspace's rows come from those stores or
	// from a mirror is a per-call question the Dispatcher answers. An overlay
	// workspace refuses the filtered call rather than answering it unnarrowed.
	agents.RegisterListTool(registry, provider, native)
	// The three lifecycle transitions reach their owning modules directly
	// rather than through the Dispatcher: each one's behaviour IS that
	// module's entry point, which is what the REST route calls too.
	relinker, disqualifier, advancer := lifecycleSeams(pool)
	// disqualify_lead is the one of the three the overlay provider cannot
	// serve for a mirrored type, so it takes the guard the REST middleware
	// applies to the same verb; relink and project-phase are not SoR record
	// writes and stay available in either mode.
	agents.RegisterLifecycleTools(registry, provider, relinker, nativeOnlyDisqualifier(sorMode, disqualifier), advancer)
	// enrich rides the site-read seam rather than the datasource one: it reads
	// the company's OWN website, which no record provider can answer.
	agents.RegisterEnrichTool(registry, provider, enricher)
	// Pipeline config, and it registers next to the core CRUD set because it is
	// what makes two of those verbs reachable: create_record for a deal and
	// advance_deal both name ids no other tool yields. Config is not a record,
	// so it rides its own seam rather than the datasource one — and that seam
	// needs the overlay guard the record verbs get from the Dispatcher for free.
	agents.RegisterPipelineTool(registry, nativeOnlyPipelines(sorMode, pipelineLister(pool)))
	// The confirm-first queue, read and answered from the same conversation a
	// call was staged in. Nothing here decides anything the person behind the
	// passport could not decide in the app.
	agents.RegisterApprovalTools(registry, approvalQueue(approvalsSvc))
	agents.RegisterReportTool(registry, nativeOnlyReportRunner(sorMode, reportToolRunner(newReportEngine(pool))), reportToolCatalog())
	// The vocabulary that plan is written in, as a TOOL and not only as the
	// margince://schema/reports resource — same reason describe_query_vocabulary
	// exists next to query_workspace, and one reason more: the Surface-B runner
	// is offered no resource read at all, so for a scheduled agent this is the
	// ONLY route to the names run_report refuses against.
	//
	// It takes the SAME overlay guard run_report does, and the query
	// vocabulary's comment says why: a vocabulary looks answerable anywhere, and
	// what decides it is not whether the names are true but whether the verb
	// they describe can be called. Here it cannot.
	agents.RegisterReportVocabularyTool(registry,
		nativeOnlyReportVocabularyReader{mode: sorMode, inner: agents.NewReportVocabularyResource(reportToolCatalog())})
	// The forecast, read through the same assembler the HTTP surface uses, so
	// the two transports cannot disagree about what a quarter contains.
	// A report whose every figure came from a saved run, rendered through the
	// SAME validator and renderer POST /analytics/reports/render calls. The
	// floor is DefaultFloor on both paths: a figure a person may not see is one
	// a model asking on their behalf may not see either.
	agents.RegisterAnalyticsReportTool(registry,
		analyticsReportComposer(pool, analyticsquery.DefaultFloor))
	// The grammar that document is written in, as a TOOL and not only as the
	// margince://schema/report-blocks resource — same reason
	// describe_report_vocabulary exists beside run_report: the Surface-B runner
	// is offered no resource step, so for a scheduled agent this is the only
	// route to the kinds compose_analytics_report refuses against.
	agents.RegisterReportBlocksTool(registry,
		agents.NewReportBlocksResource(reportBlockGrammar()))
	agents.RegisterForecastTool(registry, forecastToolReader(pool))
	agents.RegisterMovementTool(registry, movementToolReader(pool))
	agents.RegisterAssuranceTool(registry, assuranceToolReader(pool))
	agents.RegisterInputChecksTool(registry, inputChecksToolReader(pool))
	agents.RegisterCoverageTool(registry, coverageToolReader(pool))
	// The governed workspace query. It takes the provider as well as the runner
	// because the two halves of an answer come from different places: the plan
	// selects records through the search module, and each selected record is
	// READ back through the datasource seam — the one path that stamps the
	// trust tier, collects the envelope's freshness, applies this caller's
	// object RBAC and row scope to the record itself, and charges the record
	// against their read bound. The guard is outermost for
	// the same reason the intent tools' is: the executor queries native tables
	// an overlay workspace has no rows in.
	// The seat namer names each row's owner; seatNamer says why.
	agents.RegisterQueryTool(registry, provider,
		nativeOnlyQueryRunner(sorMode, queryRunner(pool, embedder)),
		seatNamer(identity.NewService(pool)))
	// The vocabulary that plan is written in, as a TOOL and not only as the
	// margince://schema/query resource — because a client that reads tools and
	// not resources could otherwise watch query_workspace refuse a name and
	// have no way to learn the right one. Same resolver, same document; the
	// resource stays for the clients that prefer it.
	//
	// It takes the SAME outermost overlay guard query_workspace does. A
	// vocabulary looks answerable anywhere — it describes a grammar rather
	// than reading rows — but in an overlay workspace the plans it teaches are
	// refused outright, so serving it there advertises a field list nothing
	// can execute.
	agents.RegisterVocabularyTool(registry, nativeOnlyVocabularyReader{
		mode:  sorMode,
		inner: search.NewQuerySchemaResource(queryVocabulary(pool)),
	})
	// The morning brief. It ranks the rep's own open deals out of the native
	// tables, which an overlay workspace has no rows in, so it takes the same
	// outermost guard the other native-only engines do: "not available here"
	// rather than an empty queue that reads as a quiet morning.
	agents.RegisterBriefTool(registry, nativeOnlyBriefReader(sorMode, briefReader(pool)))
	// The write half: the overnight agent puts what it found onto the run it
	// just read. Same guard, same engine, and the engine owns every refusal —
	// whose run, which items, which citations.
	agents.RegisterAnnotateBriefTool(registry, nativeOnlyBriefAnnotator(sorMode, briefAnnotator(pool)))
	// The intent tools ground on the graph walk; search_context rides the same
	// retriever's ranked half, which is what the embed lane is for.
	// The comms tools ride the same store paths as the HTTP transport.
	// The overlay guard stays OUTERMOST so a mirror-backed workspace is
	// refused before either read runs; the risk decorator sits inside it and
	// adds the coverage findings a deal anchor would otherwise assemble
	// without.
	retriever := nativeOnlyRetriever{
		mode: sorMode,
		inner: riskAwareRetriever{
			pool:  pool,
			inner: search.NewRetriever(search.NewStore(InstallationDB(pool)), embedder),
		},
	}
	// The brief seam takes no overlay guard of its own: nativeOnlyRetriever
	// above refuses an overlay workspace before prep_for_meeting reaches
	// either half, and a second guard on one tool would make two comments
	// claim one refusal — which is what the guard census refuses.
	agents.RegisterIntentTools(registry, retriever, meetingBrief)
	// The transport directory, read from this package's boot snapshot — the
	// composed set is the composition root's fact, so the module takes it as a
	// seam rather than enumerating connectors it may not reach.
	agents.RegisterChannelProviderTools(registry, channelProviderDirectory{})
	// search_context takes the provider as well, for the reason query_workspace
	// does: the retriever answers refs and excerpts, and every record behind
	// them is READ BACK through the datasource seam — where the trust tier is
	// stamped, the caller's own row scope is re-applied, and the record is
	// charged against their read bound.
	agents.RegisterContextSearchTool(registry, provider, retriever)
	// Identity resolution. The ladder is workspace-wide by design — a duplicate
	// is a duplicate whoever is looking — so the provider is not decoration
	// here: it is the ONLY thing that applies this caller's row scope to a
	// record the resolver named, and the tool serves nothing it did not read
	// back through it.
	agents.RegisterResolveTool(registry, provider, nativeOnlyResolver(sorMode, entityResolver(pool)))
	agents.RegisterWhoamiTool(registry, actingIdentity(pool))
	agents.RegisterColleaguesTool(registry, colleagueLister(pool))
	agents.RegisterTagTools(registry, tagSeam(pool))
	// The migrate-in verbs, ALWAYS served: the contract declares all four, and
	// a registry that does not serve a declared verb advertises something
	// tools/list cannot offer. A registry built with no Server falls back to
	// bare handlers — its reads work, and the three verbs that need the source
	// file refuse with errNoObjectStore, which is what a role storing no
	// objects can honestly do.
	if imports == nil {
		imports = importsOverDB(db)
	}
	agents.RegisterImportTools(registry, imports)
	// The pipeline-risk intents: the candidate set rides the deals
	// module's row-scoped list, the drafts land through the provider.
	agents.RegisterSlippingTools(registry, nativeOnlySlippingLister(sorMode, slippingLister(pool)), followUpDrafter(provider))
	// The commercial reads: what this workspace promised and has not
	// delivered, and what the delivery side of a project is being handed.
	// Both ride the owning modules' own gated store paths
	// (commercialseams.go), and both take the outermost overlay guard the
	// other native-only engines do — a mirrored workspace has no task
	// projection and no project at all, and "nothing is outstanding" is the
	// one wrong answer here that reads as good news.
	agents.RegisterCommitmentTool(registry, nativeOnlyCommitments(sorMode, commitmentLister(pool)))
	agents.RegisterHandoffTool(registry, nativeOnlyHandoff(sorMode, handoffReader(pool)))
	// The project page, read by the tool through the SAME assembly the HTTP
	// route serves (project360seam.go), under the same per-section gates.
	agents.RegisterProject360Tool(registry, nativeOnlyProject360(sorMode, project360Reader(pool)))
	// The relationship-graph reads (ADR-0078): who here knows this contact,
	// how a deal is covered, who can get us into an account, and which of the
	// caller's deals the coverage rules flag. All 🟢 — they name people, they
	// change nothing.
	agents.RegisterNetworkTools(registry, whoKnowsLister(pool), coverageReader(pool, people.NewStore(InstallationDB(pool))),
		nativeOnlyIntroPath(sorMode, introPathLister(pool)),
		nativeOnlyAtRisk(sorMode, atRiskLister(pool, people.NewStore(InstallationDB(pool)))))
	agents.RegisterCommsTools(registry, newCommsAdapter(pool, drafter, send), provider)
	// The location check (🟢), and the verb the probe card hangs off. It reads
	// no record and takes no seam, so it registers unconditionally.
	//
	// TEMPORARY. It exists to answer one question — does a chat host let a
	// Margince card read the device's position — which no document can answer
	// and which has a different answer per host. Delete it and its view once the
	// matrix is filled in; see apps.GeoProbeURI.
	agents.RegisterGeoProbeTool(registry)
	// The composed extension set's governed tools ride the same registry
	// and admission gate as the core tools, registered last so a name that
	// collides with a core verb fails loudly (RegisterExtensions stashed
	// them at boot, before this ran).
	//
	// They carry NO native-only guard, and that is now a debt rather than a
	// property. It used to be a property: a handler was handed a context and
	// raw JSON and nothing else, so it could not read a domain table at all.
	// It can now — extension.ToolHandler receives a per-call Runtime whose Tx
	// runs the unit's own SQL on this pool (compose/extruntime.go), which
	// reaches core tables directly and bypasses the Dispatcher's mode routing
	// entirely. An extension tool CAN therefore answer from native state for a
	// workspace whose system of record is the overlay mirror.
	//
	// Nothing is exposed today: openchannel is the only served first-party unit
	// and its SQL reaches none but its own tables, and the composed set is the
	// trust boundary (see buildExtensionTools). The guard — or the per-unit
	// grants that would make it unnecessary — is issue #627, to settle before a
	// unit reads a domain table.
	registerComposedTools(registry)
	return registry
}

// reportToolRunner adapts the engine to the tool seam: decode the
// plan arguments, run, re-encode the contract-shaped result.
func reportToolRunner(engine *reportEngine) agents.ReportRunner {
	return func(ctx context.Context, report string, planArgs json.RawMessage) (json.RawMessage, error) {
		var req reportRequest
		if len(planArgs) > 0 {
			// STRICT: a plan argument this engine does not serve is refused by
			// name, not dropped. A lenient decode would answer a request for
			// something this engine cannot do — a historical snapshot, say —
			// with current state and no warning, and a silent wrong answer is
			// worse than a refusal because nothing tells the caller to look
			// again.
			// The unserved key is named BEFORE the shape refusal. The strict
			// decode alone answers "a plan argument is not the shape this tool
			// takes" and then describes the arguments the caller did not send,
			// so a caller who sent one unserved key is told to re-check shapes
			// that were never wrong, and can loop on it. Which key is unserved
			// is a question this package answers exactly.
			if unserved := unservedPlanArguments(planArgs); len(unserved) > 0 {
				return nil, httperr.Validation("arguments", "malformed_json",
					"this tool does not take "+strings.Join(unserved, ", ")+
						"; its plan arguments are `"+slotFilters+"`, `"+slotGroupBy+"` and `"+slotAggregates+"`")
			}
			if err := strictDecodeReportPlan(planArgs, &req); err != nil {
				// Server-authored. The REST twin forwards the decoder's own text
				// under the field `body`, which is wrong here twice over: this tool
				// has no `body` argument, and the Go decoder names internal types
				// (`compose.reportRequest`) an agent can neither read nor act on.
				//
				// The field is `arguments` — what the MCP surface actually calls the
				// object the caller supplied — because the decoder cannot say WHICH
				// of the three plan arguments is misshapen, and naming one would
				// point at an argument that may well be correct. The message carries
				// all three shapes, which is the part a caller acts on.
				return nil, httperr.Validation("arguments", "malformed_json",
					"a plan argument is not the shape this tool takes: `filters` is an object, "+
						"`group_by` an array of strings, `aggregates` an array of {fn, field, as} objects")
			}
		}
		outcome, err := engine.Run(ctx, report, req)
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"report":  outcome.Report,
			"plan":    outcome.Plan,
			"columns": outcome.Columns,
			// Never null: every other list-shaped answer on this surface
			// normalizes, because a model reads null as "unknown" where an
			// empty array says "none matched". reportOutcome.Rows guarantees
			// it, so this is the shape both transports already agree on.
			"rows":         outcome.Rows,
			"total_rows":   len(outcome.Rows),
			"generated_at": outcome.GeneratedAt,
			// The frame, same as the HTTP envelope carries. A number without
			// the zone that cut its days and the month its year opens is not
			// placeable, and a model reading it will place it wrongly rather
			// than ask.
			"as_of":                   outcome.GeneratedAt,
			"timezone":                outcome.Timezone,
			"base_currency":           outcome.BaseCurrency,
			"fiscal_year_start_month": outcome.FiscalYearStartMonth,
		}
		// A field mask shrank this run's row set: say so, exactly like the
		// HTTP envelope does — a reduced total with no signal is the
		// ambiguity the field exists to prevent, and a model acts on it.
		if outcome.ExcludedByPermission != nil {
			result["excluded_by_permission"] = *outcome.ExcludedByPermission
		}
		return json.Marshal(result)
	}
}

// decidingApprovalsService builds the approvals engine the TOOL surface decides
// through: the registration list plus the send-dependent releases.
//
// Both halves or neither. The list alone leaves held_draft — the one kind whose
// release is a send — with no executor on this engine, so approving one here
// would commit the decision, answer the caller success, and leave the message
// held forever. The HTTP inbox gets the same two halves at a different moment
// (applySendPath), because its surface is built before the send path is
// assembled and this one is built after.
// The process logger rides along for the reason the HTTP door takes one: a
// bundle member whose effect fails AFTER its decision has committed is reported
// to the caller by outcome alone, so the cause has nowhere to go but the log —
// and this is the door where the wire carries the least.
func decidingApprovalsService(pool *pgxpool.Pool, send SendPath, log *slog.Logger) *approvals.Service {
	svc := approvalsServiceWithEffects(pool)
	registerLateApprovalEffects(svc, pool, send)
	if log != nil {
		svc = svc.WithLogger(log)
	}
	return svc
}
