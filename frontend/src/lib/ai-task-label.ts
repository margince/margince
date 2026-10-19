import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

type AiTask = components["schemas"]["AiTask"];

const TASK_NAME_KEYS: Readonly<Record<AiTask, MessageKey>> = {
  account_scan: "aiTask.name.account_scan",
  agent_loop: "aiTask.name.agent_loop",
  brief_ranking: "aiTask.name.brief_ranking",
  capture_classify: "aiTask.name.capture_classify",
  capture_confidentiality_verdict:
    "aiTask.name.capture_confidentiality_verdict",
  capture_counterparty_verdict: "aiTask.name.capture_counterparty_verdict",
  cert_judge: "aiTask.name.cert_judge",
  cold_start: "aiTask.name.cold_start",
  corpus_ask: "aiTask.name.corpus_ask",
  deal_health: "aiTask.name.deal_health",
  document_extract: "aiTask.name.document_extract",
  draft_reply: "aiTask.name.draft_reply",
  enrich: "aiTask.name.enrich",
  growth_fit: "aiTask.name.growth_fit",
  nl_search: "aiTask.name.nl_search",
  offer_draft: "aiTask.name.offer_draft",
  owed_verdict: "aiTask.name.owed_verdict",
  propose_roles: "aiTask.name.propose_roles",
  rate_extract: "aiTask.name.rate_extract",
  request_settlement: "aiTask.name.request_settlement",
  signal_extract: "aiTask.name.signal_extract",
  site_extract: "aiTask.name.site_extract",
  site_fact_extract: "aiTask.name.site_fact_extract",
  site_triage: "aiTask.name.site_triage",
  stage_evidence_extract: "aiTask.name.stage_evidence_extract",
  summarize: "aiTask.name.summarize",
  tag_suggest: "aiTask.name.tag_suggest",
  transcript: "aiTask.name.transcript",
  transcript_propose: "aiTask.name.transcript_propose",
  voice_build: "aiTask.name.voice_build",
  weekly_learnings: "aiTask.name.weekly_learnings",
  weekly_review: "aiTask.name.weekly_review",
};

function isAiTask(value: string): value is AiTask {
  return Object.hasOwn(TASK_NAME_KEYS, value);
}

export function aiTaskLabel(
  task: string | undefined,
  t: Translator,
  fallback?: string | null,
): string {
  if (task === "embeddings") return t("aiRouting.lane.embeddings");
  if (task && isAiTask(task)) return t(TASK_NAME_KEYS[task]);
  const name = fallback?.trim();
  return name && name !== task ? name : t("aiTask.name.unknown");
}
