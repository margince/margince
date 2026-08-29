// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The Morning-Brief HTTP surface (E05): the home read (GetMorningBrief),
// the on-open/explicit refresh (GenerateMorningBrief), and the per-rep
// acted/dismissed/snoozed marks (B-E05.13, A77). It shadows the generated stubs over
// the BriefEngine. The brief is a PERSONAL lens — every operation is
// scoped to the acting rep by the engine, and another rep's item reads as
// not-found (existence-hiding), never forbidden.

import (
	"log/slog"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers wires the brief transport to the engine.
type Handlers struct {
	engine *BriefEngine
}

// NewHandlers binds the transport to a ready engine; compose constructs
// it once per process role.
func NewHandlers(engine *BriefEngine) Handlers { return Handlers{engine: engine} }

// WithL2Ranker forwards the api role's model lane to the engine — the
// deterministic §10.1 floor serves either way.
func (h Handlers) WithL2Ranker(brain briefBrain, log *slog.Logger) {
	h.engine.WithL2Ranker(brain, log)
}

// GetMorningBrief re-reads the acting rep's latest persisted run — the
// on-open path that never re-ranks (B-E05.3b). No run yet is a 404, the
// same existence-hiding shape as any absent personal resource. The read
// resolves snoozes against the current instant: expired ones re-surface,
// running ones stay hidden (A77/AC-home-6).
func (h Handlers) GetMorningBrief(w http.ResponseWriter, r *http.Request) {
	run, err := h.engine.LatestRun(r.Context(), time.Now().UTC())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, briefRunToWire(run))
}

// GenerateMorningBrief assembles today's run if the overnight pass has not.
//
// The night owns generation, so on an ordinary morning this route finds the
// day's run already there and returns it with 200 rather than ranking a second
// time. 201 is reserved for the call that actually assembled one — the rep
// activated today, or the morning a worker was down for. It reads and stages
// only: no deal field mutates and nothing is sent.
func (h Handlers) GenerateMorningBrief(w http.ResponseWriter, r *http.Request) {
	run, assembled, err := h.engine.SnapshotRunForDay(r.Context(), time.Now().UTC())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	status := http.StatusOK
	if assembled {
		status = http.StatusCreated
	}
	httperr.WriteJSON(w, status, briefRunToWire(run))
}

// MarkBriefItemActed records that the rep acted on a queue item; the next
// run drops the deal until it materially changes (B-E05.13).
func (h Handlers) MarkBriefItemActed(w http.ResponseWriter, r *http.Request, itemID openapi_types.UUID) {
	item, err := h.engine.MarkActed(r.Context(), ids.UUID(itemID), time.Now().UTC())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, briefItemToWire(item))
}

// MarkBriefItemDismissed records a dismissal; the deal reappears only if a
// new linked activity arrives after the mark (B-E05.13).
func (h Handlers) MarkBriefItemDismissed(w http.ResponseWriter, r *http.Request, itemID openapi_types.UUID) {
	item, err := h.engine.MarkDismissed(r.Context(), ids.UUID(itemID), time.Now().UTC())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, briefItemToWire(item))
}

// SnoozeBriefItem hides a queue item until the requested instant, after
// which it re-surfaces as actionable (A77/AC-home-6). A snooze that is
// already over would be a no-op wearing a success code — refused as
// client error instead.
func (h Handlers) SnoozeBriefItem(w http.ResponseWriter, r *http.Request, itemID openapi_types.UUID) {
	var req crmcontracts.BriefSnoozeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	now := time.Now().UTC()
	if !req.SnoozedUntil.After(now) {
		httperr.Write(w, r, httperr.Validation("snoozed_until", "not_in_future", "snoozed_until must lie in the future"))
		return
	}
	item, err := h.engine.MarkSnoozed(r.Context(), ids.UUID(itemID), req.SnoozedUntil, now)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, briefItemToWire(item))
}

