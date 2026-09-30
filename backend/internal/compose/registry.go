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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
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
	return registryWithGate(db, auth.NewGate(identity.NewServiceFor(db)), nil, send, companyEnricher{}, nil, nil, nil,
		meetingBriefReader(newMeetingBriefService(db)), slog.Default(), registryFeatures{reporting: true})
}

type registryFeatures struct{ lists, reporting bool }

func registryWithDraftBrain(pool *pgxpool.Pool, brain completer, send SendPath) *agents.Registry {
	db := InstallationDB(pool)
	brief := meetingBriefReader(newMeetingBriefService(db))
	if brain == nil {
		return registryWithGate(db, auth.NewGate(identity.NewService(pool)), nil, send, companyEnricher{}, nil, nil, nil, brief, slog.Default(), registryFeatures{})
	}
	return registryWithGate(db, auth.NewGate(identity.NewService(pool)), newReplyDrafter(pool, brain, nil), send, companyEnricher{}, nil, nil, nil, brief, slog.Default(), registryFeatures{})
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
func registryWithGate(db *database.DB, gate *auth.Gate, drafter activities.EmailDrafter, send SendPath, enricher agents.CompanyEnricher, embedder search.Embedder, transcriptOnLanding activities.TranscriptReadEnqueue, imports agents.Imports, meetingBrief agents.MeetingBriefReader, log *slog.Logger, features registryFeatures, opts ...agents.RegistryOption) *agents.Registry {
	pool, provider := db.Pool(), providerWithTranscripts(db, transcriptOnLanding)
	// Every mutating tool shares retry claims and replay authorization on every host role.
	opts = append(opts, withContractTierFloor(),
		agents.WithIdempotency(toolIdempotency(pool)), agents.WithReplayReader(provider),
		agents.WithBaseLanguage(installationLanguage(pool)))
	// Approval decisions need the same registered effects as the HTTP path.
	approvalsSvc := decidingApprovalsService(pool, send, log)
	registry := agents.NewRegistry(approvalsAdapter{svc: approvalsSvc}, gate, opts...)
	agents.RegisterCoreTools(registry, provider, provider, provider, fieldOwnership{pool: pool}, newConsumerMailSeam(db),
		openDuplicatesFor(pool))
	agents.RegisterListTool(registry, provider, provider)
	relinker, disqualifier, demoter, advancer := lifecycleSeams(pool)
	agents.RegisterLifecycleTools(registry, provider, relinker, disqualifier, demoter, advancer)
	agents.RegisterBulkTool(registry, bulkChangeSeam{engine: newBulkEngine(db, gate).withListsIf(features.lists)})
	agents.RegisterEnrichTool(registry, provider, enricher)
	agents.RegisterPipelineTool(registry, pipelineLister(pool))
	agents.RegisterApprovalTools(registry, approvalQueue(approvalsSvc))
	agents.RegisterReportTool(registry,
		reportToolRunner(newReportEngine(pool)),
		reportToolCatalog(), reportPlanVocabulary())
	agents.RegisterReportVocabularyTool(registry,
		agents.NewReportVocabularyResource(reportToolCatalog()))
	if features.reporting {
		agents.RegisterReportingTool(registry, reportingReader(pool))
	}
	agents.RegisterAnalyticsReportTool(registry,
		analyticsReportComposer(pool, analyticsquery.DefaultFloor, features.reporting))
	// The UI and tools share calculation engines and current-reader disclosure rules.
	agents.RegisterAnalyticsQueryTool(registry,
		analyticsQueryToolRunner(InstallationDB(pool)))
	agents.RegisterAnalyticsVocabularyTool(registry, analyticsVocabularyReader{})
	agents.RegisterRecordFieldsTool(registry, agents.RecordFieldsResource{})
	agents.RegisterReportBlocksTool(registry,
		agents.NewReportBlocksResource(reportBlockGrammar()))
	agents.RegisterForecastTool(registry, forecastToolReader(pool))
	agents.RegisterMovementTool(registry, movementToolReader(pool))
	agents.RegisterAssuranceTool(registry, assuranceToolReader(pool))
	agents.RegisterInputChecksTool(registry, inputChecksToolReader(pool))
	agents.RegisterCoverageTool(registry, coverageToolReader(pool))
	// Search references are read back through the governed provider before disclosure.
	agents.RegisterQueryTool(registry, provider,
		queryRunner(pool, embedder),
		seatNamer(identity.NewService(pool)))
	agents.RegisterVocabularyTool(registry, search.NewQuerySchemaResource(queryVocabulary(pool)))
	agents.RegisterBriefTool(registry, briefReader(pool))
	agents.RegisterAnnotateBriefTool(registry, briefAnnotator(pool))
	searchRetriever, retriever := registryRetrievers(pool, embedder)
	agents.RegisterIntentTools(registry, retriever, meetingBrief, provider)
	agents.RegisterChannelProviderTools(registry, channelProviderDirectory{})
	agents.RegisterContextSearchTool(registry, provider, retriever)
	agents.RegisterReportEvidenceTool(registry, provider, reportEvidenceSeam{
		db: InstallationDB(pool), floor: analyticsquery.DefaultFloor,
		ranker: searchRetriever, classifier: searchRetriever,
	}.SearchReportEvidence)
	agents.RegisterResolveTool(registry, provider, entityResolver(pool))
	agents.RegisterWhoamiTool(registry, actingIdentity(pool))
	agents.RegisterColleaguesTool(registry, colleagueLister(pool))
	agents.RegisterTagTools(registry, tagSeam(pool))
	agents.RegisterListTools(registry, newListSeam(pool, features.lists))
	agents.RegisterImportTools(registry, importsOr(imports, db))
	agents.RegisterSlippingTools(registry, slippingLister(pool), followUpDrafter(provider))
	agents.RegisterCommitmentTool(registry, commitmentLister(pool))
	agents.RegisterHandoffTool(registry, handoffReader(pool))
	agents.RegisterProject360Tool(registry, project360Reader(pool))
	agents.RegisterNetworkTools(registry, whoKnowsLister(pool, contacts.NewStore(InstallationDB(pool))), coverageReader(pool, contacts.NewStore(InstallationDB(pool))),
		introPathLister(pool),
		atRiskLister(pool, contacts.NewStore(InstallationDB(pool))))
	agents.RegisterCommsTools(registry, newCommsAdapter(pool, drafter, send), provider)
	agents.RegisterMeetingInvitationTool(registry, newCommsAdapter(pool, drafter, send), provider)
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
		// The owner narrowing too: a model reading a per-rep breakdown with no
		// signal would report it as every rep's.
		if outcome.PopulationNarrowed != "" {
			result["population_narrowed"] = outcome.PopulationNarrowed
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

// providerWithTranscripts is the provider over db, starting a transcript read
// when one lands if the role wired a reader for it.
func providerWithTranscripts(db *database.DB, onLanding activities.TranscriptReadEnqueue) *Provider {
	provider := NewProviderFor(db)
	if onLanding != nil {
		provider = provider.WithTranscriptEnqueue(onLanding)
	}
	return provider
}

func registryRetrievers(pool *pgxpool.Pool, embedder search.Embedder) (*search.Retriever, riskAwareRetriever) {
	retriever := search.NewRetriever(search.NewStore(InstallationDB(pool)), embedder)
	return retriever, riskAwareRetriever{pool: pool, inner: retriever}
}
