// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type ActivityKind = components["schemas"]["AiActivityItem"]["kind"];

/**
 * Which of the Core's two live states a running occurrence puts the orb in.
 *
 * The Core's vocabulary is the agent's work lifecycle (WDS-CORE-2), and `ingest`
 * and `working` are two halves of it that mean different things to a reader:
 * evidence coming IN, and the agent reasoning over evidence it already holds.
 * Only the agent can put the orb in either. The rail used to reach `ingest` from
 * the browser's own fetches, which meant the one state named for the agent
 * taking something in was the one state the agent could never cause.
 *
 * TOTAL over the contract's kinds, and the compiler holds it there, for the same
 * reason `ACTIVITY_LINE` is total: a kind added upstream must be classified by
 * somebody who knows what it does, not defaulted by whoever added it.
 *
 * Totality covers kinds the rail does not narrate, and that is deliberate. The
 * lane is a fact about the WORK, not about whether this surface draws a sentence
 * for it: `enrich` is evidence arriving whether or not a line is written for it,
 * and a map that classified only the displayed seven would have to be revisited
 * every time the copy file changed its mind about what to show.
 */
export type AgentLane = "ingest" | "working";

export const ACTIVITY_LANE: Readonly<Record<ActivityKind, AgentLane>> = {
  // Evidence arriving: mail, documents, web pages, transcripts, provider data,
  // and the reader's own past writing. In every case the agent is taking
  // something in that it did not have a moment ago.
  capture_classify: "ingest",
  owed_verdict: "ingest",
  capture_confidentiality_verdict: "ingest",
  capture_counterparty_verdict: "ingest",
  cold_start: "ingest",
  document_extract: "ingest",
  enrich: "ingest",
  rate_extract: "ingest",
  signal_extract: "ingest",
  site_extract: "ingest",
  site_fact_extract: "ingest",
  site_triage: "ingest",
  transcript: "ingest",
  voice_build: "ingest",
  // The account scan takes the account's exchanges in and reads them; what
  // it produces is a reading of what arrived, not a draft.
  account_scan: "ingest",

  // Reasoning over what is already held, and producing something from it: a
  // ranking, a verdict, a draft, a summary, a review.
  brief_ranking: "working",
  cert_judge: "working",
  // The retrieval is settled before this task is reached — the passages have
  // already cleared the grounding floor — so what the model does here is write
  // the answer from evidence the workspace already holds.
  corpus_ask: "working",
  deal_health: "working",
  draft_reply: "working",
  growth_fit: "working",
  morning_brief: "working",
  nl_search: "working",
  offer_draft: "working",
  overnight_at_risk_sweep: "working",
  summarize: "working",
  propose_roles: "working",
  transcript_propose: "working",
  weekly_review: "working",
};

/**
 * The lane one running occurrence belongs to.
 *
 * A kind this build has never heard of is what an older tab gets from a newer
 * server, and it answers `working`. That is the honest half of what is known:
 * something is running, and which half of the lifecycle it is in cannot be said.
 * Answering `null` and letting the caller rest the orb would be worse, because
 * an agent reported at rest while it runs is the one claim this surface exists
 * to prevent.
 */
export function laneFor(item: Readonly<{ kind: string }>): AgentLane {
  return isActivityKind(item.kind) ? ACTIVITY_LANE[item.kind] : "working";
}

/**
 * Whether a raw string is a kind this build knows, as a narrowing the runtime
 * actually performed rather than an assertion somebody wrote.
 */
function isActivityKind(kind: string): kind is ActivityKind {
  return Object.hasOwn(ACTIVITY_LANE, kind);
}