// AnnotateMorningBrief writes the overnight pass's findings onto the acting
// rep's own current run.
//
// 204 rather than the annotated run: the caller is the agent that just wrote
// it, and handing the prose straight back is how a loop reads its own output as
// new information and talks itself into a second pass. The person reads it
// through GET /brief like everything else.
func (h Handlers) AnnotateMorningBrief(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.AnnotateBriefRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	ann := Annotation{Items: make([]ItemAnnotation, 0, len(req.Items))}
	if req.Narrative != nil {
		ann.Narrative = *req.Narrative
	}
	for _, item := range req.Items {
		one := ItemAnnotation{ItemID: ids.UUID(item.ItemId), Finding: item.Finding}
		for _, cited := range item.CitedEvidence {
			one.CitedEvidence = append(one.CitedEvidence, ids.UUID(cited))
		}
		ann.Items = append(ann.Items, one)
	}
	if err := h.engine.AnnotateCurrentRun(r.Context(), ann, time.Now().UTC()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func briefRunToWire(run BriefRun) crmcontracts.MorningBrief {
	items := make([]crmcontracts.MorningBriefItem, 0, len(run.Items))
	for _, item := range run.Items {
		items = append(items, briefItemToWire(item))
	}
	norm := run.RevenueNormMinor
	day := openapi_types.Date{Time: run.LocalDay}
	// Omitted rather than empty when the run predates the column: a client
	// reading "" would caption the figure with nothing, which is the bare
	// number this field exists to stop.
	var normCurrency *string
	if run.RevenueNormCurrency != "" {
		normCurrency = &run.RevenueNormCurrency
	}
	return crmcontracts.MorningBrief{
		Id:                  openapi_types.UUID(run.ID),
		GeneratedAt:         run.GeneratedAt,
		AsOf:                run.AsOf,
		LocalDay:            &day,
		CandidateCount:      run.CandidateCount,
		RevenueNormMinor:    &norm,
		RevenueNormCurrency: normCurrency,
		Narrative:           nullableText(run.Narrative),
		AnnotatedAt:         run.AnnotatedAt,
		Items:               items,
	}
}

// nullableText serves empty prose as JSON null rather than "".
//
// The distinction is the contract's: null means no pass wrote one, and the
// screen tells that apart from "a pass ran and had nothing to say" through
// annotated_at. An empty string on the wire would be a third spelling of the
// same absence that neither field documents.
func nullableText(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

// lineageToWire renders why a dismissed deal came back, absent when it never
// was dismissed.
func lineageToWire(lineage *ItemLineage) *crmcontracts.MorningBriefItemLineage {
	if lineage == nil {
		return nil
	}
	return &crmcontracts.MorningBriefItemLineage{
		DismissedOn:            openapi_types.Date{Time: lineage.DismissedOn},
		ReturnedWithActivityAt: lineage.ReturnedWith,
	}
}

func briefItemToWire(item BriefRunItem) crmcontracts.MorningBriefItem {
	evidence := make([]openapi_types.UUID, 0, len(item.EvidenceIDs))
	for _, id := range item.EvidenceIDs {
		evidence = append(evidence, openapi_types.UUID(id))
	}
	return crmcontracts.MorningBriefItem{
		Id:        openapi_types.UUID(item.ID),
		DealId:    openapi_types.UUID(item.DealID),
		Rank:      item.Rank,
		Composite: float32(item.Composite),
		FeatureVector: crmcontracts.MorningBriefFeatureVector{
			Winnability: float32(item.Features.Winnability),
			Revenue:     float32(item.Features.Revenue),
			Timing:      float32(item.Features.Timing),
			Momentum:    float32(item.Features.Momentum),
			Warmth:      float32(item.Features.Warmth),
		},
		EvidenceIds:  evidence,
		State:        crmcontracts.MorningBriefItemState(item.State),
		StateAt:      item.StateAt,
		SnoozedUntil: item.SnoozedUntil,
		Finding:      nullableText(item.Finding),
		Lineage:      lineageToWire(item.Lineage),
	}
}
